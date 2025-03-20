package chrome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/KakashiHatake324/mockjs"
)

func TestLaunchChrome(t *testing.T) {
	ctx := context.Background()
	chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	expected := "Launching Chrome from /Applications/Google Chrome.app/Contents/MacOS/Google Chrome on port 9222"

	options, err := NewOptions(&ctx, chromePath, false, "", "", false, true)
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
		validPath := false
		for _, expectedPath := range expectedPaths {
			if path == expectedPath {
				validPath = true
				break
			}
		}
		if !validPath {
			t.Fatalf("Invalid Chrome path for Windows: %s", path)
		}
	}
}

func TestLaunchChromeWithArgs(t *testing.T) {
	ctx := context.Background()
	chromePath, err := GetChromePath()
	if err != nil {
		t.Fatalf("GetChromePath() error = %v", err)
	}
	headless := false
	pd := mockjs.Random_range(100000, 999999)
	options, err := NewOptions(&ctx, chromePath, headless, "142.173.80.190:5190:MZeH5aeTIh:CwEOKuP6Ca", fmt.Sprintf("%d", pd), true, true)
	if err != nil {
		t.Fatalf("NewOptions() error = %v", err)
	}

	args := []FlagType{
		DisableFeatures([]string{"PreloadMediaEngagementData", "AutofillServerCommunication"}),
		DisableGPU,
		DisableExtentions,
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
	err = browser.GetCurrentPage().Navigate("https://www.ticketmaster.com")
	if err != nil {
		if err.Error() != "Invalid InterceptionId." {
			t.Fatalf("Navigate() error = %v", err)
		}
	}
	time.Sleep(2 * time.Second)
	err = browser.GetCurrentPage().Navigate("https://checkout.ticketmaster.com/0dea01533ffa4e44a653c87237175238?ccp_channel=0\u0026ccp_src=2\u0026edp=https%3A%2F%2Fwww.ticketmaster.com%2Fevent%2F3C00618CF2C7211C\u0026f_appview=false\u0026f_appview_ln=false\u0026f_appview_version=1\u0026f_layout=")
	if err != nil {
		if err.Error() != "Invalid InterceptionId." {
			t.Fatalf("Navigate() error = %v", err)
		}
	}
	time.Sleep(2 * time.Second)
	t.Logf("Passed test")
}
