package chrome

import _ "embed"

//go:embed js/stealth.min.js
var stealthJS string

// UseStealth uses the stealth mode for the browser
func (p *Page) UseStealth() error {
	command := p.evaluate(stealthJS)
	if err := p.send(command); err != nil {
		return err
	} else {
		return nil
	}
}
