// Package recaptcha harvests reCAPTCHA v3 (and Enterprise) tokens from a real
// headless Chrome, reusing the same warm-browser + isolated-context + proxy-MITM
// design as the Kasada harvester.
//
// For each harvest it navigates a fresh isolated context to the target origin,
// fulfills that top-level document with a minimal page that only loads the
// reCAPTCHA API (spoofing the origin so the token is bound to the correct
// domain), and forwards every other request through a per-harvest fingerprinted
// TLS client bound to your proxy — so the token's score is computed against the
// same egress IP you'll reuse. It then calls grecaptcha.execute(sitekey, action)
// and returns the resulting token.
//
// Only token generation is supported (v3 / invisible-style). It does not solve
// v2 image challenges.
package recaptcha

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/internal/browserforward"
	"github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

// Config configures a reusable reCAPTCHA Harvester (the warm browser).
type Config struct {
	// UserAgent overrides the browser UA. Defaults to a Chrome 133 UA.
	UserAgent string
	// Headless controls whether Chrome runs headless. Defaults to true.
	Headless *bool
	// Verbose enables verbose Chrome logging.
	Verbose bool
	// Deadline is the per-harvest timeout in seconds. Defaults to 30.
	Deadline int
	// BlockResources drops heavy/irrelevant resources. Defaults to true.
	BlockResources *bool
	// TLSProfile is the TLS/HTTP fingerprint for the forwarding client.
	// Defaults to Chrome 133.
	TLSProfile profiles.ClientProfile
	// MaxConcurrency caps parallel harvests in HarvestMany. Defaults to 5.
	MaxConcurrency int
	// ExtraFlags are appended to the Chrome launch flags.
	ExtraFlags []chrome.FlagType
	// InitScript, if set, is evaluated in every harvest document before page
	// scripts run (navigator/WebGL/userAgentData overrides).
	InitScript string
}

// Version selects which reCAPTCHA flavor to solve.
type Version string

const (
	// V3 is standard reCAPTCHA v3 (grecaptcha.execute with an action → score).
	V3 Version = "v3"
	// V3Enterprise is reCAPTCHA Enterprise score-based
	// (grecaptcha.enterprise.execute). Verified via createAssessment, not
	// siteverify.
	V3Enterprise Version = "v3-enterprise"
	// V2Invisible is reCAPTCHA v2 invisible: an invisible widget is rendered and
	// executed; a token is returned only when the risk analysis passes silently
	// (image challenges are NOT solved).
	V2Invisible Version = "v2-invisible"
)

// normalize returns the version, defaulting empty to V3.
func (v Version) normalize() Version {
	if v == "" {
		return V3
	}
	return v
}

// valid reports whether v is a supported version.
func (v Version) valid() bool {
	switch v.normalize() {
	case V3, V3Enterprise, V2Invisible:
		return true
	default:
		return false
	}
}

// Target describes a single reCAPTCHA token request.
type Target struct {
	// Domain is the site the token is bound to. Accepts "example.com",
	// "https://example.com", or a full URL; only the origin is used.
	Domain string
	// SiteKey is the reCAPTCHA site key (data-sitekey / render key).
	SiteKey string
	// Action is the action name (e.g. "login") for v3/Enterprise. Ignored for
	// v2 invisible.
	Action string
	// Version selects the reCAPTCHA flavor. Defaults to V3.
	Version Version
	// Cookies are seeded into the isolated context before navigation (e.g. a
	// target-domain session cookie, or a .google.com _GRECAPTCHA cookie to look
	// like a returning visitor). A cookie with an empty Domain defaults to the
	// target domain.
	Cookies []*chrome.Cookie
}

// Result holds a harvested token.
type Result struct {
	Token   string
	Domain  string
	SiteKey string
	Action  string
	Proxy   string
	Elapsed time.Duration
	// Cookies is the isolated context's cookie jar after the harvest, including
	// anything set during the flow (target-domain and google.com cookies such as
	// _GRECAPTCHA). Empty if the jar could not be read.
	Cookies []*chrome.Cookie
}

// Harvester keeps one warm Chrome and mints reCAPTCHA v3 tokens on demand, each
// in its own isolated context, forwarding through a per-harvest proxy.
type Harvester struct {
	cfg        Config
	browser    *chrome.Browser
	ua         string
	uaMetadata map[string]any
	profile    profiles.ClientProfile
	block      bool
	maxConc    int
	initScript string
}

