package chrome

func (p *Page) enablePage() *Command {
	return p.NewCommand("Page.enable", nil)
}

func (p *Page) enableNetwork() *Command {
	return p.NewCommand("Network.enable", nil)
}

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

func (p *Page) navigateTo(url string) *Command {
	return p.NewCommand("Page.navigate", map[string]any{"url": url})
}

func (p *Page) evaluate(script string) *Command {
	p.handleVerbose("Evaluating " + script)
	return p.NewCommand("Runtime.evaluate", map[string]any{"expression": script})
}

func (p *Page) getAllCookies() *Command {
	p.handleVerbose("Getting all cookies")
	return p.NewCommand("Network.getAllCookies", nil)
}
