package chrome

import (
	"encoding/json"
	"strings"
)

type Cookies struct {
	Cookies []*Cookie `json:"cookies"`
}
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  int64  `json:"expires"`
	Size     int    `json:"size"`
	HttpOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
	SameSite string `json:"sameSite"`
	Priority string `json:"priority"`
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

// GetAllCookies returns all cookies
func (p *Page) GetAllCookies() ([]*Cookie, error) {
	command := p.getAllCookies()
	if response, err := p.sendAndReceive(command); err != nil {
		return nil, err
	} else {
		var cookies Cookies
		json.Unmarshal([]byte(response.Value), &cookies)
		return cookies.Cookies, nil
	}
}
