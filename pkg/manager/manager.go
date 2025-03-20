package manager

import (
	"sync"

	"github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
)

// BrowserManager is a manager for the browsers
type BrowserManager struct {
	browsers map[string]*chrome.Browser
	mu       sync.Mutex
}

// NewBrowserManager creates a new BrowserManager
func NewBrowserManager() *BrowserManager {
	return &BrowserManager{
		browsers: make(map[string]*chrome.Browser),
		mu:       sync.Mutex{},
	}
}

// GetBrowser returns a browser by id
func (m *BrowserManager) GetBrowser(id string) *chrome.Browser {
	return m.browsers[id]
}

// AddBrowser adds a browser to the manager
func (m *BrowserManager) AddBrowser(browser *chrome.Browser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.browsers[browser.GetID()] = browser
}

// GetAllBrowsers returns all browsers
func (m *BrowserManager) GetAllBrowsers() []*chrome.Browser {
	m.mu.Lock()
	defer m.mu.Unlock()
	var browsers []*chrome.Browser
	for _, browser := range m.browsers {
		browsers = append(browsers, browser)
	}
	return browsers
}

// RemoveBrowser removes a browser by id
func (m *BrowserManager) RemoveBrowser(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if browser, ok := m.browsers[id]; ok {
		browser.Close()
		delete(m.browsers, id)
	}
	return nil
}

// CloseAllBrowsers closes all browsers
func (m *BrowserManager) CloseAllBrowsers() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, browser := range m.browsers {
		browser.Close()
	}
	m.browsers = make(map[string]*chrome.Browser)
	return nil
}

// GetBrowserCount returns the number of browsers
func (m *BrowserManager) GetBrowserCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.browsers)
}

// InitializeBrowser initializes a new browser
func (m *BrowserManager) InitializeBrowser(startUrl string, opts *chrome.Options, args ...[]chrome.FlagType) (*chrome.Browser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	browser, err := chrome.LaunchChrome(startUrl, opts, args...)
	if err != nil {
		return nil, err
	}
	m.browsers[browser.GetID()] = browser
	return browser, nil
}
