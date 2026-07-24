package chrome

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// browserRPC is a minimal single-user synchronous CDP connection to the
// browser-level endpoint (as opposed to a page target). It is used to manage
// browser contexts and targets, which are not addressable from a page session.
type browserRPC struct {
	conn *websocket.Conn
	id   int
}

// dialBrowser opens a synchronous RPC connection to the browser-level debugger
// websocket (from /json/version).
func dialBrowser(port string) (*browserRPC, error) {
	versionURL := fmt.Sprintf("http://localhost:%s/json/version", port)
	resp, err := http.Get(versionURL)
	if err != nil {
		return nil, fmt.Errorf("chrome: fetch /json/version: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("chrome: decode /json/version: %w", err)
	}
	if payload.WebSocketDebuggerURL == "" {
		return nil, fmt.Errorf("chrome: no browser webSocketDebuggerUrl")
	}

	conn, _, err := websocket.DefaultDialer.Dial(payload.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("chrome: dial browser ws: %w", err)
	}
	return &browserRPC{conn: conn}, nil
}

// call sends a CDP command and blocks until the matching response arrives,
// returning its "result" object. Events and other responses are skipped.
func (r *browserRPC) call(method string, params map[string]any) (map[string]any, error) {
	r.id++
	id := r.id
	cmd := map[string]any{"id": id, "method": method}
	if params != nil {
		cmd["params"] = params
	}
	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}
	if err := r.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return nil, err
	}

	for {
		_, message, err := r.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		var resp map[string]any
		if err := json.Unmarshal(message, &resp); err != nil {
			continue
		}
		respID, ok := resp["id"].(float64)
		if !ok || int(respID) != id {
			continue // event or a different command's response
		}
		if errObj, ok := resp["error"].(map[string]any); ok {
			return nil, fmt.Errorf("chrome: %s error: %v", method, errObj["message"])
		}
		result, _ := resp["result"].(map[string]any)
		return result, nil
	}
}

func (r *browserRPC) close() {
	if r.conn != nil {
		r.conn.Close()
	}
}

// NewIsolatedPage creates a fresh, isolated browser context (its own
// cookies/storage jar) with a single blank tab, connects a Page to it, and
// returns the Page plus the browser context id. Dispose the context with
// DisposeContext when finished. This enables running independent sessions
// concurrently in one warm browser without cookie cross-contamination.
func (b *Browser) NewIsolatedPage() (*Page, string, error) {
	port := b.Opts.GetPort()

	rpc, err := dialBrowser(port)
	if err != nil {
		return nil, "", err
	}
	defer rpc.close()

	ctxRes, err := rpc.call("Target.createBrowserContext", map[string]any{"disposeOnDetach": false})
	if err != nil {
		return nil, "", fmt.Errorf("chrome: create browser context: %w", err)
	}
	contextID, _ := ctxRes["browserContextId"].(string)
	if contextID == "" {
		return nil, "", fmt.Errorf("chrome: empty browserContextId")
	}

	targetRes, err := rpc.call("Target.createTarget", map[string]any{
		"url":              "about:blank",
		"browserContextId": contextID,
	})
	if err != nil {
		return nil, "", fmt.Errorf("chrome: create target: %w", err)
	}
	targetID, _ := targetRes["targetId"].(string)
	if targetID == "" {
		return nil, "", fmt.Errorf("chrome: empty targetId")
	}

	wsURL, currentURL, err := findTargetWS(port, targetID)
	if err != nil {
		return nil, contextID, err
	}

	p := newPage(targetID, wsURL, currentURL, b.verbose, b.Opts.GetProxyUser(), b.Opts.GetProxyPass())
	conn, err := p.newSocket(wsURL)
	if err != nil {
		return nil, contextID, fmt.Errorf("chrome: connect target ws: %w", err)
	}
	p.wsConn = conn
	b.AddPage(p)

	return p, contextID, nil
}

// DisposeContext tears down a browser context created by NewIsolatedPage,
// closing its tabs and freeing its resources.
func (b *Browser) DisposeContext(contextID string) error {
	if contextID == "" {
		return nil
	}
	rpc, err := dialBrowser(b.Opts.GetPort())
	if err != nil {
		return err
	}
	defer rpc.close()

	_, err = rpc.call("Target.disposeBrowserContext", map[string]any{"browserContextId": contextID})
	return err
}

// findTargetWS polls /json/list for the given target id and returns its page
// websocket url and current url.
func findTargetWS(port, targetID string) (string, string, error) {
	listURL := fmt.Sprintf("http://localhost:%s/json/list", port)
	var lastErr error
	for range 50 {
		resp, err := http.Get(listURL)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var targets []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
			resp.Body.Close()
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		resp.Body.Close()

		for _, t := range targets {
			if id, _ := t["id"].(string); id == targetID {
				wsURL, _ := t["webSocketDebuggerUrl"].(string)
				currentURL, _ := t["url"].(string)
				if wsURL != "" {
					return wsURL, currentURL, nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return "", "", fmt.Errorf("chrome: locate target %s ws: %w", targetID, lastErr)
	}
	return "", "", fmt.Errorf("chrome: target %s not found in /json/list", targetID)
}
