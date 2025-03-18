package chrome

import (
	"fmt"
	"log"
	"strings"
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
	communicator    chan any
	proxyUser       string
	proxyPass       string
}

// newPage creates a new Page
func newPage(id string, wsUrl string, currentUrl string, verbose bool, proxyUser string, proxyPass string) *Page {
	return &Page{
		id:           id,
		wsUrl:        wsUrl,
		currentUrl:   currentUrl,
		verbose:      verbose,
		logger:       logger.NewLoggerInstance(id, "page"),
		communicator: make(chan any),
		proxyUser:    proxyUser,
		proxyPass:    proxyPass,
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
	if err := p.send(command); err != nil {
		return err
	}
	return nil
}

// EnableFetch enables fetch
func (p *Page) EnableFetch() error {
	if p.verbose {
		p.logger.Info("Enabling fetch")
	}
	command := p.enableFetch()
	if err := p.send(command); err != nil {
		if p.verbose {
			p.logger.Error(fmt.Sprintf("error enabling fetch: %s", err), err)
		}
		return err
	}
	if p.verbose {
		p.logger.Info("Fetch enabled")
	}

	return nil
}

// EnableNetwork enables network
func (p *Page) EnableNetwork() error {
	if p.verbose {
		p.logger.Info("Enabling network")
	}
	command := p.enableNetwork()
	if err := p.send(command); err != nil {
		if p.verbose {
			p.logger.Error(fmt.Sprintf("error enabling network: %s", err), err)
		}
		return err
	}
	if p.verbose {
		p.logger.Info("Network enabled")
	}
	return nil
}

// EnablePage enables page
func (p *Page) EnablePage() error {
	if p.verbose {
		p.logger.Info("Enabling page")
	}
	command := p.enablePage()
	if err := p.send(command); err != nil {
		if p.verbose {
			p.logger.Error(fmt.Sprintf("error enabling page: %s", err), err)
		}
		return err
	}
	if p.verbose {
		p.logger.Info("Page enabled")
	}
	return nil
}

// NavigateWithWaitLoad navigates to a given URL and waits for the page to load
func (p *Page) NavigateWithWaitLoad(url string) error {
	if p.verbose {
		p.logger.Info(fmt.Sprintf("navigating to %s", url))
	}

	command := p.navigateTo(url)
	if err := p.send(command); err != nil {
		return err
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
