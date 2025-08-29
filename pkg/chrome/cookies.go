package chrome

import (
	"encoding/json"
	"errors"
)

// Cookies is a struct that contains a list of cookies
type Cookies struct {
	Cookies []*Cookie `json:"cookies"`
}

// Cookie is a struct that contains a cookie
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

// GetAllCookies returns all cookies
func (p *Page) GetAllCookies() ([]*Cookie, error) {
	command := p.getAllCookies()
	if response, err := p.sendAndReceive(command); err != nil {
		return nil, err
	} else {
		var cookies Cookies
		json.Unmarshal([]byte(response.StringValue()), &cookies)
		return cookies.Cookies, nil
	}
}

// SetCookie sets a cookie for the current page
func (p *Page) SetCookie(cookie *Cookie) error {
	if cookie.Domain == "" {
		return errors.New("domain is required")
	}
	command := p.setCookie(cookie)
	if err := p.send(command); err != nil {
		return err
	} else {
		return nil
	}
}

// SetCookieCookies sets multiple cookies for the current page
func (p *Page) SetCookieCookies(cookies []*Cookie) error {
	for _, cookie := range cookies {
		if cookie.Domain == "" {
			return errors.New("domain is required")
		}
		if err := p.SetCookie(cookie); err != nil {
			return err
		}
	}
	return nil
}
