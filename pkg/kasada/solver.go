package kasada

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
)

// SolveKasada harvests Kasada KP SDK headers from a real Chrome session
// and generates the local x-kpsdk-cd proof-of-work payload.
type SolveKasada struct {
	Context     context.Context
	Deadline    int
	UserAgent   string
	RequestUrl  string
	KpsdkST     int64
	ProxyString string
	XKpsdkCt    string
	XKpsdkCd    string
	XKpsdkV     string
	Headless    *bool
	Verbose     bool

	mu  sync.Mutex
	err error
}

// HandleKasada navigates to RequestUrl, harvests x-kpsdk-ct/st/v from network
// traffic, and generates x-kpsdk-cd. If KpsdkST is already set, only regenerates CD.
func (c *SolveKasada) HandleKasada() error {
	if c.KpsdkST != 0 {
		c.XKpsdkCd = GenerateCD(c.KpsdkST)
		return nil
	}

	c.XKpsdkCd = ""
	c.XKpsdkCt = ""
	c.err = nil

	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Deadline <= 0 {
		c.Deadline = 20
	}

	parentCtx := c.Context
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(c.Deadline)*time.Second)
	defer cancel()

	headless := true
	if c.Headless != nil {
		headless = *c.Headless
	}

	opts, err := chrome.NewOptions(
		&ctx,
		"",
		headless,
		c.ProxyString,
		"",
		c.Verbose,
		true,
		nil,
		true, // force Chrome like browser-go
		false,
	)
	if err != nil {
		return fmt.Errorf("kasada: create options: %w", err)
	}

	flags := []chrome.FlagType{
		chrome.NoFirstRun,
		chrome.NoDefaultBrowserCheck,
	}
	if c.UserAgent != "" {
		flags = append(flags, chrome.SetUserAgent(c.UserAgent))
	}

	browser, err := chrome.LaunchChrome("about:blank", opts, flags)
	if err != nil {
		return fmt.Errorf("kasada: launch chrome: %w", err)
	}
	defer browser.Close()

	page := browser.GetCurrentPage()
	page.WithContext(ctx)

	if err := page.EnableNetwork(); err != nil {
		return fmt.Errorf("kasada: enable network: %w", err)
	}
	if err := page.EnablePage(); err != nil {
		return fmt.Errorf("kasada: enable page: %w", err)
	}
	// Proxy auth requires Fetch
	if opts.GetProxyUser() != "" {
		if err := page.EnableFetch(); err != nil {
			return fmt.Errorf("kasada: enable fetch: %w", err)
		}
	}

	page.SetNetworkRequestHandler(func(params map[string]any) {
		select {
		case <-parentCtx.Done():
			c.setErr(errors.New("main context was cancelled"))
			return
		case <-ctx.Done():
			c.setErr(errors.New("deadline has exceeded so the context cancelled"))
			return
		default:
		}

		request, ok := params["request"].(map[string]any)
		if !ok {
			return
		}
		headers, ok := request["headers"].(map[string]any)
		if !ok {
			return
		}
		if v := headerValue(headers, "x-kpsdk-v"); v != "" {
			c.mu.Lock()
			c.XKpsdkV = v
			c.mu.Unlock()
		}
	})

	page.SetNetworkResponseHandler(func(params map[string]any) {
		select {
		case <-parentCtx.Done():
			c.setErr(errors.New("main context was cancelled"))
			return
		case <-ctx.Done():
			c.setErr(errors.New("deadline has exceeded so the context cancelled"))
			return
		default:
		}

		response, ok := params["response"].(map[string]any)
		if !ok {
			return
		}
		headers, ok := response["headers"].(map[string]any)
		if !ok {
			return
		}

		st := headerValue(headers, "x-kpsdk-st")
		if st == "" {
			return
		}

		status := responseStatus(response)
		if status == 429 {
			c.setErr(errors.New("blocked"))
			return
		}

		kpst, err := strconv.ParseInt(st, 10, 64)
		if err != nil {
			return
		}
		ct := headerValue(headers, "x-kpsdk-ct")

		c.mu.Lock()
		c.KpsdkST = kpst
		c.XKpsdkCt = ct
		c.mu.Unlock()
	})

	if err := page.Navigate(c.RequestUrl); err != nil {
		return fmt.Errorf("kasada: navigate: %w", err)
	}

	for {
		c.mu.Lock()
		st := c.KpsdkST
		solveErr := c.err
		c.mu.Unlock()

		if solveErr != nil {
			return solveErr
		}
		if st != 0 {
			break
		}

		select {
		case <-parentCtx.Done():
			return errors.New("main context was cancelled")
		case <-ctx.Done():
			return errors.New("deadline has exceeded so the context cancelled")
		default:
		}

		body, bodyErr := page.GetContent()
		if bodyErr == nil && strings.Contains(body, "The Chromium Authors") {
			return errors.New("blocked")
		}

		time.Sleep(time.Second)
	}

	c.mu.Lock()
	st := c.KpsdkST
	c.mu.Unlock()

	c.XKpsdkCd = GenerateCD(st)
	return nil
}

// Headers returns the Kasada headers map ready to attach to HTTP requests.
func (c *SolveKasada) Headers() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]string{
		"x-kpsdk-ct": c.XKpsdkCt,
		"x-kpsdk-cd": c.XKpsdkCd,
		"x-kpsdk-v":  c.XKpsdkV,
	}
}

func (c *SolveKasada) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.err = err
	}
}

func headerValue(headers map[string]any, key string) string {
	if v, ok := headers[key]; ok {
		return fmt.Sprint(v)
	}
	lower := strings.ToLower(key)
	for k, v := range headers {
		if strings.ToLower(k) == lower {
			return fmt.Sprint(v)
		}
	}
	return ""
}

func responseStatus(response map[string]any) int {
	switch v := response["status"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
