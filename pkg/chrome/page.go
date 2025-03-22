package chrome

import (
	"encoding/json"
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

// Click performs a mouse click at the specified coordinates
func (p *Page) Click(x, y int) error {
	p.handleVerbose(fmt.Sprintf("clicking at coordinates (%d, %d)", x, y))
	command := p.dispatchMouseEvent("mousePressed", x, y)
	if err := p.send(command); err != nil {
		return err
	}

	// Send mouseReleased event to complete the click
	command = p.dispatchMouseEvent("mouseReleased", x, y)
	if err := p.send(command); err != nil {
		return err
	}

	p.handleVerbose("click completed")
	return nil
}

// ClickElement finds and clicks on an element by selector
func (p *Page) ClickElement(selector string) error {
	p.handleVerbose(fmt.Sprintf("clicking element with selector: %s", selector))

	// Get element coordinates
	script := fmt.Sprintf(`
        (function() {
            const element = document.querySelector("%s");
            if (!element) return null;
            
            const rect = element.getBoundingClientRect();
            return {
                x: Math.floor(rect.left + rect.width / 2),
                y: Math.floor(rect.top + rect.height / 2)
            };
        })()
    `, selector)

	response, err := p.Evaluate(script)
	if err != nil {
		return err
	}

	if response.Value == "null" {
		return fmt.Errorf("element not found: %s", selector)
	}

	// Parse coordinates from response
	var coords struct {
		X int `json:"x"`
		Y int `json:"y"`
	}

	if err := json.Unmarshal([]byte(response.Value), &coords); err != nil {
		return fmt.Errorf("failed to parse element coordinates: %w", err)
	}

	// Perform the click
	return p.Click(coords.X, coords.Y)
}

// InputText inputs text into the currently focused element
func (p *Page) InputText(text string) error {
	p.handleVerbose(fmt.Sprintf("inputting text: %s", text))

	for _, char := range text {
		command := p.dispatchKeyEvent("keyDown", string(char))
		if err := p.send(command); err != nil {
			return err
		}

		command = p.dispatchKeyEvent("keyUp", string(char))
		if err := p.send(command); err != nil {
			return err
		}
	}

	p.handleVerbose("text input completed")
	return nil
}

// FocusElement focuses on an element by selector before inputting text
func (p *Page) FocusAndInputText(selector, text string) error {
	p.handleVerbose(fmt.Sprintf("focusing and inputting text to element with selector: %s", selector))

	// Focus the element
	script := fmt.Sprintf(`
        (function() {
            const element = document.querySelector("%s");
            if (!element) return false;
            
            element.focus();
            return true;
        })()
    `, selector)

	response, err := p.Evaluate(script)
	if err != nil {
		return err
	}

	if response.Value != "true" {
		return fmt.Errorf("element not found or cannot be focused: %s", selector)
	}

	// Input the text
	return p.InputText(text)
}

// Helper methods for constructing Chrome DevTools Protocol commands

// dispatchMouseEvent creates a command to dispatch a mouse event
func (p *Page) dispatchMouseEvent(eventType string, x, y int) *Command {
	p.counterLock.Lock()
	defer p.counterLock.Unlock()

	p.messageCounter++
	return &Command{
		Id:     p.messageCounter,
		Method: "Input.dispatchMouseEvent",
		Params: map[string]interface{}{
			"type":       eventType,
			"x":          x,
			"y":          y,
			"button":     "left",
			"clickCount": 1,
		},
	}
}

// dispatchKeyEvent creates a command to dispatch a key event
func (p *Page) dispatchKeyEvent(eventType, text string) *Command {
	p.counterLock.Lock()
	defer p.counterLock.Unlock()

	p.messageCounter++
	params := map[string]interface{}{
		"type": eventType,
	}

	if text != "" {
		params["text"] = text
	}

	return &Command{
		Id:     p.messageCounter,
		Method: "Input.dispatchKeyEvent",
		Params: params,
	}
}

// FindElementByXPath finds an element by XPath and returns its properties
func (p *Page) FindElementByXPath(xpath string) (*CommandResponse, error) {
	p.handleVerbose(fmt.Sprintf("finding element with XPath: %s", xpath))

	script := fmt.Sprintf(`
        (function() {
            const result = document.evaluate(
                "%s", 
                document, 
                null, 
                XPathResult.FIRST_ORDERED_NODE_TYPE, 
                null
            );
            
            if (!result.singleNodeValue) return null;
            
            const element = result.singleNodeValue;
            const rect = element.getBoundingClientRect();
            
            return {
                exists: true,
                tagName: element.tagName,
                id: element.id,
                className: element.className,
                textContent: element.textContent,
                isVisible: !(element.offsetParent === null),
                position: {
                    x: Math.floor(rect.left + rect.width / 2),
                    y: Math.floor(rect.top + rect.height / 2)
                },
                dimensions: {
                    width: rect.width,
                    height: rect.height
                }
            };
        })()
    `, strings.ReplaceAll(xpath, `"`, `\"`))

	return p.Evaluate(script)
}

// ClickElementByXPath finds and clicks on an element by XPath
func (p *Page) ClickElementByXPath(xpath string) error {
	p.handleVerbose(fmt.Sprintf("clicking element with XPath: %s", xpath))

	response, err := p.FindElementByXPath(xpath)
	if err != nil {
		return err
	}

	if response.Value == "null" {
		return fmt.Errorf("element not found with XPath: %s", xpath)
	}

	// Parse element data from response
	var elementData struct {
		Exists   bool `json:"exists"`
		Position struct {
			X int `json:"x"`
			Y int `json:"y"`
		} `json:"position"`
	}

	if err := json.Unmarshal([]byte(response.Value), &elementData); err != nil {
		return fmt.Errorf("failed to parse element data: %w", err)
	}

	if !elementData.Exists {
		return fmt.Errorf("element not found with XPath: %s", xpath)
	}

	// Perform the click
	return p.Click(elementData.Position.X, elementData.Position.Y)
}

// FocusAndInputTextByXPath focuses on an element by XPath and inputs text
func (p *Page) FocusAndInputTextByXPath(xpath, text string) error {
	p.handleVerbose(fmt.Sprintf("focusing and inputting text to element with XPath: %s", xpath))

	// Focus the element
	script := fmt.Sprintf(`
        (function() {
            const result = document.evaluate(
                "%s", 
                document, 
                null, 
                XPathResult.FIRST_ORDERED_NODE_TYPE, 
                null
            );
            
            if (!result.singleNodeValue) return false;
            
            const element = result.singleNodeValue;
            element.focus();
            return true;
        })()
    `, strings.ReplaceAll(xpath, `"`, `\"`))

	response, err := p.Evaluate(script)
	if err != nil {
		return err
	}

	if response.Value != "true" {
		return fmt.Errorf("element not found or cannot be focused with XPath: %s", xpath)
	}

	// Input the text
	return p.InputText(text)
}

// GetElementText gets the text content of an element by XPath
func (p *Page) GetElementTextByXPath(xpath string) (string, error) {
	p.handleVerbose(fmt.Sprintf("getting text from element with XPath: %s", xpath))

	response, err := p.FindElementByXPath(xpath)
	if err != nil {
		return "", err
	}

	if response.Value == "null" {
		return "", fmt.Errorf("element not found with XPath: %s", xpath)
	}

	// Parse element data from response
	var elementData struct {
		TextContent string `json:"textContent"`
	}

	if err := json.Unmarshal([]byte(response.Value), &elementData); err != nil {
		return "", fmt.Errorf("failed to parse element data: %w", err)
	}

	return elementData.TextContent, nil
}

// GetElementAttribute gets an attribute value of an element by XPath
func (p *Page) GetElementAttributeByXPath(xpath, attributeName string) (string, error) {
	p.handleVerbose(fmt.Sprintf("getting attribute '%s' from element with XPath: %s", attributeName, xpath))

	script := fmt.Sprintf(`
        (function() {
            const result = document.evaluate(
                "%s", 
                document, 
                null, 
                XPathResult.FIRST_ORDERED_NODE_TYPE, 
                null
            );
            
            if (!result.singleNodeValue) return null;
            
            const element = result.singleNodeValue;
            return element.getAttribute("%s");
        })()
    `, strings.ReplaceAll(xpath, `"`, `\"`), attributeName)

	response, err := p.Evaluate(script)
	if err != nil {
		return "", err
	}

	if response.Value == "null" {
		return "", fmt.Errorf("element not found with XPath: %s or attribute '%s' not found", xpath, attributeName)
	}

	// Remove quotes from the value
	value := response.Value
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}

	return value, nil
}

