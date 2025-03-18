package chrome

import (
	"fmt"
	"golang-remote-chrome/internal/logger"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Page represents a single page in the browser
type Page struct {
	id             string
	wsUrl          string
	currentUrl     string
	wsConn         *websocket.Conn
	verbose        bool
	logger         *logger.LoggerInstance
	messageCounter int
	frameId        string
}

// newPage creates a new Page
func newPage(id string, wsUrl string, currentUrl string, wsConn *websocket.Conn, verbose bool) *Page {
	return &Page{
		id:         id,
		wsUrl:      wsUrl,
		currentUrl: currentUrl,
		wsConn:     wsConn,
		verbose:    verbose,
		logger:     logger.NewLoggerInstance(id, "page"),
	}
}

// Close closes the Page
func (p *Page) close() error {
	if p.verbose {
		p.logger.Info(fmt.Sprintf("closing page %s", p.id))
	}
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
	if p.verbose {
		p.logger.Info(fmt.Sprintf("navigating to %s", url))
	}
	command := p.navigateTo(url)
	if response, err := p.sendAndReceive(command); err != nil {
		return err
	} else {
		if p.verbose {
			p.logger.Info(fmt.Sprintf("navigated to %s: Response: %s", url, response))
		}
	}
	return nil
}

// NavigateWithWaitLoad navigates to a given URL and waits for the page to load
func (p *Page) NavigateWithWaitLoad(url string) error {
	if p.verbose {
		p.logger.Info(fmt.Sprintf("navigating to %s", url))
	}

	command := p.navigateTo(url)
	if response, err := p.sendAndReceive(command); err != nil {
		return err
	} else {
		if p.verbose {
			p.logger.Info(fmt.Sprintf("navigated to %s: Response: %s", url, response))
		}
	}
	return p.waitForPageLoad()
}

// waitForPageLoad waits for the page to load
func (p *Page) waitForPageLoad() error {
	if p.verbose {
		p.logger.Info(fmt.Sprintf("waiting for page to load"))
	}
	for {
		state, err := p.checkReadyState()
		if err != nil {
			return err
		}
		log.Println(state)
		if strings.Contains(state, "complete") {
			if p.verbose {
				p.logger.Info(fmt.Sprintf("page loaded"))
			}
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

// GetTitle returns the title of the Page
func (p *Page) GetTitle() (string, error) {
	if response, err := p.Evaluate("document.title"); err != nil {
		return "", err
	} else {
		return response.Value, nil
	}
}

// GetContent returns the content of the Page
func (p *Page) GetContent() (string, error) {
	if response, err := p.Evaluate("document.documentElement.outerHTML"); err != nil {
		return "", err
	} else {
		return response.Value, nil
	}
}

// checkReadyState checks if the page is ready
func (p *Page) checkReadyState() (string, error) {
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