// NewHarvester launches one warm Chrome. The browser makes no direct network
// connections: every request is intercepted and either origin-spoofed or
// forwarded through the per-harvest TLS client.
func NewHarvester(cfg Config) (*Harvester, error) {
	if cfg.Deadline <= 0 {
		cfg.Deadline = 30
	}
	maxConc := cfg.MaxConcurrency
	if maxConc <= 0 {
		maxConc = 5
	}
	block := true
	if cfg.BlockResources != nil {
		block = *cfg.BlockResources
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUA
	}
	profile := cfg.TLSProfile
	if profile.GetClientHelloId().Client == "" {
		profile = profiles.Chrome_133
	}
	headless := true
	if cfg.Headless != nil {
		headless = *cfg.Headless
	}

	ctx := context.Background()
	opts, err := chrome.NewOptions(&ctx, "", headless, "", "", cfg.Verbose, true, nil, true, false)
	if err != nil {
		return nil, fmt.Errorf("recaptcha: create options: %w", err)
	}

	flags := []chrome.FlagType{chrome.NoFirstRun, chrome.NoDefaultBrowserCheck, chrome.SetUserAgent(ua)}
	if runtime.GOOS == "linux" {
		flags = append(flags, chrome.NoSandbox, chrome.DisableDevSHM)
	}
	flags = append(flags, cfg.ExtraFlags...)

	browser, err := chrome.LaunchChrome("about:blank", opts, flags)
	if err != nil {
		return nil, fmt.Errorf("recaptcha: launch chrome: %w", err)
	}

	return &Harvester{
		cfg:        cfg,
		browser:    browser,
		ua:         ua,
		uaMetadata: browserforward.BuildUAMetadata(ua),
		profile:    profile,
		block:      block,
		maxConc:    maxConc,
		initScript: cfg.InitScript,
	}, nil
}

// Close shuts down the warm browser.
func (h *Harvester) Close() error {
	if h.browser != nil {
		return h.browser.Close()
	}
	return nil
}

// Harvest mints one token for target through the given proxy in a fresh
// isolated context.
func (h *Harvester) Harvest(ctx context.Context, proxy string, target Target) (*Result, error) {
	return h.harvestOne(ctx, proxy, target)
}

// HarvestMany mints one token per proxy (all for the same target) concurrently,
// bounded by MaxConcurrency. results[i]/errs[i] correspond to proxies[i].
func (h *Harvester) HarvestMany(ctx context.Context, proxies []string, target Target) ([]*Result, []error) {
	results := make([]*Result, len(proxies))
	errs := make([]error, len(proxies))

	sem := make(chan struct{}, h.maxConc)
	var wg sync.WaitGroup
	for i, proxy := range proxies {
		wg.Add(1)
		go func(i int, proxy string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()

			res, err := h.harvestOne(ctx, proxy, target)
			results[i], errs[i] = res, err
		}(i, proxy)
	}
	wg.Wait()
	return results, errs
}

