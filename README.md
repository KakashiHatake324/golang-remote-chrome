# golang-remote-chrome

A Go library for controlling Chrome via the Chrome DevTools Protocol. Useful for web automation, testing, and scraping.

## Features

- Launch and control Chrome instances
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

### Navigation
```go
// Simple navigation
err := page.Navigate("https://example.com")

// Navigation with wait for page load
err := page.NavigateWithWait("https://example.com")
time.Sleep(2 * time.Second) // Wait for dynamic content
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

## Requirements

- Go 1.32 or later
- Chrome/Chromium browser installed

## License

MIT License 