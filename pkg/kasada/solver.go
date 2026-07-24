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
	XKpsdkH     string
	Headless    *bool
	Verbose     bool

	// Site-specific harvest flow. When Site is set, the solver loads the
	// mapped page, waits for the Kasada script to load, fires the site's
	// trigger request, harvests the SDK-injected x-kpsdk-* headers off it,
	// then aborts the request so nothing is actually submitted.
	Site     string
	PageName string
	// Email is used by trigger requests that need one (e.g. Ticketmaster login).
	Email string

	// Harvested holds every x-kpsdk-* header captured from the trigger request.
	Harvested map[string]string

	mu          sync.Mutex
	err         error
	harvestDone bool
}

// HandleKasada navigates to RequestUrl, harvests x-kpsdk-ct/st/v from network
// traffic, and generates x-kpsdk-cd. If KpsdkST is already set, only regenerates CD.
func (c *SolveKasada) HandleKasada() error {
	if c.KpsdkST != 0 {
		c.XKpsdkCd = GenerateCD(c.KpsdkST)
		return nil
	}

	if c.Site != "" {
		return c.handleSiteFlow()
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

	out := map[string]string{}
	// Start with any extra x-kpsdk-* headers harvested from the trigger request.
	for k, v := range c.Harvested {
		out[k] = v
	}
	if c.XKpsdkCt != "" {
		out["x-kpsdk-ct"] = c.XKpsdkCt
	}
	if c.XKpsdkCd != "" {
		out["x-kpsdk-cd"] = c.XKpsdkCd
	}
	if c.XKpsdkV != "" {
		out["x-kpsdk-v"] = c.XKpsdkV
	}
	if c.XKpsdkH != "" {
		out["x-kpsdk-h"] = c.XKpsdkH
	}
	return out
}

// handleSiteFlow loads a known site/page, waits for the Kasada script to load,
// fires the site's trigger request, then harvests and aborts it.
func (c *SolveKasada) handleSiteFlow() error {
	flow, err := resolveSiteFlow(c.Site, c.PageName)
	if err != nil {
		return err
	}

	c.err = nil
	c.harvestDone = false
	c.Harvested = map[string]string{}

	navURL := c.RequestUrl
	if navURL == "" {
		navURL = flow.NavigateURL
	}
	if navURL == "" {
		return fmt.Errorf("kasada: no navigate URL for site %q page %q", c.Site, c.PageName)
	}

	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Deadline <= 0 {
		c.Deadline = 45
	}

	parentCtx := c.Context
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(c.Deadline)*time.Second)
	defer cancel()

	headless := true
	if c.Headless != nil {
		headless = *c.Headless
	}

	opts, err := chrome.NewOptions(&ctx, "", headless, c.ProxyString, "", c.Verbose, true, nil, true, false)
	if err != nil {
		return fmt.Errorf("kasada: create options: %w", err)
	}

	flags := []chrome.FlagType{chrome.NoFirstRun, chrome.NoDefaultBrowserCheck}
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

	if err := page.EnablePage(); err != nil {
		return fmt.Errorf("kasada: enable page: %w", err)
	}
	if err := page.EnableNetwork(); err != nil {
		return fmt.Errorf("kasada: enable network: %w", err)
	}

	// Intercept the trigger request at the Request stage so we can read the
	// SDK-injected headers and abort it before it reaches the server.
	if err := page.EnableRequestInterception([]map[string]any{
		chrome.CreateRequestPattern("*"+flow.HarvestMatch+"*", "Request"),
	}); err != nil {
		return fmt.Errorf("kasada: enable request interception: %w", err)
	}

	page.SetRequestAbortHandler(func(params map[string]any) bool {
		request, ok := params["request"].(map[string]any)
		if !ok {
			return false
		}
		url, _ := request["url"].(string)
		if !strings.Contains(url, flow.HarvestMatch) {
			return false
		}
		headers, ok := request["headers"].(map[string]any)
		if !ok {
			return false
		}

		c.mu.Lock()
		for k, v := range headers {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, "x-kpsdk-") {
				c.Harvested[lk] = fmt.Sprint(v)
			}
		}
		c.XKpsdkCt = firstNonEmpty(c.Harvested["x-kpsdk-ct"], c.XKpsdkCt)
		c.XKpsdkCd = firstNonEmpty(c.Harvested["x-kpsdk-cd"], c.XKpsdkCd)
		c.XKpsdkV = firstNonEmpty(c.Harvested["x-kpsdk-v"], c.XKpsdkV)
		c.XKpsdkH = firstNonEmpty(c.Harvested["x-kpsdk-h"], c.XKpsdkH)
		if c.XKpsdkCt != "" && c.XKpsdkCd != "" {
			c.harvestDone = true
		}
		c.mu.Unlock()
		return true // abort the request
	})

	// Detect when the Kasada anti-bot script has loaded on the page.
	ipsSeen := make(chan struct{}, 1)
	var ipsOnce sync.Once
	page.SetNetworkResponseHandler(func(params map[string]any) {
		response, ok := params["response"].(map[string]any)
		if !ok {
			return
		}
		url, _ := response["url"].(string)
		if flow.IPSMatch != "" && strings.Contains(url, flow.IPSMatch) {
			ipsOnce.Do(func() { close(ipsSeen) })
		}
	})

	if err := page.Navigate(navURL); err != nil {
		return fmt.Errorf("kasada: navigate: %w", err)
	}

	triggerScript := flow.BuildTrigger(c.Email)
	triggered := false
	var ipsSeenAt time.Time

	for {
		c.mu.Lock()
		done := c.harvestDone
		solveErr := c.err
		c.mu.Unlock()

		if solveErr != nil {
			return solveErr
		}
		if done {
			break
		}

		select {
		case <-parentCtx.Done():
			return errors.New("main context was cancelled")
		case <-ctx.Done():
			return errors.New("deadline has exceeded so the context cancelled")
		case <-ipsSeen:
			if ipsSeenAt.IsZero() {
				ipsSeenAt = time.Now()
			}
		default:
		}

		// Block detection.
		if body, bodyErr := page.GetContent(); bodyErr == nil && strings.Contains(body, "The Chromium Authors") {
			return errors.New("blocked")
		}

		// Once the Kasada script is present, wait for the SDK to initialize,
		// then fire the trigger request exactly once.
		if !triggered && !ipsSeenAt.IsZero() {
			ready := false
			if r, e := page.Evaluate(kpsdkReadyScript); e == nil {
				ready = r.BoolValueOrDefault()
			}
			// Fall back to firing anyway if the SDK is slow to expose itself.
			if ready || time.Since(ipsSeenAt) > 3*time.Second {
				if _, e := page.Evaluate(triggerScript); e != nil {
					return fmt.Errorf("kasada: trigger request: %w", e)
				}
				triggered = true
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Prefer the SDK's own x-kpsdk-cd; only synthesize one if it was missing
	// but a server timestamp was captured.
	c.mu.Lock()
	if c.XKpsdkCd == "" && c.KpsdkST != 0 {
		c.XKpsdkCd = GenerateCD(c.KpsdkST)
	}
	c.mu.Unlock()

	return nil
}

func (c *SolveKasada) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.err = err
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
