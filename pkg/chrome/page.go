package chrome

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/internal/logger"

	"github.com/gorilla/websocket"
)

// RequestInterceptor represents a request interceptor configuration
// It manages request interception patterns and paused requests
type RequestInterceptor struct {
	patterns        []map[string]any
	isEnabled       bool
	pausedRequests  map[string]map[string]any
	pausedResponses map[string]map[string]any
	pausedLock      sync.RWMutex
	responseHandler func(params map[string]any)
}

// Page represents a single page in the browser
type Page struct {
	id                   string
	wsUrl                string
	currentUrl           string
	wsConn               *websocket.Conn
	verbose              bool
	logger               *logger.LoggerInstance
	messageCounter       int
	proxyIdentifier      int
	frameId              string
	loadEventFired       chan any
	communicator         chan any
	proxyUser            string
	proxyPass            string
	socketLock           sync.Mutex
	counterLock          sync.Mutex
	ctx                  context.Context
	requestPausedHandler func(params map[string]any)
	handlerLock          sync.Mutex
	interceptor          *RequestInterceptor
}

// newPage creates a new Page
func newPage(id string, wsUrl string, currentUrl string, verbose bool, proxyUser string, proxyPass string) *Page {
	return &Page{
		id:             id,
		ctx:            context.Background(),
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
		interceptor: &RequestInterceptor{
			patterns:        make([]map[string]any, 0),
			isEnabled:       false,
			pausedRequests:  make(map[string]map[string]any),
			pausedResponses: make(map[string]map[string]any),
			pausedLock:      sync.RWMutex{},
		},
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

func (p *Page) EnableFeatures() error {
	if err := p.EnableFetch(); err != nil {
		return err
	}

	if err := p.EnableNetwork(); err != nil {
		return err
	}

	if err := p.EnablePage(); err != nil {
		return err
	}
	return nil
}

// GetCurrentUrl returns the current URL of the Page
func (p *Page) WithContext(ctx context.Context) {
	p.ctx = ctx
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

	return p.WaitForPageLoad(60 * time.Second)
}

// WaitForPageLoad waits for the page to load
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

// WaitForPageLoad waits for the page to load
func (p *Page) WaitForPageLoad(t time.Duration) error {
	p.handleVerbose("waiting for page to load")
	for {
		select {
		case <-time.After(t):
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
		return response.StringValue(), nil
	}
}

// GetContent returns the content of the Page
func (p *Page) GetContent() (string, error) {
	p.handleVerbose("getting content")
	if response, err := p.Evaluate("document.documentElement.outerHTML"); err != nil {
		return "", err
	} else {
		return response.StringValue(), nil
	}
}

// checkReadyState checks if the page is ready
func (p *Page) checkReadyState() (string, error) {
	p.handleVerbose("checking ready state")
	if response, err := p.Evaluate("document.readyState"); err != nil {
		return "", err
	} else {
		return response.StringValue(), nil
	}
}

// Evaluate evaluates a given script and returns the result
func (p *Page) Evaluate(script string) (*CommandResponse, error) {
	command := p.evaluate(script)
	return p.sendAndReceive(command)
}

// Evaluate evaluates a given script and returns the result
func (p *Page) EvaluateAsync(script string) (*CommandResponse, error) {
	command := p.evaluateAsync(script)
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

// EnableRequestInterception enables request interception with the specified patterns
// patterns: Array of request patterns to intercept. Each pattern can specify:
//   - urlPattern: URL pattern to match (e.g., "*", "*.example.com", "https://api.example.com/*")
//   - requestStage: When to intercept ("Request" or "Response")
//   - resourceType: Optional resource type filter (e.g., "Document", "Script", "Stylesheet")
func (p *Page) EnableRequestInterception(patterns []map[string]any) error {
	p.handleVerbose("Enabling request interception")

	p.interceptor.pausedLock.Lock()
	p.interceptor.patterns = patterns
	p.interceptor.isEnabled = true
	p.interceptor.pausedLock.Unlock()

	command := p.enableFetchWithPatterns(patterns)
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Request interception enabled")
	return nil
}

// DisableRequestInterception disables request interception
func (p *Page) DisableRequestInterception() error {
	p.handleVerbose("Disabling request interception")

	p.interceptor.pausedLock.Lock()
	p.interceptor.isEnabled = false
	p.interceptor.pausedRequests = make(map[string]map[string]any)
	p.interceptor.pausedResponses = make(map[string]map[string]any)
	p.interceptor.pausedLock.Unlock()

	command := p.disableFetch()
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Request interception disabled")
	return nil
}

// PauseRequest pauses a specific request by its ID
func (p *Page) PauseRequest(requestID string) {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Store the request ID as paused (the actual request data will be stored when it's intercepted)
	p.interceptor.pausedRequests[requestID] = make(map[string]any)
	p.handleVerbose(fmt.Sprintf("Request %s marked for pausing", requestID))
}

// ResumeRequest resumes a paused request with optional modifications
// requestID: The ID of the request to resume
// modifications: Optional map containing modifications to apply:
//   - url: New URL to redirect the request to
//   - method: HTTP method to use (GET, POST, etc.)
//   - headers: Map of headers to add/modify
//   - body: Request body content
func (p *Page) ResumeRequest(requestID string, modifications ...map[string]any) error {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Remove from paused requests
	delete(p.interceptor.pausedRequests, requestID)

	// Continue the request with optional modifications
	return p.continueRequestWithModifications(requestID, modifications...)
}

// GetPausedRequests returns a list of currently paused request IDs
func (p *Page) GetPausedRequests() []string {
	p.interceptor.pausedLock.RLock()
	defer p.interceptor.pausedLock.RUnlock()

	requestIDs := make([]string, 0, len(p.interceptor.pausedRequests))
	for requestID := range p.interceptor.pausedRequests {
		requestIDs = append(requestIDs, requestID)
	}
	return requestIDs
}

// IsRequestPaused checks if a request is currently paused
func (p *Page) IsRequestPaused(requestID string) bool {
	p.interceptor.pausedLock.RLock()
	defer p.interceptor.pausedLock.RUnlock()

	_, exists := p.interceptor.pausedRequests[requestID]
	return exists
}

// SetRequestInterceptorPatterns sets new patterns for request interception
func (p *Page) SetRequestInterceptorPatterns(patterns []map[string]any) error {
	if !p.interceptor.isEnabled {
		return fmt.Errorf("request interception is not enabled")
	}

	p.interceptor.pausedLock.Lock()
	p.interceptor.patterns = patterns
	p.interceptor.pausedLock.Unlock()

	// Update the fetch patterns
	command := p.enableFetchWithPatterns(patterns)
	return p.send(command)
}

// GetPausedRequestData returns the request data for a paused request
func (p *Page) GetPausedRequestData(requestID string) (map[string]any, bool) {
	p.interceptor.pausedLock.RLock()
	defer p.interceptor.pausedLock.RUnlock()

	if pausedData, exists := p.interceptor.pausedRequests[requestID]; exists {
		if requestData, ok := pausedData["requestData"].(map[string]any); ok {
			return requestData, true
		}
	}
	return nil, false
}

// FailRequest fails a paused request with an error reason
func (p *Page) FailRequest(requestID string, errorReason string) error {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Remove from paused requests
	delete(p.interceptor.pausedRequests, requestID)

	command := p.failRequest(requestID, errorReason)
	return p.send(command)
}

// FulfillRequest fulfills a paused request with a custom response
func (p *Page) FulfillRequest(requestID string, responseCode int, responseHeaders []map[string]string, body string) error {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Remove from paused requests
	delete(p.interceptor.pausedRequests, requestID)

	command := p.fulfillRequest(requestID, responseCode, responseHeaders, body)
	return p.send(command)
}

// CreateRequestPattern creates a request pattern for interception
func CreateRequestPattern(urlPattern string, requestStage string) map[string]any {
	return map[string]any{
		"urlPattern":   urlPattern,
		"requestStage": requestStage,
	}
}

// CreateRequestPatternWithResourceType creates a request pattern with resource type filtering
func CreateRequestPatternWithResourceType(urlPattern string, requestStage string, resourceType string) map[string]any {
	return map[string]any{
		"urlPattern":   urlPattern,
		"requestStage": requestStage,
		"resourceType": resourceType,
	}
}

// EnableResponseInterception enables response interception with the specified patterns
// patterns: Array of response patterns to intercept. Each pattern can specify:
//   - urlPattern: URL pattern to match (e.g., "*", "*.example.com", "https://api.example.com/*")
//   - requestStage: Should be "Response" for response interception
//   - resourceType: Optional resource type filter (e.g., "Document", "Script", "Stylesheet")
func (p *Page) EnableResponseInterception(patterns []map[string]any) error {
	p.handleVerbose("Enabling response interception")

	p.interceptor.pausedLock.Lock()
	p.interceptor.patterns = append(p.interceptor.patterns, patterns...)
	p.interceptor.isEnabled = true
	p.interceptor.pausedLock.Unlock()

	command := p.enableFetchWithPatterns(p.interceptor.patterns)
	if err := p.send(command); err != nil {
		return err
	}
	p.handleVerbose("Response interception enabled")
	return nil
}

// SetResponsePausedHandler sets a callback function to be executed when a response is paused
func (p *Page) SetResponsePausedHandler(handler func(params map[string]any)) {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()
	p.interceptor.responseHandler = handler
}

// PauseResponse pauses a specific response by its request ID
func (p *Page) PauseResponse(requestID string) {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Store the request ID as paused for response
	p.interceptor.pausedResponses[requestID] = make(map[string]any)
	p.handleVerbose(fmt.Sprintf("Response for request %s marked for pausing", requestID))
}

// ResumeResponse resumes a paused response
func (p *Page) ResumeResponse(requestID string) error {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Remove from paused responses
	delete(p.interceptor.pausedResponses, requestID)

	// Continue the response
	command := p.continueRequest(requestID)
	return p.send(command)
}

// GetPausedResponses returns a list of currently paused response request IDs
func (p *Page) GetPausedResponses() []string {
	p.interceptor.pausedLock.RLock()
	defer p.interceptor.pausedLock.RUnlock()

	requestIDs := make([]string, 0, len(p.interceptor.pausedResponses))
	for requestID := range p.interceptor.pausedResponses {
		requestIDs = append(requestIDs, requestID)
	}
	return requestIDs
}

// IsResponsePaused checks if a response is currently paused
func (p *Page) IsResponsePaused(requestID string) bool {
	p.interceptor.pausedLock.RLock()
	defer p.interceptor.pausedLock.RUnlock()

	_, exists := p.interceptor.pausedResponses[requestID]
	return exists
}

// GetPausedResponseData returns the response data for a paused response
func (p *Page) GetPausedResponseData(requestID string) (map[string]any, bool) {
	p.interceptor.pausedLock.RLock()
	defer p.interceptor.pausedLock.RUnlock()

	if pausedData, exists := p.interceptor.pausedResponses[requestID]; exists {
		if responseData, ok := pausedData["responseData"].(map[string]any); ok {
			return responseData, true
		}
	}
	return nil, false
}

// FulfillResponse fulfills a paused response with a custom response
func (p *Page) FulfillResponse(requestID string, responseCode int, responseHeaders []map[string]string, body string) error {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Remove from paused responses
	delete(p.interceptor.pausedResponses, requestID)

	command := p.fulfillRequest(requestID, responseCode, responseHeaders, body)
	return p.send(command)
}

// FailResponse fails a paused response with an error reason
func (p *Page) FailResponse(requestID string, errorReason string) error {
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()

	// Remove from paused responses
	delete(p.interceptor.pausedResponses, requestID)

	command := p.failRequest(requestID, errorReason)
	return p.send(command)
}

// CreateResponsePattern creates a response pattern for interception
func CreateResponsePattern(urlPattern string) map[string]any {
	return map[string]any{
		"urlPattern":   urlPattern,
		"requestStage": "Response",
	}
}

// CreateResponsePatternWithResourceType creates a response pattern with resource type filtering
func CreateResponsePatternWithResourceType(urlPattern string, resourceType string) map[string]any {
	return map[string]any{
		"urlPattern":   urlPattern,
		"requestStage": "Response",
		"resourceType": resourceType,
	}
}
