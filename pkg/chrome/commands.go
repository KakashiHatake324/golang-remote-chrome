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
		"patterns": []map[string]interface{}{
			{
				"urlPattern":   "*",
				"requestStage": "Request",
			},
		},
	})
}

// navigateTo navigates to a given URL
func (p *Page) navigateTo(url string) *Command {
	return p.NewCommand("Page.navigate", map[string]any{"url": url})
}

// evaluate evaluates a given script
func (p *Page) evaluate(script string) *Command {
	p.handleVerbose("Evaluating " + script)
	return p.NewCommand("Runtime.evaluate", map[string]any{"expression": script})
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
