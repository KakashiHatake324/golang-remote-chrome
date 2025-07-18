package chrome

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/internal/logger"

	"github.com/gorilla/websocket"
)

// Page represents a single page in the browser
type Page struct {
	id              string
	wsUrl           string
	currentUrl      string
	wsConn          *websocket.Conn
	verbose         bool
	logger          *logger.LoggerInstance
	messageCounter  int
	proxyIdentifier int
	frameId         string
	loadEventFired  chan any
	communicator    chan any
	proxyUser       string
	proxyPass       string
	socketLock      sync.Mutex
	counterLock     sync.Mutex

	requestPausedHandler func(params map[string]any)
	handlerLock          sync.Mutex
}

// newPage creates a new Page
func newPage(id string, wsUrl string, currentUrl string, verbose bool, proxyUser string, proxyPass string) *Page {
	return &Page{
		id:             id,
		wsUrl:          wsUrl,
		currentUrl:     currentUrl,
		verbose:        verbose,
		logger:         logger.NewLoggerInstance(id, "page"),
		loadEventFired: make(chan any),
		communicator:   make(chan any),
		proxyUser:      proxyUser,
		proxyPass:      proxyPass,
		socketLock:     sync.Mutex{},
		counterLock:    sync.Mutex{},
	}
}

func (p *Page) handleVerbose(msg string) {
	if p.verbose {
		p.logger.Info(msg)
	}
}

// Close closes the Page
func (p *Page) close() error {
	p.handleVerbose(fmt.Sprintf("closing page %s", p.id))
	if p.wsConn != nil {
		return p.wsConn.Close()
	}
	return nil
}

// GetCurrentUrl returns the current URL of the Page
func (p *Page) GetCurrentUrl() string {
	return p.currentUrl
}

// Navigate navigates to a given URL
func (p *Page) Navigate(url string) error {
	p.handleVerbose(fmt.Sprintf("navigating to %s", url))
	command := p.navigateTo(url)
	if err := p.send(command); err != nil {
		return err
	}
	return nil
}

// EnableFetch enables fetch
func (p *Page) EnableFetch() error {
	p.handleVerbose("Enabling fetch")
	command := p.enableFetch()
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Fetch enabled")

	return nil
}

func (p *Page) DisableFetch() error {
	p.handleVerbose("Disabling fetch")
	command := p.disableFetch()
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Fetch disabled")
	return nil
}

// EnableNetwork enables network
func (p *Page) EnableNetwork() error {
	p.handleVerbose("Enabling network")
	command := p.enableNetwork()
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Network enabled")
	return nil
}

// EnablePage enables page
func (p *Page) EnablePage() error {
	p.handleVerbose("Enabling page")
	command := p.enablePage()
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Page enabled")
	return nil
}

// NavigateWithWaitLoad navigates to a given URL and waits for the page to load
func (p *Page) NavigateWithWaitLoad(url string) error {
	p.handleVerbose(fmt.Sprintf("navigating to %s", url))
	command := p.navigateTo(url)
	if err := p.send(command); err != nil {
		if err.Error() != "Invalid InterceptionId." {
			return nil
		}
		return err
	}

	return p.waitForPageLoad()
}

// waitForPageLoad waits for the page to load
func (p *Page) waitForPageReady() error {
	p.handleVerbose("waiting for chrome to be ready")
	for {
		state, err := p.checkReadyState()
		if err != nil {
			return err
		}
		if strings.Contains(state, "complete") {
			p.handleVerbose("chrome is ready")
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForPageLoad waits for the page to load
func (p *Page) waitForPageLoad() error {
	p.handleVerbose("waiting for page to load")
	for {
		select {
		case <-time.After(30 * time.Second):
			return fmt.Errorf("page did not load in time")
		case <-p.loadEventFired:
			p.handleVerbose("page has loded")
			return nil
		}
	}
}

// GetTitle returns the title of the Page
func (p *Page) GetTitle() (string, error) {
	p.handleVerbose("getting title")
	if response, err := p.Evaluate("document.title"); err != nil {
		return "", err
	} else {
		return response.Value, nil
	}
}

// GetContent returns the content of the Page
func (p *Page) GetContent() (string, error) {
	p.handleVerbose("getting content")
	if response, err := p.Evaluate("document.documentElement.outerHTML"); err != nil {
		return "", err
	} else {
		return response.Value, nil
	}
}

// checkReadyState checks if the page is ready
func (p *Page) checkReadyState() (string, error) {
	p.handleVerbose("checking ready state")
	if response, err := p.Evaluate("document.readyState"); err != nil {
		return "", err
	} else {
		return response.Value, nil
	}
}

// Evaluate evaluates a given script and returns the result
func (p *Page) Evaluate(script string) (*CommandResponse, error) {
	command := p.evaluate(script)
	return p.sendAndReceive(command)
}

// GetFrameId returns the frame ID of the Page
func (p *Page) GetFrameId() string {
	return p.frameId
}

// SetRequestPausedHandler sets a callback function to be executed when a request is paused.
func (p *Page) SetRequestPausedHandler(handler func(params map[string]any)) {
	p.handlerLock.Lock()
	defer p.handlerLock.Unlock()
	p.requestPausedHandler = handler
}
