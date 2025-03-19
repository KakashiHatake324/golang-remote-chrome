package chrome

import (
	"encoding/json"
	"fmt"

	"github.com/KakashiHatake324/mockjs"
	"github.com/gorilla/websocket"
)

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

			// Handle different WebSocket messages
			if method, exists := response["method"]; exists {
				switch method {
				case "Fetch.requestPaused":
					p.handleRequestPaused(message)
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
						p.handleVerbose("Disabling fetch")
						p.DisableFetch()
					}
				default:

					if p.verbose {
						//p.logger.Info(fmt.Sprintf("Unhandled event: %s", method))
					}
				}
			} else {
				p.communicator <- response
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
		return
	}

	if method, exists := response["method"]; exists && method == "Fetch.requestPaused" {
		// Get the requestId and any additional details you might need
		params := response["params"].(map[string]any)
		requestID := params["requestId"].(string)

		// You can modify the request or simply continue it
		// Here, we continue the request without modification
		p.continueRequest(requestID)
	}
}

// Function to send the continueRequest command to Chrome
func (p *Page) continueRequest(requestID string) {
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
