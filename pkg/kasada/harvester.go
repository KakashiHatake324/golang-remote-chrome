package kasada

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// defaultHarvesterUA matches the default TLS fingerprint (Chrome 133) so the
// browser and the forwarding TLS client present a consistent client.
const defaultHarvesterUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

// HarvesterConfig configures a reusable Kasada Harvester.
type HarvesterConfig struct {
	// Site/PageName select the harvest flow (see sites.go).
	Site     string
	PageName string
	// Email is used by trigger requests that require one.
	Email string
	// UserAgent overrides the browser UA. Defaults to a Chrome 133 UA.
	UserAgent string
	// Headless controls whether Chrome runs headless. Defaults to true.
	Headless *bool
	// Verbose enables verbose Chrome logging.
	Verbose bool
	// Deadline is the per-harvest timeout in seconds. Defaults to 45.
	Deadline int
	// BlockResources drops heavy/irrelevant resources during harvesting.
	// Defaults to true. Never blocks Kasada scripts or the trigger endpoint.
	BlockResources *bool
	// TLSProfile is the TLS/HTTP fingerprint used by the forwarding client.
	// Defaults to Chrome 133.
	TLSProfile profiles.ClientProfile
	// MaxConcurrency caps how many harvests run in parallel in HarvestMany.
	// Defaults to 5.
	MaxConcurrency int
	// InitScript, if set, is evaluated in every harvest document before the
	// page's own scripts run (via Page.addScriptToEvaluateOnNewDocument). Use it
	// to install navigator / WebGL / userAgentData overrides so the JS-visible
	// fingerprint matches the spoofed UA — important when the host OS (e.g. Linux
	// in a container) differs from the identity the UA claims.
	InitScript string
	// ExtraFlags are appended to the Chrome launch flags.
	ExtraFlags []chrome.FlagType
}

// HarvestResult holds the headers captured for a single harvest.
type HarvestResult struct {
	Headers  map[string]string
	XKpsdkCt string
	XKpsdkCd string
	XKpsdkV  string
	XKpsdkH  string
	KpsdkST  int64
	Proxy    string
	Elapsed  time.Duration
}

// Harvester keeps a single warm Chrome instance and harvests Kasada headers on
// demand. Each harvest runs in its own isolated browser context (separate
// cookie/storage jar) and forwards every browser request through a per-harvest
// fingerprinted TLS client bound to the chosen proxy. This means the harvested
// token is issued against the same egress IP you will reuse for the real API
// calls, harvests are fully isolated from one another, and many can run
// concurrently in the one warm browser without paying the launch cost each time.
//
// Usage:
//
//	h, _ := kasada.NewHarvester(kasada.HarvesterConfig{Site: "ticketmaster", PageName: "login"})
//	defer h.Close()
//	res, _ := h.Harvest(ctx, "user:pass@host:port")
//	// or, for throughput, run many at once:
//	results, errs := h.HarvestMany(ctx, proxies)
type Harvester struct {
	cfg        HarvesterConfig
	flow       siteFlow
	browser    *chrome.Browser
	ua         string
	profile    profiles.ClientProfile
	block      bool
	maxConc    int
	initScript string
}

