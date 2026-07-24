package chrome

import (
	"context"
	"encoding/base64"
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
	ctx                    context.Context
	requestPausedHandler   func(params map[string]any)
	requestAbortHandler    func(params map[string]any) bool
	networkRequestHandler  func(params map[string]any)
	networkResponseHandler func(params map[string]any)
	handlerLock            sync.Mutex
	interceptor            *RequestInterceptor
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

// SetRequestAbortHandler sets a synchronous callback evaluated for every paused
// request. If it returns true, the request is failed (aborted) instead of being
// continued. This runs before the auto-continue, so it can reliably stop a
// request after reading its (already SDK-augmented) headers.
func (p *Page) SetRequestAbortHandler(handler func(params map[string]any) bool) {
	p.handlerLock.Lock()
	defer p.handlerLock.Unlock()
	p.requestAbortHandler = handler
}

// SetNetworkRequestHandler sets a callback for Network.requestWillBeSent events.
func (p *Page) SetNetworkRequestHandler(handler func(params map[string]any)) {
	p.handlerLock.Lock()
	defer p.handlerLock.Unlock()
	p.networkRequestHandler = handler
}

// SetNetworkResponseHandler sets a callback for Network.responseReceived events.
func (p *Page) SetNetworkResponseHandler(handler func(params map[string]any)) {
	p.handlerLock.Lock()
	defer p.handlerLock.Unlock()
	p.networkResponseHandler = handler
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
	p.handleVerbose("Setting response paused handler")
	p.interceptor.pausedLock.Lock()
	defer p.interceptor.pausedLock.Unlock()
	p.interceptor.responseHandler = handler
	p.handleVerbose("Response paused handler set successfully")
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

// EnableResponseLogging enables response logging without pausing responses
// This is a convenience method that sets up response interception and logging
func (p *Page) EnableResponseLogging(patterns []map[string]any) error {
	p.handleVerbose("Enabling response logging")

	// Enable response interception
	err := p.EnableResponseInterception(patterns)
	if err != nil {
		return err
	}

	// Set up a default logging handler
	p.SetResponsePausedHandler(func(params map[string]any) {
		requestID := params["requestId"].(string)
		response := params["response"].(map[string]any)
		url := response["url"].(string)
		status := response["status"].(float64)
		method := response["method"].(string)

		// Log response details
		p.handleVerbose(fmt.Sprintf("Response: %s %s -> %s (Status: %.0f)", method, url, requestID, status))

		// Log response headers if available
		if headers, ok := response["headers"].(map[string]any); ok {
			p.handleVerbose(fmt.Sprintf("Response headers: %+v", headers))
		}

		// Log response body size if available
		if bodySize, ok := response["bodySize"].(float64); ok {
			p.handleVerbose(fmt.Sprintf("Response body size: %.0f bytes", bodySize))
		}
	})

	p.handleVerbose("Response logging enabled")
	return nil
}

// EnableAllResponseLogging enables logging for all responses
func (p *Page) EnableAllResponseLogging() error {
	patterns := []map[string]any{
		CreateResponsePattern("*"),
	}
	return p.EnableResponseLogging(patterns)
}

// EnableResponseLoggingWithHandler enables response logging with a custom handler
// The handler will be called for each response but responses won't be paused unless you call PauseResponse()
func (p *Page) EnableResponseLoggingWithHandler(patterns []map[string]any, handler func(params map[string]any)) error {
	p.handleVerbose("Enabling response logging with custom handler")

	// Enable response interception
	err := p.EnableResponseInterception(patterns)
	if err != nil {
		return err
	}

	// Set up the custom handler
	p.SetResponsePausedHandler(handler)

	p.handleVerbose("Response logging with custom handler enabled")
	return nil
}

// LogResponseData is a helper function to extract and log response data
func LogResponseData(params map[string]any) map[string]any {
	response := params["response"].(map[string]any)
	requestID := params["requestId"].(string)

	logData := map[string]any{
		"requestId": requestID,
		"url":       response["url"],
		"method":    response["method"],
		"status":    response["status"],
		"headers":   response["headers"],
	}

	// Add body size if available
	if bodySize, ok := response["bodySize"].(float64); ok {
		logData["bodySize"] = bodySize
	}

	// Add mime type if available
	if mimeType, ok := response["mimeType"].(string); ok {
		logData["mimeType"] = mimeType
	}

	return logData
}

// ResponseBody represents the response body data
type ResponseBody struct {
	Body          string `json:"body"`
	Base64Encoded bool   `json:"base64Encoded"`
}

// GetResponseBody gets the response body for a request ID
// This works for requests that have been intercepted and are currently paused
func (p *Page) GetResponseBody(requestID string) (*ResponseBody, error) {
	p.handleVerbose(fmt.Sprintf("Getting response body for request %s", requestID))

	command := p.getResponseBody(requestID)
	response, err := p.sendAndReceive(command)
	if err != nil {
		return nil, fmt.Errorf("failed to get response body: %v", err)
	}

	// Parse the response
	if response.Value == nil {
		return nil, fmt.Errorf("no response body data received")
	}

	result, ok := response.Value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	body := &ResponseBody{}
	if bodyStr, exists := result["body"]; exists {
		body.Body = bodyStr.(string)
	}
	if base64Encoded, exists := result["base64Encoded"]; exists {
		body.Base64Encoded = base64Encoded.(bool)
	}

	return body, nil
}

// GetResponseBodyWithRetry gets the response body with retry mechanism
// It waits for the response to be complete before attempting to get the body
func (p *Page) GetResponseBodyWithRetry(requestID string, maxRetries int, retryDelay time.Duration) (*ResponseBody, error) {
	p.handleVerbose(fmt.Sprintf("Getting response body for request %s with retry", requestID))

	var lastErr error
	for i := range maxRetries {
		body, err := p.GetResponseBody(requestID)
		if err == nil {
			return body, nil
		}

		lastErr = err

		// Check if it's an Invalid InterceptionId error - don't retry these
		if strings.Contains(err.Error(), "Invalid InterceptionId") {
			return nil, fmt.Errorf("request %s is no longer valid (already completed or resumed): %v", requestID, err)
		}

		p.handleVerbose(fmt.Sprintf("Attempt %d failed: %v, retrying in %v", i+1, err, retryDelay))

		if i < maxRetries-1 { // Don't sleep on the last attempt
			time.Sleep(retryDelay)
		}
	}

	return nil, fmt.Errorf("failed to get response body after %d attempts: %v", maxRetries, lastErr)
}

// GetResponseBodyAsStringWithRetry gets the response body as string with retry mechanism
func (p *Page) GetResponseBodyAsStringWithRetry(requestID string, maxRetries int, retryDelay time.Duration) (string, error) {
	responseBody, err := p.GetResponseBodyWithRetry(requestID, maxRetries, retryDelay)
	if err != nil {
		return "", err
	}

	if responseBody.Base64Encoded {
		// Decode base64 if needed
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return string(decoded), nil
	}

	return responseBody.Body, nil
}

// GetResponseBodyAsBytesWithRetry gets the response body as bytes with retry mechanism
func (p *Page) GetResponseBodyAsBytesWithRetry(requestID string, maxRetries int, retryDelay time.Duration) ([]byte, error) {
	responseBody, err := p.GetResponseBodyWithRetry(requestID, maxRetries, retryDelay)
	if err != nil {
		return nil, err
	}

	if responseBody.Base64Encoded {
		// Decode base64 if needed
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return decoded, nil
	}

	return []byte(responseBody.Body), nil
}

// GetResponseBodyImmediately gets response body immediately when response is intercepted
// This should be called right after PauseResponse() and before ResumeResponse()
func (p *Page) GetResponseBodyImmediately(requestID string) (*ResponseBody, error) {
	p.handleVerbose(fmt.Sprintf("Getting response body immediately for request %s", requestID))

	// Try to get the body immediately without waiting
	body, err := p.GetResponseBody(requestID)
	if err != nil {
		// If it fails with "headers not received", wait a bit and try again
		if strings.Contains(err.Error(), "headers received") {
			p.handleVerbose("Headers not ready, waiting briefly...")
			time.Sleep(100 * time.Millisecond)
			return p.GetResponseBody(requestID)
		}
		return nil, err
	}

	return body, nil
}

// GetResponseBodyWithWait gets response body with proper waiting for headers
// This is the most reliable method for getting response bodies
func (p *Page) GetResponseBodyWithWait(requestID string, maxWaitTime time.Duration) (*ResponseBody, error) {
	p.handleVerbose(fmt.Sprintf("Getting response body with wait for request %s", requestID))

	startTime := time.Now()
	var lastErr error

	for time.Since(startTime) < maxWaitTime {
		body, err := p.GetResponseBody(requestID)
		if err == nil {
			return body, nil
		}

		lastErr = err

		// Check if it's an Invalid InterceptionId error - don't retry these
		if strings.Contains(err.Error(), "Invalid InterceptionId") {
			return nil, fmt.Errorf("request %s is no longer valid (already completed or resumed): %v", requestID, err)
		}

		// If headers not ready, wait a bit and try again
		if strings.Contains(err.Error(), "headers received") {
			p.handleVerbose("Headers not ready, waiting 50ms...")
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// For other errors, wait a bit longer
		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("failed to get response body after waiting %v: %v", maxWaitTime, lastErr)
}

// GetResponseBodyAsStringImmediately gets response body as string immediately
func (p *Page) GetResponseBodyAsStringImmediately(requestID string) (string, error) {
	responseBody, err := p.GetResponseBodyImmediately(requestID)
	if err != nil {
		return "", err
	}

	if responseBody.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return string(decoded), nil
	}

	return responseBody.Body, nil
}

// GetResponseBodyAsBytesImmediately gets response body as bytes immediately
func (p *Page) GetResponseBodyAsBytesImmediately(requestID string) ([]byte, error) {
	responseBody, err := p.GetResponseBodyImmediately(requestID)
	if err != nil {
		return nil, err
	}

	if responseBody.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return decoded, nil
	}

	return []byte(responseBody.Body), nil
}

// GetResponseBodyAsStringWithWait gets response body as string with proper waiting
func (p *Page) GetResponseBodyAsStringWithWait(requestID string, maxWaitTime time.Duration) (string, error) {
	responseBody, err := p.GetResponseBodyWithWait(requestID, maxWaitTime)
	if err != nil {
		return "", err
	}

	if responseBody.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return string(decoded), nil
	}

	return responseBody.Body, nil
}

// GetResponseBodyAsBytesWithWait gets response body as bytes with proper waiting
func (p *Page) GetResponseBodyAsBytesWithWait(requestID string, maxWaitTime time.Duration) ([]byte, error) {
	responseBody, err := p.GetResponseBodyWithWait(requestID, maxWaitTime)
	if err != nil {
		return nil, err
	}

	if responseBody.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return decoded, nil
	}

	return []byte(responseBody.Body), nil
}

