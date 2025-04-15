package chrome

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	internals "github.com/KakashiHatake324/golang-remote-chrome/internal/chrome"
	"github.com/KakashiHatake324/golang-remote-chrome/internal/logger"
	"github.com/google/uuid"
)

// Browser represents a single instance of the Chrome browser
type Browser struct {
	id          string
	context     *context.Context
	cmd         *exec.Cmd
	proxy       string
	Opts        *Options
	Pages       map[string]*Page
	currentPage *Page
	pageLock    sync.Mutex
	logger      *logger.LoggerInstance
	verbose     bool
	wait        func(cmd *exec.Cmd) (*os.ProcessState, error)
}

// newBrowser creates a new Browser
func newBrowser(id string, ctx *context.Context, cmd *exec.Cmd, proxy string, opts *Options, startPage *Page, startPageId string) *Browser {
	return &Browser{
		id:          id,
		context:     ctx,
		cmd:         cmd,
		proxy:       proxy,
		Opts:        opts,
		Pages:       map[string]*Page{startPageId: startPage},
		currentPage: startPage,
		logger:      logger.NewLoggerInstance(uuid.New().String(), "browser"),
		verbose:     opts.GetVerbose(),
	}
}

// SetWait sets the wait function for the Browser
func (b *Browser) SetWait(wait func(cmd *exec.Cmd) (*os.ProcessState, error)) {
	b.wait = wait
}

// WaitClose waits for the browser to close
func (b *Browser) WaitClose() (*os.ProcessState, error) {
	return b.wait(b.cmd)
}

// GetID returns the id of the Browser
func (b *Browser) GetID() string {
	return b.id
}

// GetCurrentPage returns the current page of the Browser
func (b *Browser) GetCurrentPage() *Page {
	return b.currentPage
}

// Close closes the browser and the process and removes the allocated memory
func (b *Browser) Close() error {
	b.pageLock.Lock()
	defer b.pageLock.Unlock()
	if b.verbose {
		b.logger.Info("closing browser")
	}

	// Close all pages first
	for _, page := range b.Pages {
		page.close()
		delete(b.Pages, page.id)
	}

	if b.cmd != nil && b.cmd.Process != nil {
		if b.verbose {
			b.logger.Info("killing browser process")
		}

		pid := b.cmd.Process.Pid

		if runtime.GOOS == "windows" {
			// First attempt with taskkill
			cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
			if output, err := cmd.CombinedOutput(); err != nil {
				if b.verbose {
					b.logger.Warn(fmt.Sprintf("taskkill failed: %v, output: %s", err, output))
				}
				// If taskkill fails, try alternative methods
				// 1. Try to terminate the process directly
				b.cmd.Process.Kill()

				// 2. Double-check with another tasklist to verify
				checkCmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
				if checkOutput, checkErr := checkCmd.CombinedOutput(); checkErr == nil {
					if strings.Contains(string(checkOutput), strconv.Itoa(pid)) {
						// Process still exists, try one more forceful termination
						exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
					}
				}
			}

			// Wait for a short time to ensure process cleanup
			time.Sleep(time.Second)

			// Final verification
			checkCmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
			if output, err := checkCmd.CombinedOutput(); err == nil {
				if strings.Contains(string(output), strconv.Itoa(pid)) {
					if b.verbose {
						b.logger.Warn("Process still exists after termination attempts")
					}
				}
			}
		} else {
			// On Unix-like systems, kill the process group
			pgid, err := syscall.Getpgid(pid)
			if err == nil {
				// Send SIGTERM first for graceful shutdown
				syscall.Kill(-pgid, syscall.SIGTERM)

				// Wait briefly for graceful shutdown
				time.Sleep(time.Second)

				// Force kill if process still exists
				if processExists(pid) {
					syscall.Kill(-pgid, syscall.SIGKILL)
				}
			} else {
				// Fallback to direct process kill if getting pgid fails
				b.cmd.Process.Kill()
			}
		}

		// Wait for the process to finish
		b.cmd.Wait()
	}

	if b.Opts.GetRemoveProfile() {
		if b.verbose {
			b.logger.Warn("deleting profile")
		}
		// Retrieve the current user
		usr, err := user.Current()
		if err != nil {
			return fmt.Errorf("error retrieving user: %v", err)
		}

		userDataDir := filepath.Join(usr.HomeDir, "tmp", b.Opts.GetUser())
		if runtime.GOOS != "windows" {
			userDataDir = "/" + userDataDir
		}
		if err := internals.DeleteProfileFolder(userDataDir); err != nil {
			return fmt.Errorf("error deleting profile: %v", err)
		}
	}
	b = nil
	return nil
}

// Helper function to check if a process exists
func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Sending signal 0 checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ClosePage closes a page and removes it from the Browser
func (b *Browser) ClosePage(page *Page) error {
	b.pageLock.Lock()
	defer b.pageLock.Unlock()
	if page == nil {
		return fmt.Errorf("page not found")
	}
	page.close()
	delete(b.Pages, page.id)
	return nil
}

// GetOptions returns the options of the Browser
func (o *Browser) GetOptions() *Options {
	return o.Opts
}

// GetPages returns the pages of the Browser
func (o *Browser) GetPages() map[string]*Page {
	o.pageLock.Lock()
	defer o.pageLock.Unlock()
	return o.Pages
}

// AddPage adds a page to the Browser
func (o *Browser) AddPage(page *Page) {
	o.pageLock.Lock()
	defer o.pageLock.Unlock()
	o.Pages[page.id] = page
}

// GetPage returns a page from the Browser
func (o *Browser) GetPage(id string) *Page {
	o.pageLock.Lock()
	defer o.pageLock.Unlock()
	for _, page := range o.Pages {
		if page.id == id {
			return page
		}
	}
	return nil
}