// NewHarvester launches one warm Chrome. The browser makes no direct network
// connections: every request is intercepted at the Request stage and fulfilled
// from a per-harvest TLS client, so no upstream proxy is configured on Chrome.
func NewHarvester(cfg HarvesterConfig) (*Harvester, error) {
	flow, err := resolveSiteFlow(cfg.Site, cfg.PageName)
	if err != nil {
		return nil, err
	}

	if cfg.Deadline <= 0 {
		cfg.Deadline = 45
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
		ua = defaultHarvesterUA
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
	// No proxy on Chrome — all egress happens via the per-harvest TLS client.
	opts, err := chrome.NewOptions(&ctx, "", headless, "", "", cfg.Verbose, true, nil, true, false)
	if err != nil {
		return nil, fmt.Errorf("kasada: create options: %w", err)
	}

	flags := []chrome.FlagType{chrome.NoFirstRun, chrome.NoDefaultBrowserCheck, chrome.SetUserAgent(ua)}
	// Containers run Chrome as root with a small /dev/shm; the sandbox can't
	// initialize there and Chrome refuses to start without these on Linux.
	if runtime.GOOS == "linux" {
		flags = append(flags, chrome.NoSandbox, chrome.DisableDevSHM)
	}
	flags = append(flags, cfg.ExtraFlags...)

	browser, err := chrome.LaunchChrome("about:blank", opts, flags)
	if err != nil {
		return nil, fmt.Errorf("kasada: launch chrome: %w", err)
	}

	return &Harvester{
		cfg:        cfg,
		flow:       flow,
		browser:    browser,
		ua:         ua,
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

// Harvest performs a single harvest through the given proxy in its own isolated
// browser context, reusing the warm browser process.
func (h *Harvester) Harvest(ctx context.Context, proxy string) (*HarvestResult, error) {
	return h.harvestOne(ctx, proxy)
}

// HarvestMany harvests one token per proxy concurrently (bounded by
// MaxConcurrency), each in its own isolated context. results[i]/errs[i]
// correspond to proxies[i]; on success errs[i] is nil, on failure results[i] is
// nil. Since each harvest is dominated by network/challenge wait time, running
// them in parallel multiplies throughput without changing per-harvest latency.
func (h *Harvester) HarvestMany(ctx context.Context, proxies []string) ([]*HarvestResult, []error) {
	results := make([]*HarvestResult, len(proxies))
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

			res, err := h.harvestOne(ctx, proxy)
			results[i], errs[i] = res, err
		}(i, proxy)
	}
	wg.Wait()
	return results, errs
}

// harvestOne runs the full flow for a single proxy in a fresh isolated context.
func (h *Harvester) harvestOne(ctx context.Context, proxy string) (result *HarvestResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Prefer the lightweight Kasada interstitial: it only loads ips.js, so the
	// token is issued without downloading the heavy protected page.
	navURL := h.flow.BootstrapURL
	if navURL == "" {
		navURL = h.flow.NavigateURL
	}
	if navURL == "" {
		return nil, fmt.Errorf("kasada: no navigate URL for site %q page %q", h.cfg.Site, h.cfg.PageName)
	}

	client, err := buildTLSClient(proxy, h.profile, h.cfg.Deadline)
	if err != nil {
		return nil, err
	}

	page, contextID, err := h.browser.NewIsolatedPage()
	if err != nil {
		return nil, fmt.Errorf("kasada: new isolated page: %w", err)
	}
	defer func() {
		_ = h.browser.DisposeContext(contextID)
		_ = h.browser.ClosePage(page)
	}()

	if err := page.EnablePage(); err != nil {
		return nil, fmt.Errorf("kasada: enable page: %w", err)
	}
	// Install fingerprint overrides before any document script runs so the
	// anti-bot SDK sees the spoofed navigator/WebGL instead of the host OS.
	if h.initScript != "" {
		if err := page.AddScriptToEvaluateOnNewDocument(h.initScript); err != nil {
			return nil, fmt.Errorf("kasada: add init script: %w", err)
		}
	}
	if err := page.EnableNetwork(); err != nil {
		return nil, fmt.Errorf("kasada: enable network: %w", err)
	}
	if err := page.EnableRequestInterceptionExact([]map[string]any{
		chrome.CreateRequestPattern("*", "Request"),
	}); err != nil {
		return nil, fmt.Errorf("kasada: enable request interception: %w", err)
	}

	hctx, cancel := context.WithTimeout(ctx, time.Duration(h.cfg.Deadline)*time.Second)
	defer cancel()
	page.WithContext(hctx)

	s := &harvestSession{
		h:           h,
		page:        page,
		client:      client,
		harvested:   map[string]string{},
		harvestedCh: make(chan struct{}),
		ipsCh:       make(chan struct{}),
	}
	page.SetRequestInterceptHandler(s.handleIntercept)

	start := time.Now()
	if err := page.Navigate(navURL); err != nil {
		return nil, fmt.Errorf("kasada: navigate: %w", err)
	}

	if err := s.run(ctx, hctx); err != nil {
		return nil, err
	}
	return s.result(proxy, time.Since(start)), nil
}

// harvestSession holds all per-harvest state for a single isolated context. Its
// handleIntercept runs on many concurrent goroutines (one per paused request)
// and only ever uses fire-and-forget page calls, so it cannot race the run
// loop's Evaluate calls on this page's response channel.
type harvestSession struct {
	h      *Harvester
	page   *chrome.Page
	client tls_client.HttpClient

	mu          sync.Mutex
	harvested   map[string]string
	xct         string
	xcd         string
	xv          string
	xh          string
	kpsdkST     int64
	harvestDone bool
	harvestedCh chan struct{}
	harvestOnce sync.Once
	ipsCh       chan struct{}
	ipsOnce     sync.Once
}

// run drives the page: wait for the Kasada script, probe SDK readiness, fire the
// trigger, and block until the headers are harvested (or the deadline hits).
func (s *harvestSession) run(parentCtx, hctx context.Context) error {
	triggerScript := s.h.flow.BuildTrigger(s.h.cfg.Email)
	triggered := false
	var ipsSeenAt time.Time
	ipsChan := s.ipsCh

	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		done := s.harvestDone
		s.mu.Unlock()
		if done {
			return nil
		}

		select {
		case <-parentCtx.Done():
			return errors.New("kasada: parent context cancelled")
		case <-hctx.Done():
			return errors.New("kasada: harvest deadline exceeded")
		case <-s.harvestedCh:
			continue
		case <-ipsChan:
			ipsSeenAt = time.Now()
			ipsChan = nil
		case <-ticker.C:
		}

		if !triggered && !ipsSeenAt.IsZero() {
			ready := false
			if r, e := s.page.Evaluate(kpsdkReadyScript); e == nil {
				ready = r.BoolValueOrDefault()
			}
			if ready || time.Since(ipsSeenAt) > 1500*time.Millisecond {
				if _, e := s.page.Evaluate(triggerScript); e != nil {
					return fmt.Errorf("kasada: trigger request: %w", e)
				}
				triggered = true
			}
		}
	}
}

