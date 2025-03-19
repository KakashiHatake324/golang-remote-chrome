package chrome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/KakashiHatake324/mockjs"
)

func TestLaunchChrome(t *testing.T) {
	ctx := context.Background()
	chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	expected := "Launching Chrome from /Applications/Google Chrome.app/Contents/MacOS/Google Chrome on port 9222"

	options, err := NewOptions(&ctx, chromePath, false, "", "", false, true)
	if err != nil {
		t.Errorf("NewOptions() error = %v", err)
		return
	}
	got, err := LaunchChrome("https://www.google.com", options)
	if err != nil {
		t.Errorf("LaunchChrome() error = %v", err)
		return
	}
	if got.Opts.GetChromePath() != expected {
		t.Errorf("LaunchChrome() = %q, want %q", got.Opts.GetChromePath(), expected)
	}
}

func TestGetChromePath(t *testing.T) {
	path, err := GetChromePath()
	if err != nil {
		t.Errorf("GetChromePath() error = %v", err)
		return
	}

	// Verify the path exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Chrome path does not exist: %v", err)
	}

	// Verify the path is correct for the current OS
	switch runtime.GOOS {
	case "darwin":
		if path != "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" &&
			path != "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary" {
			t.Errorf("Invalid Chrome path for macOS: %s", path)
		}
	case "windows":
		expectedPaths := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
		}
		validPath := false
		for _, expectedPath := range expectedPaths {
			if path == expectedPath {
				validPath = true
				break
			}
		}
		if !validPath {
			t.Errorf("Invalid Chrome path for Windows: %s", path)
		}
	}
}

func TestLaunchChromeWithArgs(t *testing.T) {
	ctx := context.Background()
	chromePath, err := GetChromePath()
	if err != nil {
		t.Errorf("GetChromePath() error = %v", err)
		return
	}
	headless := false
	pd := mockjs.Random_range(100000, 999999)
	options, err := NewOptions(&ctx, chromePath, headless, "http://MZeH5aeTIh:CwEOKuP6Ca@142.173.80.190:5190", fmt.Sprintf("%d", pd), true, true)
	if err != nil {
		t.Errorf("NewOptions() error = %v", err)
		return
	}
	browser, err := LaunchChrome("", options, []string{})
	if err != nil {
		t.Errorf("LaunchChrome() error = %v", err)
		return
	}
	if browser.Opts.GetChromePath() != chromePath {
		t.Errorf("LaunchChrome() = %q, want %q", browser.Opts.GetChromePath(), chromePath)
	}
	if browser.Opts.GetHeadless() != headless {
		t.Errorf("LaunchChrome() = %t, want %t", browser.Opts.GetHeadless(), headless)
	}
	if !slices.Contains(browser.Opts.GetArgs(), "--no-first-run") {
		t.Errorf("LaunchChrome() = %q, want %q", browser.Opts.GetArgs(), []string{"--no-first-run"})
	}
	err = browser.GetCurrentPage().Navigate("https://www.ticketmaster.com/event/0000617D0901855C")
	if err != nil {
		t.Errorf("Navigate() error = %v", err)
		return
	}
	if status, err := browser.WaitClose(); err != nil {
		t.Errorf("WaitClose() error = %v", err)
	} else {
		t.Logf("WaitClose() = %v", status)
	}
}
