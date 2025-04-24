package chrome

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/KakashiHatake324/mockjs"
	"github.com/google/uuid"
)

// GetChromePath returns the default Chrome executable path based on the operating system
func GetChromePath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		// macOS Chrome path
		chromiumPath := "/Applications/Chromium.app/Contents/MacOS/Chromium"
		if _, err := os.Stat(chromiumPath); err == nil {
			return chromiumPath, nil
		}
		// macOS Chrome path
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chromePath); err == nil {
			return chromePath, nil
		}
		// Check for Chrome Canary on macOS
		canaryPath := "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary"
		if _, err := os.Stat(canaryPath); err == nil {
			return canaryPath, nil
		}
	case "windows":
		// Windows Chrome paths
		possiblePaths := []string{
			// Windows Chromium paths
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Chromium", "Application", "chromium.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Chromium", "Application", "chromium.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Chromium", "Application", "chromium.exe"),
			// Windows Chrome paths
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
		}
		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	return "", fmt.Errorf("chrome not found on %s", runtime.GOOS)
}

// LaunchChrome launches a new instance of Chrome
func LaunchChrome(startUrl string, opts *Options, argsOpts ...[]FlagType) (*Browser, error) {
	var id string
	if opts.GetUser() != "" {
		id = opts.GetUser()
	} else {
		id = uuid.New().String()
	}

	if opts.GetVerbose() {
		opts.GetLogger().Info("Verbose mode enabled")
	}
	if opts.GetChromePath() == "" {
		chromePath, err := GetChromePath()
		if err != nil {
			return nil, fmt.Errorf("error getting chrome path: %v", err)
		}
		opts.SetChromePath(chromePath)
	}

	if strings.Contains(opts.GetChromePath(), "chromium.exe") {
		opts.SetName("chromium.exe")
	} else {
		opts.SetName("chrome.exe")
	}

	// Set args
	args := []FlagType{}
	if argsOpts != nil {
		args = argsOpts[0]
		opts.SetArgs(args)
	}

	// Retrieve the current user
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("error retrieving user: %v", err)
	}

	//WINDOWS
	//C:\users\USER\appdata\local\temp\
	userDataDir := filepath.Join(usr.HomeDir, "tmp", opts.GetUser())
	if runtime.GOOS != "windows" {
		userDataDir = "/" + userDataDir
	}
	/*
		if err := internals.UnzipDefaultProfile(userDataDir); err != nil {
			return nil, fmt.Errorf("error unzipping default profile: %v", err)
		}
	*/
	if opts.GetUser() != "" {
		args = append(args, FlagType(fmt.Sprintf("--user-data-dir=%s", userDataDir)))
	} else {
		if runtime.GOOS == "windows" {
			args = append(args, FlagType("--user-data-dir=%TEMP%\\chrome-temp"))
		} else {
			args = append(args, FlagType("--user-data-dir=/tmp/chrome-temp"))
		}
	}

	// Set headless
	if opts.GetHeadless() {
		args = append(args, FlagType("--headless=new"))
	}

	// Load extension
	if opts.GetExtensionPath() != "" {
		if opts.GetHeadless() {
			return nil, fmt.Errorf("headless mode is not supported with extensions")
		}
		args = append(args, FlagType(fmt.Sprintf("--load-extension=%s", opts.GetExtensionPath())))
	}

	// Set proxy
	if opts.GetProxy() != "" {
		args = append(args, FlagType(fmt.Sprintf("--proxy-server=%s", opts.GetProxy())))
	}
	args = append(args, FlagType(fmt.Sprintf("--remote-debugging-port=%s", opts.GetPort())))
	if opts.GetVerbose() {
		// Convert []FlagType to []string for strings.Join
		strArgs := make([]string, len(args))
		for i, arg := range args {
			strArgs[i] = string(arg)
		}
		opts.GetLogger().Warn(fmt.Sprintf("%s %s", opts.GetChromePath(), strings.Join(strArgs, " ")))
	}
	// Launch Chrome
	// Convert []FlagType to []string for exec.Command
	strArgs := make([]string, len(args))
	for i, arg := range args {
		strArgs[i] = string(arg)
	}
	cmd := exec.CommandContext(*opts.GetContext(), opts.GetChromePath(), strArgs...)
	if err := cmd.Start(); err != nil {
		opts.handleVerbose(fmt.Sprintf("chrome failed to start: %v", err))
		return nil, fmt.Errorf("failed to start browser: %v", err)
	}

	// Wait for Chrome debugger to be ready
	if err := waitForChromeDebugger(opts.GetPort(), 25*time.Second); err != nil {
		opts.handleVerbose(fmt.Sprintf("chrome failed to start: %v", err))
		return nil, fmt.Errorf("chrome failed to start: %v", err)
	}

	pid := cmd.Process.Pid
	opts.handleVerbose(fmt.Sprintf("Chrome started with PID: %d", pid))
	page, err := connectPage(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to page: %v", err)
	}
	opts.handleVerbose(fmt.Sprintf("connected to page: %s", page.id))

	browser := newBrowser(id, opts.GetContext(), cmd, opts.GetProxy(), opts, page, page.id)

	if startUrl != "" {
		if err := browser.GetCurrentPage().Navigate(startUrl); err != nil {
			return nil, fmt.Errorf("failed to navigate to %s: %v", startUrl, err)
		}
	}

	opts.handleVerbose(fmt.Sprintf("intialized chrome browser: %s", startUrl))

	browser.SetWait(func(cmd *exec.Cmd) (*os.ProcessState, error) {
		return cmd.Process.Wait()
	})

	return browser, nil
}

// connectPage connects to a page and returns a Page object
func connectPage(opts *Options) (*Page, error) {
	for range 10 {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%s/json", opts.GetPort()))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch active pages: %v", err)
		}
		defer resp.Body.Close()

		var pages []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&pages); err != nil {
			return nil, fmt.Errorf("failed to decode JSON: %v", err)
		}
		opts.handleVerbose(mockjs.InitWindow().JSON.Stringify(pages))
		for _, page := range pages {
			if page["type"] != "page" {
				continue
			}
			pageId := page["id"].(string)
			wsUrl := page["webSocketDebuggerUrl"].(string)
			currentUrl := page["url"].(string)
			p := newPage(pageId, wsUrl, currentUrl, opts.GetVerbose(), opts.GetProxyUser(), opts.GetProxyPass())
			p.wsConn, err = p.newSocket(wsUrl)
			if err != nil {
				continue
			}
			return p, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return nil, errors.New("no page found")
}
