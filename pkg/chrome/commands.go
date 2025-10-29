package chrome

import "encoding/json"

// enablePage enables the page
func (p *Page) enablePage() *Command {
	return p.NewCommand("Page.enable", nil)
}

// enableNetwork enables the network
func (p *Page) enableNetwork() *Command {
	return p.NewCommand("Network.enable", nil)
}

// enableFetch enables the fetch
func (p *Page) enableFetch() *Command {
	p.proxyIdentifier = p.messageCounter + 1
	return p.NewCommand("Fetch.enable", map[string]any{
		"handleAuthRequests": true,
		"patterns": []map[string]any{
			{
				"urlPattern":   "*",
				"requestStage": "Request",
			},
		},
	})
}

// enableFetchWithPatterns enables fetch with custom patterns
func (p *Page) enableFetchWithPatterns(patterns []map[string]any) *Command {
	p.proxyIdentifier = p.messageCounter + 1
	patterns = append(patterns, map[string]any{
		"urlPattern":   "*",
		"requestStage": "Request",
	})
	patterns = append(patterns, map[string]any{
		"urlPattern":   "*",
		"requestStage": "Response",
	})
	return p.NewCommand("Fetch.enable", map[string]any{
		"handleAuthRequests": true,
		"patterns":           patterns,
	})
}

// continueRequest continues a paused request
func (p *Page) continueRequest(requestID string) *Command {
	return p.NewCommand("Fetch.continueRequest", map[string]any{
		"requestId": requestID,
	})
}

// continueRequestWithModifications continues a paused request with modifications
func (p *Page) continueRequestWithModifications(requestID string, modifications ...map[string]any) error {
	params := map[string]any{
		"requestId": requestID,
	}

	// Apply modifications if provided
	if len(modifications) > 0 {
		mod := modifications[0]
		if url, ok := mod["url"]; ok {
			params["url"] = url
		}
		if method, ok := mod["method"]; ok {
			params["method"] = method
		}
		if headers, ok := mod["headers"]; ok {
			params["headers"] = headers
		}
		if body, ok := mod["body"]; ok {
			params["body"] = body
		}
	}

	command := p.NewCommand("Fetch.continueRequest", params)
	return p.send(command)
}

// failRequest fails a paused request
func (p *Page) failRequest(requestID string, errorReason string) *Command {
	return p.NewCommand("Fetch.failRequest", map[string]any{
		"requestId":   requestID,
		"errorReason": errorReason,
	})
}

// fulfillRequest fulfills a paused request with a custom response
func (p *Page) fulfillRequest(requestID string, responseCode int, responseHeaders []map[string]string, body string) *Command {
	return p.NewCommand("Fetch.fulfillRequest", map[string]any{
		"requestId":       requestID,
		"responseCode":    responseCode,
		"responseHeaders": responseHeaders,
		"body":            body,
	})
}

// getResponseBody gets the response body for a request
func (p *Page) getResponseBody(requestID string) *Command {
	return p.NewCommand("Fetch.getResponseBody", map[string]any{
		"requestId": requestID,
	})
}

// disableFetch disables the fetch
func (p *Page) disableFetch() *Command {
	return p.NewCommand("Fetch.disable", nil)
}

// waitLoad waits for the page to load
func (p *Page) waitLoad() *Command {
	return p.NewCommand("Page.waitForLoadEvent", map[string]any{"event": "load"})
}

// navigateTo navigates to a given URL
func (p *Page) navigateTo(url string) *Command {
	return p.NewCommand("Page.navigate", map[string]any{"url": url})
}

// evaluate evaluates a given script and returns the result
func (p *Page) evaluate(script string) *Command {
	p.handleVerbose("Evaluating " + script)
	return p.NewCommand("Runtime.evaluate", map[string]any{
		"expression":    script,
		"returnByValue": true,
	})
}

// evaluate evaluates a given script
func (p *Page) evaluateAsync(script string) *Command {
	p.handleVerbose("Evaluating " + script)
	return p.NewCommand("Runtime.evaluate", map[string]any{"expression": script, "awaitPromise": true, "returnByValue": true})
}

// getAllCookies gets all cookies
func (p *Page) getAllCookies() *Command {
	p.handleVerbose("Getting all cookies")
	return p.NewCommand("Network.getAllCookies", nil)
}

// setCookie sets a cookie
func (p *Page) setCookie(cookie *Cookie) *Command {
	p.handleVerbose("Setting cookie " + cookie.Name)
	jsonCookie, _ := json.Marshal(cookie)
	var v map[string]any
	json.Unmarshal(jsonCookie, &v)
	return p.NewCommand("Network.setCookie", v)
}
