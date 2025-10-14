package chrome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"slices"

	"github.com/KakashiHatake324/mockjs"
)

func TestLaunchChrome(t *testing.T) {
	ctx := context.Background()
	chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	expected := "Launching Chrome from /Applications/Google Chrome.app/Contents/MacOS/Google Chrome on port 9222"

	options, err := NewOptions(&ctx, chromePath, false, "", "", false, true, []string{})
	if err != nil {
		t.Fatalf("NewOptions() error = %v", err)
		return
	}
	got, err := LaunchChrome("https://www.google.com", options)
	if err != nil {
		t.Fatalf("LaunchChrome() error = %v", err)
		return
	}
	if got.Opts.GetChromePath() != expected {
		t.Fatalf("LaunchChrome() = %q, want %q", got.Opts.GetChromePath(), expected)
	}
}

func TestGetChromePath(t *testing.T) {
	path, err := GetChromePath()
	if err != nil {
		t.Fatalf("GetChromePath() error = %v", err)
		return
	}

	// Verify the path exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Chrome path does not exist: %v", err)
	}

	// Verify the path is correct for the current OS
	switch runtime.GOOS {
	case "darwin":
		if path != "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" &&
			path != "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary" {
			t.Fatalf("Invalid Chrome path for macOS: %s", path)
		}
	case "windows":
		expectedPaths := []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "Google", "Chrome", "Application", "chrome.exe"),
		}
		validPath := slices.Contains(expectedPaths, path)
		if !validPath {
			t.Fatalf("Invalid Chrome path for Windows: %s", path)
		}
	}
}

func TestLaunchChromeWithArgs(t *testing.T) {
	eventPage := "https://www.ticketmaster.com/metallica-m72-world-tour-atlanta-georgia-06-03-2025/event/0E00612DBA014D3E"
	extensionPath := "/Users/mpro4/Library/Application Support/golang-backend/files/tm-presale-ext"
	// proxy := "142.173.80.190:5190:MZeH5aeTIh:CwEOKuP6Ca"
	proxy := ""
	ctx := context.Background()
	chromePath, err := GetChromePath()
	if err != nil {
		t.Fatalf("GetChromePath() error = %v", err)
	}
	headless := false
	pd := mockjs.Random_range(100000, 999999)
	options, err := NewOptions(&ctx, chromePath, headless, proxy, fmt.Sprintf("%d", pd), true, true, []string{extensionPath})
	if err != nil {
		t.Fatalf("NewOptions() error = %v", err)
	}

	args := []FlagType{
		DisableFeatures([]string{"PreloadMediaEngagementData", "AutofillServerCommunication"}),
		DisableGPU,
		DisableBackgroundMode,
		DisableSoftwareRasterizer,
		NoFirstRun,
	}

	browser, err := LaunchChrome("", options, args)
	if err != nil {
		t.Fatalf("LaunchChrome() error = %v", err)
	}
	defer browser.Close()
	if browser.Opts.GetChromePath() != chromePath {
		t.Fatalf("LaunchChrome() = %q, want %q", browser.Opts.GetChromePath(), chromePath)
	}
	if browser.Opts.GetHeadless() != headless {
		t.Fatalf("LaunchChrome() = %t, want %t", browser.Opts.GetHeadless(), headless)
	}

	browser.GetCurrentPage().EnablePage()
	if browser.Opts.GetProxy() != "" {
		browser.GetCurrentPage().EnableFetch()
	}
	browser.GetCurrentPage().EnableNetwork()

	time.Sleep(2 * time.Second)
	err = browser.GetCurrentPage().Navigate(eventPage)
	if err != nil {
		if err.Error() != "Invalid InterceptionId." {
			t.Fatalf("Navigate() error = %v", err)
		}
	}
	time.Sleep(10 * time.Minute)
	t.Logf("Passed test")
}
