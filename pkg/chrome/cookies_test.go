package chrome

import (
	"context"
	"testing"
	"time"
)

func TestCookieOperations(t *testing.T) {
	// Create context
	ctx := context.Background()

	// Create browser options
	opts, err := NewOptions(&ctx, "", true, "", "test", true, false)
	if err != nil {
		t.Fatalf("Failed to create browser options: %v", err)
	}

	// Launch browser
	browser, err := LaunchChrome("", opts)
	if err != nil {
		t.Fatalf("Failed to launch browser: %v", err)
	}
	defer browser.Close()

	// Get the current page
	page := browser.GetCurrentPage()
	if page == nil {
		t.Fatal("Failed to get current page")
	}

	// Navigate to example.com
	if err := page.Navigate("https://example.com"); err != nil {
		t.Fatalf("Failed to navigate to example.com: %v", err)
	}

	// Wait for page to load
	time.Sleep(2 * time.Second)

	// Create a test cookie
	testCookie := &Cookie{
		Name:    "testCookie",
		Value:   "testValue",
		Domain:  "example.com",
		Path:    "/",
		Expires: time.Now().Add(24 * time.Hour).Unix(),
	}

	// Set the cookie
	if err := page.SetCookie(testCookie); err != nil {
		t.Fatalf("Failed to set cookie: %v", err)
	}

	// Get all cookies
	cookies, err := page.GetAllCookies()
	if err != nil {
		t.Fatalf("Failed to get cookies: %v", err)
	}

	// Verify the cookie was set correctly
	found := false
	for _, cookie := range cookies {
		if cookie.Name == testCookie.Name {
			found = true
			if cookie.Value != testCookie.Value {
				t.Errorf("Cookie value mismatch: got %s, want %s", cookie.Value, testCookie.Value)
			}
			if cookie.Domain != testCookie.Domain {
				t.Errorf("Cookie domain mismatch: got %s, want %s", cookie.Domain, testCookie.Domain)
			}
			break
		}
	}

	if !found {
		t.Error("Test cookie was not found after setting")
	}
}
