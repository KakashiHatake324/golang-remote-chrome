# golang-remote-chrome

A Go library for controlling Chrome via the Chrome DevTools Protocol. Useful for web automation, testing, and scraping.

## Features

- Launch and control Chrome instances
- Manage multiple browser instances
- Navigate web pages
- Set and get cookies
- Handle proxy authentication
- Headless mode support

## Installation

```bash
go get github.com/KakashiHatake324/golang-remote-chrome
```

## Quick Start

```go
package main

import (
    "context"
    "log"
    chrome "github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
)

func main() {
    ctx := context.Background()
    
    // Create browser options
    opts, err := chrome.NewOptions(&ctx, "", true, "", "", false, true)
    if err != nil {
        log.Fatal(err)
    }

    // Launch Chrome
    browser, err := chrome.LaunchChrome("", opts)
    if err != nil {
        log.Fatal(err)
    }
    defer browser.Close()

    // Get the page
    page := browser.GetCurrentPage()

    // Navigate to a website
    err = page.Navigate("https://example.com")
    if err != nil {
        log.Fatal(err)
    }
}
```

## Core Functions

### Fetch
```go
// Enable fetch for proxy integration
err := page.EnableFetch()

// Disable fetch after the proxy has connected
err := page.DisableFetch()
```

### Page
```go
// Enable page to receive page notifications
err := page.EnablePage()
```

### Page
```go
// Enable network to receive network notifications
err := page.EnableNetwork()
```

### Navigation
```go
// Simple navigation
err := page.Navigate("https://example.com")

// Navigation with wait for page load
err := page.NavigateWithWaitLoad("https://example.com")
```

### Cookie Management
```go
// Set a cookie
cookie := &chrome.Cookie{
    Name:    "session",
    Value:   "abc123",
    Domain:  "example.com",
    Path:    "/",
    Expires: time.Now().Add(24 * time.Hour).Unix(),
}
err := page.SetCookie(cookie)

// Get all cookies
cookies, err := page.GetAllCookies()
for _, cookie := range cookies {
    log.Printf("Cookie: %s = %s", cookie.Name, cookie.Value)
}

// Set multiple cookies
cookies := []*chrome.Cookie{
    {
        Name:   "session",
        Value:  "abc123",
        Domain: "example.com",
    },
    {
        Name:   "user",
        Value:  "john",
        Domain: "example.com",
    },
}
err := page.SetCookieCookies(cookies)
```

### Browser Manager
```go
package main

import (
    "context"
    "log"
    "github.com/KakashiHatake324/golang-remote-chrome/pkg/manager"
    "github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
)

func main() {
    ctx := context.Background()
    
    // Create browser manager
    browserManager := manager.NewBrowserManager()

    // Create options for two browsers
    opts1, _ := chrome.NewOptions(&ctx, "", false, "", "", false, true)
    opts2, _ := chrome.NewOptions(&ctx, "", false, "", "", false, true)

    // Initialize browsers
    browser1, err := browserManager.InitializeBrowser("", opts1)
    if err != nil {
        log.Fatal(err)
    }

    browser2, err := browserManager.InitializeBrowser("", opts2)
    if err != nil {
        log.Fatal(err)
    }

    // Use browsers
    browser1.GetCurrentPage().Navigate("https://example.com")
    browser2.GetCurrentPage().Navigate("https://example.org")

    // Close specific browser
    browserManager.RemoveBrowser(browser1.GetID())

    // Close all browsers
    browserManager.CloseAllBrowsers()
}
```

The Browser Manager provides:
- Multiple browser instance management
- Individual browser control
- Browser count tracking
- Safe concurrent access
- Clean shutdown of all browsers

## Requirements

- Go 1.21 or later
- Chrome/Chromium browser installed

## License

MIT License 