// FindElementsByXPath finds all elements matching an XPath and returns their properties
func (p *Page) FindElementsByXPath(xpath string) (*CommandResponse, error) {
	p.handleVerbose(fmt.Sprintf("finding elements with XPath: %s", xpath))

	script := fmt.Sprintf(`
        (function() {
            const result = document.evaluate(
                "%s", 
                document, 
                null, 
                XPathResult.ORDERED_NODE_SNAPSHOT_TYPE, 
                null
            );
            
            if (result.snapshotLength === 0) return [];
            
            const elements = [];
            for (let i = 0; i < result.snapshotLength; i++) {
                const element = result.snapshotItem(i);
                const rect = element.getBoundingClientRect();
                
                elements.push({
                    index: i,
                    tagName: element.tagName,
                    id: element.id,
                    className: element.className,
                    textContent: element.textContent,
                    isVisible: !(element.offsetParent === null),
                    position: {
                        x: Math.floor(rect.left + rect.width / 2),
                        y: Math.floor(rect.top + rect.height / 2)
                    },
                    dimensions: {
                        width: rect.width,
                        height: rect.height
                    }
                });
            }
            
            return elements;
        })()
    `, strings.ReplaceAll(xpath, `"`, `\"`))

	return p.Evaluate(script)
}

// WaitForElementByXPath waits for an element to appear with a given XPath
func (p *Page) WaitForElementByXPath(xpath string, timeout time.Duration) error {
	p.handleVerbose(fmt.Sprintf("waiting for element with XPath: %s", xpath))

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		response, err := p.FindElementByXPath(xpath)
		if err != nil {
			return err
		}

		if response.Value != "null" {
			var elementData struct {
				Exists bool `json:"exists"`
			}

			if err := json.Unmarshal([]byte(response.Value), &elementData); err != nil {
				return fmt.Errorf("failed to parse element data: %w", err)
			}

			if elementData.Exists {
				p.handleVerbose(fmt.Sprintf("element with XPath: %s found", xpath))
				return nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for element with XPath: %s", xpath)
}

// FindElementBySelector finds an element by CSS selector and returns its properties
func (p *Page) FindElementBySelector(selector string) (*CommandResponse, error) {
	p.handleVerbose(fmt.Sprintf("finding element with selector: %s", selector))

	script := fmt.Sprintf(`
        (function() {
            const element = document.querySelector("%s");
            if (!element) return null;
            
            const rect = element.getBoundingClientRect();
            
            return {
                exists: true,
                tagName: element.tagName,
                id: element.id,
                className: element.className,
                textContent: element.textContent,
                isVisible: !(element.offsetParent === null),
                position: {
                    x: Math.floor(rect.left + rect.width / 2),
                    y: Math.floor(rect.top + rect.height / 2)
                },
                dimensions: {
                    width: rect.width,
                    height: rect.height
                }
            };
        })()
    `, selector)

	return p.Evaluate(script)
}

// GetElementTextBySelector gets the text content of an element by CSS selector
func (p *Page) GetElementTextBySelector(selector string) (string, error) {
	p.handleVerbose(fmt.Sprintf("getting text from element with selector: %s", selector))

	response, err := p.FindElementBySelector(selector)
	if err != nil {
		return "", err
	}

	if response.Value == "null" {
		return "", fmt.Errorf("element not found with selector: %s", selector)
	}

	// Parse element data from response
	var elementData struct {
		TextContent string `json:"textContent"`
	}

	if err := json.Unmarshal([]byte(response.Value), &elementData); err != nil {
		return "", fmt.Errorf("failed to parse element data: %w", err)
	}

	return elementData.TextContent, nil
}

// GetElementAttributeBySelector gets an attribute value of an element by CSS selector
func (p *Page) GetElementAttributeBySelector(selector, attributeName string) (string, error) {
	p.handleVerbose(fmt.Sprintf("getting attribute '%s' from element with selector: %s", attributeName, selector))

	script := fmt.Sprintf(`
        (function() {
            const element = document.querySelector("%s");
            if (!element) return null;
            
            return element.getAttribute("%s");
        })()
    `, selector, attributeName)

	response, err := p.Evaluate(script)
	if err != nil {
		return "", err
	}

	if response.Value == "null" {
		return "", fmt.Errorf("element not found with selector: %s or attribute '%s' not found", selector, attributeName)
	}

	// Remove quotes from the value
	value := response.Value
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}

	return value, nil
}

// FindElementsBySelector finds all elements matching a CSS selector and returns their properties
func (p *Page) FindElementsBySelector(selector string) (*CommandResponse, error) {
	p.handleVerbose(fmt.Sprintf("finding elements with selector: %s", selector))

	script := fmt.Sprintf(`
        (function() {
            const elements = Array.from(document.querySelectorAll("%s"));
            if (elements.length === 0) return [];
            
            return elements.map((element, index) => {
                const rect = element.getBoundingClientRect();
                
                return {
                    index: index,
                    tagName: element.tagName,
                    id: element.id,
                    className: element.className,
                    textContent: element.textContent,
                    isVisible: !(element.offsetParent === null),
                    position: {
                        x: Math.floor(rect.left + rect.width / 2),
                        y: Math.floor(rect.top + rect.height / 2)
                    },
                    dimensions: {
                        width: rect.width,
                        height: rect.height
                    }
                };
            });
        })()
    `, selector)

	return p.Evaluate(script)
}

// WaitForElementBySelector waits for an element to appear with a given CSS selector
func (p *Page) WaitForElementBySelector(selector string, timeout time.Duration) error {
	p.handleVerbose(fmt.Sprintf("waiting for element with selector: %s", selector))

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		response, err := p.FindElementBySelector(selector)
		if err != nil {
			return err
		}

		if response.Value != "null" {
			var elementData struct {
				Exists bool `json:"exists"`
			}

			if err := json.Unmarshal([]byte(response.Value), &elementData); err != nil {
				return fmt.Errorf("failed to parse element data: %w", err)
			}

			if elementData.Exists {
				p.handleVerbose(fmt.Sprintf("element with selector: %s found", selector))
				return nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for element with selector: %s", selector)
}