// result assembles the HarvestResult, synthesizing x-kpsdk-cd only if the SDK
// did not provide one but a server timestamp was captured.
func (s *harvestSession) result(proxy string, elapsed time.Duration) *HarvestResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.xcd == "" && s.kpsdkST != 0 {
		s.xcd = GenerateCD(s.kpsdkST)
	}

	res := &HarvestResult{
		Headers:  map[string]string{},
		XKpsdkCt: s.xct,
		XKpsdkCd: s.xcd,
		XKpsdkV:  s.xv,
		XKpsdkH:  s.xh,
		KpsdkST:  s.kpsdkST,
		Proxy:    proxy,
		Elapsed:  elapsed,
	}
	for k, v := range s.harvested {
		res.Headers[k] = v
	}
	if res.XKpsdkCt != "" {
		res.Headers["x-kpsdk-ct"] = res.XKpsdkCt
	}
	if res.XKpsdkCd != "" {
		res.Headers["x-kpsdk-cd"] = res.XKpsdkCd
	}
	if res.XKpsdkV != "" {
		res.Headers["x-kpsdk-v"] = res.XKpsdkV
	}
	if res.XKpsdkH != "" {
		res.Headers["x-kpsdk-h"] = res.XKpsdkH
	}
	return res
}

// handleIntercept is the synchronous per-request forwarder for this session.
func (s *harvestSession) handleIntercept(params map[string]any) bool {
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

	// 1. The trigger request: harvest the SDK-injected headers, then abort so
	// nothing is actually submitted.
	if s.h.flow.HarvestMatch != "" && strings.Contains(rawURL, s.h.flow.HarvestMatch) {
		s.captureHarvest(headers)
		_ = s.page.FailRequest(requestID, "Aborted")
		return true
	}

	// 2. Heavy/irrelevant resources: drop them.
	if s.h.block && isBlockedURL(rawURL) {
		_ = s.page.FailRequest(requestID, "BlockedByClient")
		return true
	}

	// 3. Signal that the Kasada script is being delivered.
	if s.h.flow.IPSMatch != "" && strings.Contains(rawURL, s.h.flow.IPSMatch) {
		s.ipsOnce.Do(func() { close(s.ipsCh) })
	}

	// 4. Forward everything else through the TLS client / proxy.
	if s.client == nil {
		return false
	}

	status, respHeaders, body, err := forward(s.client, method, rawURL, headers, postData)
	if err != nil {
		_ = s.page.FailRequest(requestID, "ConnectionFailed")
		return true
	}

	// Kasada issues the token via response headers (x-kpsdk-st + x-kpsdk-ct) on
	// the ips.js telemetry response. Capture it here, straight off the wire.
	s.captureResponse(status, respHeaders)

	_ = s.page.FulfillRequest(requestID, status, respHeaders, encodeBody(body))
	return true
}