func (h *Harvester) harvestOne(ctx context.Context, proxy string, target Target) (result *Result, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if target.SiteKey == "" {
		return nil, errors.New("recaptcha: SiteKey is required")
	}
	if !target.Version.valid() {
		return nil, fmt.Errorf("recaptcha: unsupported Version %q", target.Version)
	}
	version := target.Version.normalize()
	origin, err := originOf(target.Domain)
	if err != nil {
		return nil, err
	}
	navURL := origin + "/"

	client, err := browserforward.BuildTLSClient(proxy, h.profile, h.cfg.Deadline)
	if err != nil {
		return nil, err
	}

	page, contextID, err := h.browser.NewIsolatedPage()
	if err != nil {
		return nil, fmt.Errorf("recaptcha: new isolated page: %w", err)
	}
	defer func() {
		_ = h.browser.DisposeContext(contextID)
		_ = h.browser.ClosePage(page)
	}()

	if err := page.EnablePage(); err != nil {
		return nil, fmt.Errorf("recaptcha: enable page: %w", err)
	}
	if err := page.SetUserAgentOverride(h.ua, h.uaMetadata); err != nil {
		return nil, fmt.Errorf("recaptcha: set user agent override: %w", err)
	}
	if h.initScript != "" {
		if err := page.AddScriptToEvaluateOnNewDocument(h.initScript); err != nil {
			return nil, fmt.Errorf("recaptcha: add init script: %w", err)
		}
	}
	if err := page.EnableNetwork(); err != nil {
		return nil, fmt.Errorf("recaptcha: enable network: %w", err)
	}
	if len(target.Cookies) > 0 {
		host := hostOf(origin)
		seed := make([]*chrome.Cookie, 0, len(target.Cookies))
		for _, c := range target.Cookies {
			if c == nil {
				continue
			}
			cc := *c
			if cc.Domain == "" {
				cc.Domain = host
			}
			seed = append(seed, &cc)
		}
		if err := page.SetCookieCookies(seed); err != nil {
			return nil, fmt.Errorf("recaptcha: set cookies: %w", err)
		}
	}
	if err := page.EnableRequestInterceptionExact([]map[string]any{
		chrome.CreateRequestPattern("*", "Request"),
	}); err != nil {
		return nil, fmt.Errorf("recaptcha: enable request interception: %w", err)
	}

	hctx, cancel := context.WithTimeout(ctx, time.Duration(h.cfg.Deadline)*time.Second)
	defer cancel()
	page.WithContext(hctx)

	s := &session{
		h:         h,
		page:      page,
		client:    client,
		navURL:    navURL,
		bootstrap: bootstrapHTML(target.SiteKey, version),
	}
	page.SetRequestInterceptHandler(s.handleIntercept)

	start := time.Now()
	if err := page.Navigate(navURL); err != nil {
		return nil, fmt.Errorf("recaptcha: navigate: %w", err)
	}

	token, err := s.run(ctx, hctx, target)
	if err != nil {
		return nil, err
	}

	// Best-effort snapshot of the context's jar (e.g. _GRECAPTCHA); harvest
	// already succeeded, so a read failure must not fail the result.
	jar, _ := page.GetAllCookies()

	return &Result{
		Token:   token,
		Domain:  origin,
		SiteKey: target.SiteKey,
		Action:  target.Action,
		Proxy:   proxy,
		Elapsed: time.Since(start),
		Cookies: jar,
	}, nil
}

// session holds per-harvest state for a single isolated context.
type session struct {
	h         *Harvester
	page      *chrome.Page
	client    tls_client.HttpClient
	navURL    string
	bootstrap string
}

// run polls until grecaptcha yields a token or the deadline hits. Only this
// goroutine calls Evaluate on the page, so it cannot race the fire-and-forget
// forwarder on the shared response channel.
func (s *session) run(parentCtx, hctx context.Context, target Target) (string, error) {
	script := executeScript(target.SiteKey, target.Action, target.Version.normalize())

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-parentCtx.Done():
			return "", errors.New("recaptcha: parent context cancelled")
		case <-hctx.Done():
			// Surface any JS-side error for a better message.
			jsErr := ""
			if r, e := s.page.Evaluate(`window.__rc_err || ""`); e == nil {
				jsErr = r.StringValue()
			}
			if jsErr != "" {
				return "", fmt.Errorf("recaptcha: deadline exceeded: %s", jsErr)
			}
			return "", errors.New("recaptcha: deadline exceeded")
		case <-ticker.C:
		}

		r, e := s.page.Evaluate(script)
		if e != nil {
			continue
		}
		if token := r.StringValue(); token != "" {
			return token, nil
		}
	}
}

// handleIntercept spoofs the target origin's document and forwards everything
// else through the proxy.
func (s *session) handleIntercept(params map[string]any) bool {
	requestID, _ := params["requestId"].(string)
	request, ok := params["request"].(map[string]any)
	if !ok {
		return false
	}
	rawURL, _ := request["url"].(string)
	method, _ := request["method"].(string)
	if method == "" {
		method = "GET"
	}
	headers, _ := request["headers"].(map[string]any)
	postData, _ := request["postData"].(string)

	// 1. The top-level document: fulfill with a minimal page that only loads the
	// reCAPTCHA API. This makes the browser's origin the target domain (so the
	// token binds correctly) without downloading the real site.
	if rawURL == s.navURL {
		_ = s.page.FulfillRequest(requestID, 200,
			[]map[string]string{{"name": "content-type", "value": "text/html; charset=utf-8"}},
			browserforward.EncodeBody([]byte(s.bootstrap)))
		return true
	}

	// 2. Heavy/irrelevant resources: drop them.
	if s.h.block && browserforward.IsBlockedURL(rawURL) {
		_ = s.page.FailRequest(requestID, "BlockedByClient")
		return true
	}

	// 3. Forward everything else (reCAPTCHA api.js, anchor/reload, gstatic, …)
	// through the proxy so Google sees the proxy IP.
	if s.client == nil {
		return false
	}
	status, respHeaders, body, err := browserforward.Forward(s.client, method, rawURL, headers, postData)
	if err != nil {
		_ = s.page.FailRequest(requestID, "ConnectionFailed")
		return true
	}
	_ = s.page.FulfillRequest(requestID, status, respHeaders, browserforward.EncodeBody(body))
	return true
}

