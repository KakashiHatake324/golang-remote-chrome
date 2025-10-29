package chrome

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KakashiHatake324/mockjs"
	"github.com/gorilla/websocket"
)

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// newSocket creates a new WebSocket connection to the Chrome debugger
func (p *Page) newSocket(wsUrl string) (*websocket.Conn, error) {
	ws, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return nil, err
	}

	go func() {
		defer ws.Close() // Ensure WebSocket closes on exit

		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return // Gracefully exit the loop
			}

			var response map[string]any
			if err := json.Unmarshal(message, &response); err != nil {
				p.handleVerbose(fmt.Sprintf("Failed to parse WebSocket message: %s", message))
				continue
			}

			if strings.Contains(string(message), "Page") {
				p.handleVerbose(fmt.Sprintf("Page: %s", mockjs.InitWindow().JSON.Stringify(response)))
			}

			// Handle different WebSocket messages
			if method, exists := response["method"]; exists {
				switch method {
				case "Page.loadEventFired":
					go func() {
						p.loadEventFired <- response
					}()
				case "Fetch.requestPaused":
					go p.handleRequestPaused(message)
				case "Network.responseReceived", "Network.loadingFinished", "Network.requestWillBeSentExtraInfo", "Network.requestWillBeSent", "Network.responseReceivedExtraInfo", "Page.frameAttached", "Network.dataReceived":
				case "Fetch.authRequired":
					if p.verbose {
						p.logger.Info(fmt.Sprintf("Fetch.authRequired: %s", mockjs.InitWindow().JSON.Stringify(response)))
						p.logger.Info(fmt.Sprintf("proxyUser: %s", p.proxyUser))
						p.logger.Info(fmt.Sprintf("proxyPass: %s", p.proxyPass))
					}
					if err := p.handleProxyAuth(ws, response); err != nil {
						p.handleVerbose(fmt.Sprintf("Failed to handle proxy authentication: %v", err))
					} else {
						p.handleVerbose("Proxy authentication handled, keeping fetch enabled for response logging")
						// Don't disable fetch - we want to keep response logging working
					}
				default:
				}
			} else {
				go func() {
					p.communicator <- response
				}()
			}
		}
	}()

	return ws, nil
}

// handleRequestPaused handles the request paused event
func (p *Page) handleRequestPaused(message []byte) {
	var response map[string]any
	err := json.Unmarshal(message, &response)
	if err != nil {
		p.logger.Error("Failed to unmarshal request paused event", err)
		return
	}

	params, ok := response["params"].(map[string]any)
	if !ok {
		return
	}

	requestID, _ := params["requestId"].(string)

	// Debug logging
	p.handleVerbose(fmt.Sprintf("Intercepted event for request %s", requestID))
	p.handleVerbose(fmt.Sprintf("Params keys: %v", getMapKeys(params)))
	if _, hasResponse := params["response"]; hasResponse {
		p.handleVerbose("This is a RESPONSE event")
	} else if _, hasRequest := params["request"]; hasRequest {
		p.handleVerbose("This is a REQUEST event")
	} else {
		p.handleVerbose("Unknown event type")
	}

	// Check if this is a response by looking for the response field
	if _, hasResponse := params["response"]; hasResponse {
		// This is a response - handle response interception
		p.interceptor.pausedLock.Lock()
		if pausedData, exists := p.interceptor.pausedResponses[requestID]; exists {
			pausedData["responseData"] = params
			p.interceptor.pausedResponses[requestID] = pausedData
			p.interceptor.pausedLock.Unlock()

			p.handleVerbose(fmt.Sprintf("Response for request %s is paused and waiting for resume", requestID))
			return // Don't continue the response, it's paused
		}
		p.interceptor.pausedLock.Unlock()

		// Check if response handler is set and call it
		p.interceptor.pausedLock.RLock()
		responseHandler := p.interceptor.responseHandler
		p.interceptor.pausedLock.RUnlock()

		if responseHandler != nil {
			p.handleVerbose(fmt.Sprintf("Calling response handler for request %s", requestID))
			// Run response handler in a new goroutine to avoid blocking the event loop
			go func() {
				defer func() {
					if r := recover(); r != nil {
						p.handleVerbose(fmt.Sprintf("Response handler panicked: %v", r))
					}
				}()
				responseHandler(params)
			}()
		} else {
			p.handleVerbose("No response handler set")
		}
	} else {
		// This is a request - handle request interception
		p.interceptor.pausedLock.Lock()
		if pausedData, exists := p.interceptor.pausedRequests[requestID]; exists {
			pausedData["requestData"] = params
			p.interceptor.pausedRequests[requestID] = pausedData
			p.interceptor.pausedLock.Unlock()

			p.handleVerbose(fmt.Sprintf("Request %s is paused and waiting for resume", requestID))
			return // Don't continue the request, it's paused
		}
		p.interceptor.pausedLock.Unlock()

		// Check if request handler is set and call it
		p.handlerLock.Lock()
		handler := p.requestPausedHandler
		p.handlerLock.Unlock()

		if handler != nil {
			p.handleVerbose(fmt.Sprintf("Calling request handler for request %s", requestID))
			// Run handler in a new goroutine to avoid blocking the event loop
			go handler(params)
		} else {
			p.handleVerbose("No request handler set")
		}
	}

	// Continue the request/response as before (only if not paused)
	command := p.continueRequest(requestID)
	p.send(command)
}

// Function to send the continueRequest command to Chrome
func (p *Page) continueRequestCommand(requestID string) {
	continueCommand := map[string]any{
		"id":     p.GetNewMessageCounter(),
		"method": "Fetch.continueRequest",
		"params": map[string]any{
			"requestId": requestID, // Continue the paused request
		},
	}
	if p.verbose {
		p.logger.Info(fmt.Sprintf("continueRequest: %s", mockjs.InitWindow().JSON.Stringify(continueCommand)))
	}
	p.sendCommand(continueCommand)
}

// Function to send a command to the browser
func (p *Page) sendCommand(command map[string]any) {
	p.socketLock.Lock()
	defer p.socketLock.Unlock()
	message, err := json.Marshal(command)
	if err != nil {
		if p.verbose {
			p.logger.Error("Error marshaling command:", err)
		}
	}

	err = p.wsConn.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		if p.verbose {
			p.logger.Error("Error sending command:", err)
		}
	}
	p.handleVerbose(fmt.Sprintf("sent command: %s", string(message)))
}

// handleProxyAuth handles proxy authentication by sending credentials
func (p *Page) handleProxyAuth(ws *websocket.Conn, response map[string]any) error {
	p.socketLock.Lock()
	defer p.socketLock.Unlock()
	params, _ := response["params"].(map[string]any)
	requestID, _ := params["requestId"].(string)
	authResponse := map[string]any{
		"id":     p.proxyIdentifier,
		"method": "Fetch.continueWithAuth",
		"params": map[string]any{
			"requestId": requestID,
			"authChallengeResponse": map[string]any{
				"response": "ProvideCredentials",
				"username": p.proxyUser,
				"password": p.proxyPass,
			},
		},
	}

	authJSON, _ := json.Marshal(authResponse)
	if err := ws.WriteMessage(websocket.TextMessage, authJSON); err != nil {
		return fmt.Errorf("failed to send auth response: %v", err)
	}
	return nil
}
