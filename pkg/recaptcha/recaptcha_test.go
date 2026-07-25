package recaptcha

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHarvestV3 mints a reCAPTCHA v3 token for a target you provide. It launches
// real Chrome and hits the network, so it is skipped unless RECAPTCHA_LIVE=1.
//
// Env vars:
//
//	RECAPTCHA_LIVE=1                    enable this test
//	RECAPTCHA_DOMAIN=example.com        domain the site key is registered for
//	RECAPTCHA_SITEKEY=6Lc...            reCAPTCHA v3 site key
//	RECAPTCHA_ACTION=login              v3 action (optional)
//	RECAPTCHA_ENTERPRISE=1              use the Enterprise API (optional)
//	RECAPTCHA_PROXY=host:port:user:pass proxy (recommended)
//	RECAPTCHA_PROXIES=p1,p2,...         run one token per proxy concurrently
//	RECAPTCHA_HEADLESS=0                watch the browser
//	RECAPTCHA_DEADLINE=30               per-harvest timeout, seconds
func TestHarvestV3(t *testing.T) {
	if os.Getenv("RECAPTCHA_LIVE") != "1" {
		t.Skip("set RECAPTCHA_LIVE=1 to run the live reCAPTCHA harvest test")
	}
	domain := os.Getenv("RECAPTCHA_DOMAIN")
	siteKey := os.Getenv("RECAPTCHA_SITEKEY")
	if domain == "" || siteKey == "" {
		t.Skip("set RECAPTCHA_DOMAIN and RECAPTCHA_SITEKEY to run this test")
	}

	deadline := 30
	if v := os.Getenv("RECAPTCHA_DEADLINE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			deadline = n
		}
	}
	headless := true
	if os.Getenv("RECAPTCHA_HEADLESS") == "0" {
		headless = false
	}

	h, err := NewHarvester(Config{
		Headless: &headless,
		Verbose:  os.Getenv("RECAPTCHA_VERBOSE") == "1",
		Deadline: deadline,
	})
	if err != nil {
		t.Fatalf("NewHarvester() error = %v", err)
	}
	defer h.Close()

	target := Target{
		Domain:     domain,
		SiteKey:    siteKey,
		Action:     os.Getenv("RECAPTCHA_ACTION"),
		Enterprise: os.Getenv("RECAPTCHA_ENTERPRISE") == "1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	proxies := splitCSV(os.Getenv("RECAPTCHA_PROXIES"))
	if len(proxies) == 0 {
		proxies = []string{os.Getenv("RECAPTCHA_PROXY")}
	}

	if len(proxies) == 1 {
		res, err := h.Harvest(ctx, proxies[0], target)
		if err != nil {
			t.Fatalf("Harvest error = %v", err)
		}
		t.Logf("token (%d chars) in %s: %.24s...", len(res.Token), res.Elapsed, res.Token)
		if res.Token == "" {
			t.Error("expected a token, got empty")
		}
		return
	}

	start := time.Now()
	results, errs := h.HarvestMany(ctx, proxies, target)
	wall := time.Since(start)
	ok := 0
	for i := range proxies {
		if errs[i] != nil {
			t.Errorf("token %d (%s) error = %v", i, proxies[i], errs[i])
			continue
		}
		ok++
		t.Logf("token %d in %s: %.24s...", i, results[i].Elapsed, results[i].Token)
	}
	t.Logf("harvested %d/%d tokens in %s wall-clock", ok, len(proxies), wall)
}

func splitCSV(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
