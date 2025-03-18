package chrome

func (p *Page) enablePage() *Command {
	if p.verbose {
		p.logger.Info("Enabling page")
	}
	return p.NewCommand("Page.enable", nil)
}

func (p *Page) enableNetwork() *Command {
	if p.verbose {
		p.logger.Info("Enabling network")
	}
	return p.NewCommand("Network.enable", nil)
}

func (p *Page) navigateTo(url string) *Command {
	if p.verbose {
		p.logger.Info("Navigating to " + url)
	}
	return p.NewCommand("Page.navigate", map[string]any{"url": url})
}

func (p *Page) getTitle() *Command {
	if p.verbose {
		p.logger.Info("Getting title")
	}
	return p.NewCommand("Page.getTitle", nil)
}

func (p *Page) getContent() *Command {
	if p.verbose {
		p.logger.Info("Getting content")
	}
	return p.NewCommand("Page.getContent", nil)
}

func (p *Page) evaluate(script string) *Command {
	if p.verbose {
		p.logger.Info("Evaluating " + script)
	}
	return p.NewCommand("Runtime.evaluate", map[string]any{"expression": script})
}
