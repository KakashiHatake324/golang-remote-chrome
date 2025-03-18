package chrome

import (
	"strings"
)

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// GetCookies returns the cookies for the current page
func (p *Page) GetCookies() ([]*Cookie, error) {
	if response, err := p.Evaluate("document.cookie"); err != nil {
		return nil, err
	} else {
		cookies := []*Cookie{}
		cookiesString := strings.Split(response.Value, ";")
		for _, cookie := range cookiesString {
			cookieParts := strings.SplitN(cookie, "=", 2)
			if len(cookieParts) == 2 {
				cookies = append(cookies, &Cookie{Name: strings.TrimSpace(cookieParts[0]), Value: strings.TrimSpace(cookieParts[1])})
			}
		}
		return cookies, nil
	}
}