// WaitForResponseComplete waits for a response to be completely loaded
// This is useful before trying to get response body
func (p *Page) WaitForResponseComplete(requestID string, timeout time.Duration) error {
	p.handleVerbose(fmt.Sprintf("Waiting for response %s to complete", requestID))

	// Set up a channel to receive completion signal
	done := make(chan bool, 1)

	// Set up a temporary handler to listen for response completion
	originalHandler := p.interceptor.responseHandler

	p.interceptor.pausedLock.Lock()
	p.interceptor.responseHandler = func(params map[string]any) {
		// Call original handler if it exists
		if originalHandler != nil {
			go originalHandler(params)
		}

		// Check if this is the response we're waiting for
		if params["requestId"] == requestID {
			response := params["response"].(map[string]any)
			// Check if response is complete (has all headers and body)
			if response["status"] != nil && response["headers"] != nil {
				done <- true
			}
		}
	}
	p.interceptor.pausedLock.Unlock()

	// Wait for completion or timeout
	select {
	case <-done:
		p.handleVerbose(fmt.Sprintf("Response %s completed", requestID))
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for response %s to complete", requestID)
	}
}

// GetResponseBodySafely gets response body with automatic waiting for completion
// This is the recommended method for getting response bodies
func (p *Page) GetResponseBodySafely(requestID string) (*ResponseBody, error) {
	// First wait for response to be complete
	err := p.WaitForResponseComplete(requestID, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("response not ready: %v", err)
	}

	// Now try to get the body with retry
	return p.GetResponseBodyWithRetry(requestID, 3, 500*time.Millisecond)
}