// originOf normalizes a domain/URL to a "https://host" origin.
func originOf(domain string) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", errors.New("recaptcha: Domain is required")
	}
	if !strings.Contains(domain, "://") {
		domain = "https://" + domain
	}
	u, err := url.Parse(domain)
	if err != nil {
		return "", fmt.Errorf("recaptcha: invalid domain %q: %w", domain, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("recaptcha: invalid domain %q", domain)
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Host, nil
}

// hostOf returns the host of a "scheme://host" origin.
func hostOf(origin string) string {
	if u, err := url.Parse(origin); err == nil {
		return u.Host
	}
	return ""
}

// bootstrapHTML returns a minimal document that loads the reCAPTCHA API for the
// given site key and version.
func bootstrapHTML(siteKey string, version Version) string {
	switch version {
	case V3Enterprise:
		src := "https://www.google.com/recaptcha/enterprise.js?render=" + url.QueryEscape(siteKey)
		return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><script src=%q></script></head><body></body></html>`, src)
	case V2Invisible:
		// Explicit render so we can bind an invisible widget programmatically;
		// the #rc container is the render target.
		src := "https://www.google.com/recaptcha/api.js?render=explicit"
		return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><script src=%q></script></head><body><div id="rc"></div></body></html>`, src)
	default: // V3
		src := "https://www.google.com/recaptcha/api.js?render=" + url.QueryEscape(siteKey)
		return fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"><script src=%q></script></head><body></body></html>`, src)
	}
}

// executeScript returns JS that (once) kicks off the solve and returns the token
// when ready, or "" while pending.
func executeScript(siteKey, action string, version Version) string {
	switch version {
	case V2Invisible:
		return v2InvisibleScript(siteKey)
	case V3Enterprise:
		return v3Script("grecaptcha.enterprise", siteKey, action)
	default:
		return v3Script("grecaptcha", siteKey, action)
	}
}

// v3Script drives grecaptcha[.enterprise].execute(sitekey, {action}).
func v3Script(obj, siteKey, action string) string {
	opts := "{}"
	if action != "" {
		opts = fmt.Sprintf(`{action:%q}`, action)
	}
	return fmt.Sprintf(`(function(){
  try {
    if (window.__rc_token) return window.__rc_token;
    var g = %s;
    if (typeof g === 'undefined' || !g || !g.execute) return "";
    if (!window.__rc_started) {
      window.__rc_started = true;
      g.ready(function(){
        g.execute(%q, %s).then(function(t){ window.__rc_token = t; }).catch(function(e){ window.__rc_err = String(e); });
      });
    }
    return window.__rc_token || "";
  } catch (e) { window.__rc_err = String(e); return ""; }
})()`, obj, siteKey, opts)
}

// v2InvisibleScript renders an invisible v2 widget and executes it. The token
// arrives via the callback (only when the risk check passes silently).
func v2InvisibleScript(siteKey string) string {
	return fmt.Sprintf(`(function(){
  try {
    if (window.__rc_token) return window.__rc_token;
    if (typeof grecaptcha === 'undefined' || !grecaptcha || !grecaptcha.render) return "";
    if (!window.__rc_started) {
      window.__rc_started = true;
      grecaptcha.ready(function(){
        try {
          window.__rc_wid = grecaptcha.render('rc', {
            sitekey: %q,
            size: 'invisible',
            callback: function(t){ window.__rc_token = t; },
            'error-callback': function(){ window.__rc_err = 'recaptcha error-callback'; }
          });
          grecaptcha.execute(window.__rc_wid);
        } catch (e) { window.__rc_err = String(e); }
      });
    }
    return window.__rc_token || "";
  } catch (e) { window.__rc_err = String(e); return ""; }
})()`, siteKey)
}