// captureResponse harvests the server-issued token from a forwarded response.
// This mirrors browser-go's Kasada solver and completes the harvest as soon as
// ips.js gets its token, without needing to fire the trigger request.
func (s *harvestSession) captureResponse(status int, respHeaders []map[string]string) {
	var st, ct string
	for _, hdr := range respHeaders {
		switch strings.ToLower(hdr["name"]) {
		case "x-kpsdk-st":
			st = hdr["value"]
		case "x-kpsdk-ct":
			ct = hdr["value"]
		}
	}
	if st == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// A 429 carrying x-kpsdk-st means the challenge failed / proxy is blocked.
	if status == 429 {
		return
	}

	if n, err := strconv.ParseInt(st, 10, 64); err == nil {
		s.kpsdkST = n
	}
	if ct != "" {
		s.xct = ct
		s.harvested["x-kpsdk-ct"] = ct
	}
	s.harvested["x-kpsdk-st"] = st

	if s.xct != "" && s.kpsdkST != 0 && !s.harvestDone {
		if s.xcd == "" {
			s.xcd = GenerateCD(s.kpsdkST)
			s.harvested["x-kpsdk-cd"] = s.xcd
		}
		s.harvestDone = true
		s.harvestOnce.Do(func() { close(s.harvestedCh) })
	}
}

// captureHarvest records every x-kpsdk-* header from the trigger request and
// signals completion once the essential ct/cd pair is present.
func (s *harvestSession) captureHarvest(headers map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range headers {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-kpsdk-") {
			s.harvested[lk] = fmt.Sprint(v)
		}
	}
	s.xct = firstNonEmpty(s.harvested["x-kpsdk-ct"], s.xct)
	s.xcd = firstNonEmpty(s.harvested["x-kpsdk-cd"], s.xcd)
	s.xv = firstNonEmpty(s.harvested["x-kpsdk-v"], s.xv)
	s.xh = firstNonEmpty(s.harvested["x-kpsdk-h"], s.xh)
	if st := s.harvested["x-kpsdk-st"]; st != "" {
		if n, err := strconv.ParseInt(st, 10, 64); err == nil {
			s.kpsdkST = n
		}
	}

	if s.xct != "" && s.xcd != "" && !s.harvestDone {
		s.harvestDone = true
		s.harvestOnce.Do(func() { close(s.harvestedCh) })
	}
}

// isBlockedURL reports whether a URL matches one of the default block patterns.
func isBlockedURL(rawURL string) bool {
	for _, pat := range defaultBlockedURLs {
		if matchWildcard(pat, rawURL) {
			return true
		}
	}
	return false
}

// matchWildcard does a simple '*'-glob match anchored at both ends.
func matchWildcard(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	if !strings.HasSuffix(pattern, "*") {
		lastPart := parts[len(parts)-1]
		return strings.HasSuffix(s, lastPart)
	}
	return true
}
