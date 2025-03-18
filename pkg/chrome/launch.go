package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/KakashiHatake324/mockjs"
)

// GetChromePath returns the default Chrome executable path based on the operating system
func GetChromePath() (string, error) {
	switch runtime.GOOS {
	case "darwin":
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
func LaunchChrome(startUrl string, opts *Options, argsOpts ...[]string) (*Browser, error) {

	if opts.GetVerbose() {
		opts.GetLogger().Info("Verbose mode enabled")
	}

	if startUrl == "" {
		startUrl = "about:blank"
	}

	// Set args
	args := []string{}
	if argsOpts != nil {
		args = argsOpts[0]
		opts.SetArgs(args)
	}
	args = append(args, []string{
		fmt.Sprintf("--remote-debugging-port=%s", opts.GetPort()),
		"--no-first-run",
	}...)

	// Set headless
	if opts.GetHeadless() {
		args = append(args, "--headless=new")
	}

	if opts.GetProxy() != "" {
		args = append(args, fmt.Sprintf("--proxy-server=%s", opts.GetProxy()))
	}

	if opts.GetUser() != "" {
		args = append(args, fmt.Sprintf("--user-data-dir=%s", opts.GetUser()))
	} else {
		if runtime.GOOS == "windows" {
			args = append(args, "--user-data-dir=%TEMP%\\chrome-temp")
		} else {
			args = append(args, "--user-data-dir=/tmp/chrome-temp")
		}
	}

	// Launch Chrome
	cmd := exec.CommandContext(context.Background(), opts.GetChromePath(), args...)
	if err := cmd.Start(); err != nil {
		if opts.GetVerbose() {
			opts.GetLogger().Info(fmt.Sprintf("chrome failed to start: %v", err))
		}
		return nil, fmt.Errorf("failed to start browser: %v", err)
	}

	// Wait for Chrome debugger to be ready
	if err := waitForChromeDebugger(opts.GetPort(), 10*time.Second); err != nil {
		if opts.GetVerbose() {
			opts.GetLogger().Info(fmt.Sprintf("chrome failed to start: %v", err))
		}
		return nil, fmt.Errorf("chrome failed to start: %v", err)
	}

	pid := cmd.Process.Pid
	if opts.GetVerbose() {
		opts.GetLogger().Info(fmt.Sprintf("Chrome started with PID: %d", pid))
	}

	page, err := connectPage(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to page: %v", err)
	}

	if opts.GetVerbose() {
		opts.GetLogger().Info(fmt.Sprintf("connected to page: %s", page.id))
	}

	browser := newBrowser(opts.GetContext(), cmd.Process, opts.GetProxy(), opts, page, page.id)

	if opts.GetProxy() != "" {
		page.EnableFetch()
	}
	page.EnableNetwork()
	page.EnablePage()

	if startUrl != "" {
		if err := browser.GetCurrentPage().Navigate(startUrl); err != nil {
			return nil, fmt.Errorf("failed to navigate to %s: %v", startUrl, err)
		}
	}
	if opts.GetVerbose() {
		opts.GetLogger().Info(fmt.Sprintf("intialized chrome browser: %s", startUrl))
	}
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

		opts.GetLogger().Info(mockjs.InitWindow().JSON.Stringify(pages))
		for _, page := range pages {
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
		time.Sleep(1 * time.Second)
	}

	return nil, errors.New("no page found")
}
