package kasada

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KakashiHatake324/golang-remote-chrome/internal/browserforward"
	"github.com/KakashiHatake324/golang-remote-chrome/pkg/chrome"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// defaultHarvesterUA matches the default TLS fingerprint (Chrome 133) so the
// browser and the forwarding TLS client present a consistent client.
const defaultHarvesterUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

// stealthScript masks the most common automation tells before any page script
// runs, so Kasada's sensor classifies the session as a real browser (needed to
// earn the trusted 1-AA token and the {"reload":true} interrogation result).
// navigator.webdriver is forced true by Chrome whenever a CDP client is attached
// (even with --disable-blink-features=AutomationControlled), so it is redefined
// here; the rest restore surfaces headless Chrome leaves empty/absent. Every
// patch is individually try/caught and idempotent. Injected via
// Page.addScriptToEvaluateOnNewDocument. WebGL/canvas are intentionally left
// alone (real GPU values beat forged ones); OS-specific spoofing is layered on
// top via HarvesterConfig.InitScript.
const stealthScript = `(function(){
  'use strict';
  var def = function(obj, prop, get){
    try { Object.defineProperty(obj, prop, { get: get, configurable: true }); } catch(e){}
  };

  // navigator.webdriver -> false (reinforces --disable-blink-features flag).
  try { def(Navigator.prototype, 'webdriver', function(){ return false; }); } catch(e){}
  try { delete Object.getPrototypeOf(navigator).webdriver; } catch(e){}

  // Scrub ChromeDriver globals.
  try {
    for (var k in window) { if (k.indexOf('cdc_') === 0 || k.indexOf('$cdc_') === 0) { try { delete window[k]; } catch(e){} } }
  } catch(e){}

  // Plausible locale + hardware.
  def(navigator, 'languages', function(){ return ['en-US','en']; });
  def(navigator, 'hardwareConcurrency', function(){ return 8; });

  // Non-empty plugins/mimeTypes (headless reports zero — a strong bot signal).
  try {
    var mkPlugin = function(name, desc, fn){ return { name:name, description:desc, filename:fn, length:1 }; };
    var plugins = [
      mkPlugin('PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('Chrome PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('Chromium PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('Microsoft Edge PDF Viewer','Portable Document Format','internal-pdf-viewer'),
      mkPlugin('WebKit built-in PDF','Portable Document Format','internal-pdf-viewer')
    ];
    plugins.item = function(i){ return this[i]; };
    plugins.namedItem = function(n){ return this.find(function(p){ return p.name===n; }) || null; };
    plugins.refresh = function(){};
    def(navigator, 'plugins', function(){ return plugins; });
    var mimes = [{ type:'application/pdf', suffixes:'pdf', description:'Portable Document Format' }];
    mimes.item = function(i){ return this[i]; };
    mimes.namedItem = function(n){ return this.find(function(m){ return m.type===n; }) || null; };
    def(navigator, 'mimeTypes', function(){ return mimes; });
  } catch(e){}

  // window.chrome runtime object (present in real Chrome, absent in headless).
  try {
    if (!window.chrome) { window.chrome = {}; }
    if (!window.chrome.runtime) { window.chrome.runtime = {}; }
    if (!window.chrome.app) { window.chrome.app = { isInstalled:false, InstallState:{ DISABLED:'disabled', INSTALLED:'installed', NOT_INSTALLED:'not_installed' }, RunningState:{ CANNOT_RUN:'cannot_run', READY_TO_RUN:'ready_to_run', RUNNING:'running' } }; }
    if (!window.chrome.csi) { window.chrome.csi = function(){ return {}; }; }
    if (!window.chrome.loadTimes) { window.chrome.loadTimes = function(){ return {}; }; }
  } catch(e){}

  // permissions.query for notifications should agree with Notification.permission.
  try {
    var q = window.navigator.permissions && window.navigator.permissions.query;
    if (q) {
      window.navigator.permissions.query = function(p){
        if (p && p.name === 'notifications') {
          return Promise.resolve({ state: (typeof Notification !== 'undefined' ? Notification.permission : 'default') });
        }
        return q.apply(window.navigator.permissions, arguments);
      };
    }
  } catch(e){}
})();`

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
	// RequireReload, when true, holds the harvest open until Kasada's /tl
	// interrogation returns {"reload":true} — the strong-trust signal that the
	// issued token passed the full challenge (as opposed to an empty /tl body,
	// which is a weaker "re-interrogate" token). Defaults to false: the harvest
	// completes as soon as x-kpsdk-ct/st are captured, and Reload is reported
	// on the result for the caller to inspect.
	RequireReload bool
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
	// Reload reports whether Kasada's /tl interrogation returned {"reload":true}
	// during this harvest — i.e. the token passed the full challenge (strong
	// trust). False means /tl was never seen or returned the weaker empty body.
	Reload  bool
	Proxy   string
	Elapsed time.Duration
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
	uaMetadata map[string]any
	profile    profiles.ClientProfile
	block      bool
	maxConc    int
	initScript string
	reqReload  bool
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

	// DisableBlinkAutomationControlled drops the AutomationControlled blink
	// feature so navigator.webdriver isn't forced true — the single loudest
	// automation tell that anti-bot SDKs (Kasada included) key on.
	flags := []chrome.FlagType{chrome.NoFirstRun, chrome.NoDefaultBrowserCheck, chrome.DisableBlinkAutomationControlled, chrome.SetUserAgent(ua)}
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
		uaMetadata: browserforward.BuildUAMetadata(ua),
		profile:    profile,
		block:      block,
		maxConc:    maxConc,
		initScript: cfg.InitScript,
		reqReload:  cfg.RequireReload,
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
	// token is issued without downloading the heavy protected page. But the
	// interstitial never fires Kasada's /tl interrogation, so it can't yield the
	// {"reload":true} strong-trust signal — when the caller requires that, load
	// the full protected page instead so the SDK runs its complete lifecycle.
	navURL := h.flow.BootstrapURL
	if h.reqReload && h.flow.NavigateURL != "" {
		navURL = h.flow.NavigateURL
	}
	if navURL == "" {
		navURL = h.flow.NavigateURL
	}
	if navURL == "" {
		return nil, fmt.Errorf("kasada: no navigate URL for site %q page %q", h.cfg.Site, h.cfg.PageName)
	}

	client, err := browserforward.BuildTLSClient(proxy, h.profile, h.cfg.Deadline)
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
	// Keep the UA string, Sec-CH-UA request headers, and navigator.userAgentData
	// consistent. The --user-agent launch flag only sets the UA string, leaving
	// Client Hints reflecting the real Chrome build — a detectable mismatch.
	if err := page.SetUserAgentOverride(h.ua, browserforward.NavigatorPlatform(h.ua), h.uaMetadata); err != nil {
		return nil, fmt.Errorf("kasada: set user agent override: %w", err)
	}
	// Install fingerprint overrides before any document script runs so the
	// anti-bot SDK sees the spoofed navigator/WebGL instead of the host OS.
	// The stealth prelude runs first and always: it masks the CDP-driven
	// navigator.webdriver flag and other automation residue that survive the
	// launch flag when a DevTools client is attached.
	initScript := stealthScript
	if h.initScript != "" {
		initScript += "\n" + h.initScript
	}
	if err := page.AddScriptToEvaluateOnNewDocument(initScript); err != nil {
		return nil, fmt.Errorf("kasada: add init script: %w", err)
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
	reload      bool
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
		Reload:   s.reload,
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
	if s.h.block && browserforward.IsBlockedURL(rawURL) {
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

	status, respHeaders, body, err := browserforward.Forward(s.client, method, rawURL, headers, postData)
	if err != nil {
		if os.Getenv("KASADA_DEBUG_RELOAD") != "" {
			fmt.Printf("[kasada] fwd ERR %s %s: %v\n", method, rawURL, err)
		}
		_ = s.page.FailRequest(requestID, "ConnectionFailed")
		return true
	}
	if os.Getenv("KASADA_DEBUG_RELOAD") != "" {
		fmt.Printf("[kasada] fwd %s %d %s (len=%d)\n", method, status, rawURL, len(body))
	}

	// Kasada issues the token via response headers (x-kpsdk-st + x-kpsdk-ct) on
	// the ips.js telemetry response. Capture it here, straight off the wire.
	s.captureResponse(status, respHeaders)

	// The /tl interrogation POST returns {"reload":true} in its body when the
	// token passed the full challenge (strong trust); an empty body is the
	// weaker "re-interrogate" signal. Record it off the wire.
	s.inspectReload(rawURL, body)

	_ = s.page.FulfillRequest(requestID, status, respHeaders, browserforward.EncodeBody(body))
	return true
}

// reloadTrueMarker is the /tl success body (whitespace-insensitive match below).
var reloadTrueMarker = []byte(`"reload":true`)

// inspectReload flags the strong-trust signal when the Kasada /tl interrogation
// response body is {"reload":true}. Anything else (empty body, {"reload":false})
// leaves it unset.
func (s *harvestSession) inspectReload(rawURL string, body []byte) {
	if !strings.Contains(rawURL, "/tl") || len(body) == 0 || len(body) > 4096 {
		return
	}
	hit := bytes.Contains(bytes.ReplaceAll(body, []byte(" "), nil), reloadTrueMarker)
	if os.Getenv("KASADA_DEBUG_RELOAD") != "" {
		b := body
		if len(b) > 160 {
			b = b[:160]
		}
		fmt.Printf("[kasada] /tl body reload=%v: %s\n", hit, string(b))
	}
	if !hit {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload = true
	s.harvested["x-kpsdk-reload"] = "true"
	s.tryComplete()
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

	s.tryComplete()
}

// tryComplete marks the harvest done once the required signals are present:
// always x-kpsdk-ct plus a token timestamp/cd, and — when RequireReload is set —
// the /tl {"reload":true} strong-trust signal too. Caller must hold s.mu.
func (s *harvestSession) tryComplete() {
	if s.harvestDone {
		return
	}
	if s.xct == "" || (s.kpsdkST == 0 && s.xcd == "") {
		return
	}
	if s.h.reqReload && !s.reload {
		return
	}
	if s.xcd == "" && s.kpsdkST != 0 {
		s.xcd = GenerateCD(s.kpsdkST)
		s.harvested["x-kpsdk-cd"] = s.xcd
	}
	s.harvestDone = true
	s.harvestOnce.Do(func() { close(s.harvestedCh) })
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

	s.tryComplete()
}
