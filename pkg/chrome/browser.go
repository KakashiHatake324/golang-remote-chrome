package chrome

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/google/uuid"

	internals "github.com/KakashiHatake324/golang-remote-chrome/internal/chrome"
	"github.com/KakashiHatake324/golang-remote-chrome/internal/logger"
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
func newBrowser(
	id string, ctx *context.Context, cmd *exec.Cmd, proxy string, opts *Options, startPage *Page, startPageId string,
) *Browser {
	return &Browser{
		id:          id,
		context:     ctx,
		cmd:         cmd,
		proxy:       proxy,
		Opts:        opts,
		Pages:       map[string]*Page{startPageId: startPage},
		pageLock:    sync.Mutex{},
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

		_ = killProcess(pid)

		// Wait for the process to fully exit before touching its profile dir.
		// Chrome shuts down asynchronously and keeps writing to Default/, which
		// would otherwise race the RemoveAll below ("directory not empty").
		if b.wait != nil {
			_, _ = b.wait(b.cmd)
		} else {
			_, _ = b.cmd.Process.Wait()
		}
	}

	// Remove user profile if requested. Delete exactly the --user-data-dir that
	// was used at launch so we never touch a shared parent (e.g. ~/tmp) or a
	// sibling browser's profile.
	if b.Opts.GetRemoveProfile() {
		userDataDir := b.Opts.GetUserDataDir()
		if userDataDir == "" {
			return nil
		}
		if b.verbose {
			b.logger.Warn(fmt.Sprintf("deleting profile %s", userDataDir))
		}
		if err := internals.DeleteProfileFolder(userDataDir); err != nil {
			return fmt.Errorf("error deleting profile: %v", err)
		}
	}

	return nil
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
