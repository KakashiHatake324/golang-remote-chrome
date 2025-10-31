package manager

import (
	"context"
	"testing"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
	"github.com/google/uuid"
)

func TestBrowserManagerMultipleBrowsers(t *testing.T) {
	// Create context
	ctx := context.Background()

	// Create browser options for both browsers (headless mode)
	opts, err := chrome.NewOptions(&ctx, "", false, "", uuid.New().String(), false, true, []string{}, false)
	if err != nil {
		t.Fatalf("Failed to create browser options: %v", err)
	}

	// Create browser options for both browsers (headless mode)
	opts2, err := chrome.NewOptions(&ctx, "", false, "", uuid.New().String(), false, true, []string{}, false)
	if err != nil {
		t.Fatalf("Failed to create browser options: %v", err)
	}

	// Create browser manager
	manager := NewBrowserManager()

	// Initialize first browser
	browser1, err := manager.InitializeBrowser("", opts)
	if err != nil {
		t.Fatalf("Failed to initialize first browser: %v", err)
	}

	// Initialize second browser
	browser2, err := manager.InitializeBrowser("", opts2)
	if err != nil {
		t.Fatalf("Failed to initialize second browser: %v", err)
	}

	// Navigate both browsers to example.com
	for _, browser := range []*chrome.Browser{browser1, browser2} {
		page := browser.GetCurrentPage()
		if err := page.Navigate("https://example.com"); err != nil {
			t.Fatalf("Failed to navigate to example.com: %v", err)
		}
		// Wait for page to load
		time.Sleep(2 * time.Second)
	}

	// Verify we have 2 browsers
	if count := manager.GetBrowserCount(); count != 2 {
		t.Errorf("Expected 2 browsers, got %d", count)
	}

	// Close first browser
	if err := manager.RemoveBrowser(browser1.GetID()); err != nil {
		t.Errorf("Failed to close first browser: %v", err)
	}

	// Verify all browsers are closed
	if count := manager.GetBrowserCount(); count != 1 {
		t.Errorf("Expected 1 browsers, got %d", count)
	}

	// Wait 5 seconds
	time.Sleep(2 * time.Second)

	// Close second browser
	if err := manager.RemoveBrowser(browser2.GetID()); err != nil {
		t.Errorf("Failed to close second browser: %v", err)
	}

	// Verify all browsers are closed
	if count := manager.GetBrowserCount(); count != 0 {
		t.Errorf("Expected 0 browsers, got %d", count)
	}

	// Initialize first browser
	browser3, err := manager.InitializeBrowser("", opts)
	if err != nil {
		t.Fatalf("Failed to initialize first browser: %v", err)
	}

	// Verify we have 1 browser
	if count := manager.GetBrowserCount(); count != 1 {
		t.Errorf("Expected 1 browsers, got %d", count)
	}

	if err := browser3.GetCurrentPage().NavigateWithWaitLoad("https://example.com"); err != nil {
		t.Fatalf("Failed to navigate to example.com: %v", err)
	}

	// Close first browser
	if err := manager.CloseAllBrowsers(); err != nil {
		t.Errorf("Failed to close all browsers: %v", err)
	}

	// Verify we have 0 browsers
	if count := manager.GetBrowserCount(); count != 0 {
		t.Errorf("Expected 0 browsers, got %d", count)
	}

}