// GetResponseBodyAsStringSafely gets response body as string with automatic waiting
func (p *Page) GetResponseBodyAsStringSafely(requestID string) (string, error) {
	responseBody, err := p.GetResponseBodySafely(requestID)
	if err != nil {
		return "", err
	}

	if responseBody.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return string(decoded), nil
	}

	return responseBody.Body, nil
}

// GetResponseBodyAsBytesSafely gets response body as bytes with automatic waiting
func (p *Page) GetResponseBodyAsBytesSafely(requestID string) ([]byte, error) {
	responseBody, err := p.GetResponseBodySafely(requestID)
	if err != nil {
		return nil, err
	}

	if responseBody.Base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return decoded, nil
	}

	return []byte(responseBody.Body), nil
}

// GetResponseBodyAsString gets the response body as a string, handling base64 decoding if needed
func (p *Page) GetResponseBodyAsString(requestID string) (string, error) {
	responseBody, err := p.GetResponseBody(requestID)
	if err != nil {
		return "", err
	}

	if responseBody.Base64Encoded {
		// Decode base64 if needed
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return string(decoded), nil
	}

	return responseBody.Body, nil
}

// GetResponseBodyAsBytes gets the response body as bytes, handling base64 decoding if needed
func (p *Page) GetResponseBodyAsBytes(requestID string) ([]byte, error) {
	responseBody, err := p.GetResponseBody(requestID)
	if err != nil {
		return nil, err
	}

	if responseBody.Base64Encoded {
		// Decode base64 if needed
		decoded, err := base64.StdEncoding.DecodeString(responseBody.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 response body: %v", err)
		}
		return decoded, nil
	}

	return []byte(responseBody.Body), nil
}
