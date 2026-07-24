package kasada

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHarvesterWarmReuse launches one warm Chrome and harvests Ticketmaster
// login headers twice (sequentially), each in its own isolated browser context,
// forwarding all browser traffic through a per-harvest TLS client bound to the
// given proxy.
//
// Skipped unless KASADA_LIVE=1.
//
// Env vars: see TestHarvesterConcurrent below.
func TestHarvesterWarmReuse(t *testing.T) {
	if os.Getenv("KASADA_LIVE") != "1" {
		t.Skip("set KASADA_LIVE=1 to run the live warm-browser harvest test")
	}

	h := newTestHarvester(t)
	defer h.Close()

	proxy1 := os.Getenv("KASADA_PROXY")
	proxy2 := envOr("KASADA_PROXY2", proxy1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res1, err := h.Harvest(ctx, proxy1)
	if err != nil {
		t.Fatalf("Harvest #1 error = %v", err)
	}
	t.Logf("harvest #1 (%s) took %s: %#v", proxy1, res1.Elapsed, res1.Headers)
	assertHarvest(t, "#1", res1)

	res2, err := h.Harvest(ctx, proxy2)
	if err != nil {
		t.Fatalf("Harvest #2 error = %v", err)
	}
	t.Logf("harvest #2 (%s) took %s: %#v", proxy2, res2.Elapsed, res2.Headers)
	assertHarvest(t, "#2", res2)
}

// TestHarvesterConcurrent harvests one token per proxy in parallel in a single
// warm browser, each in its own isolated context. It reports wall-clock vs the
// summed per-harvest time to show the throughput win.
//
// Skipped unless KASADA_LIVE=1.
//
// Env vars:
//
//	KASADA_LIVE=1                          enable this test
//	KASADA_PROXIES=p1,p2,p3                comma-separated proxies (one token each)
//	KASADA_PROXY=ip:port:user:pass         fallback single proxy if PROXIES unset
//	KASADA_MAXCONC=5                       max parallel harvests (default 5)
//	KASADA_UA / KASADA_EMAIL / KASADA_HEADLESS / KASADA_DEADLINE / KASADA_VERBOSE
func TestHarvesterConcurrent(t *testing.T) {
	if os.Getenv("KASADA_LIVE") != "1" {
		t.Skip("set KASADA_LIVE=1 to run the live concurrent harvest test")
	}

	proxies := splitProxies(os.Getenv("KASADA_PROXIES"))
	if len(proxies) == 0 {
		proxies = []string{os.Getenv("KASADA_PROXY")}
	}

	h := newTestHarvester(t)
	defer h.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	results, errs := h.HarvestMany(ctx, proxies)
	wall := time.Since(start)

	var summed time.Duration
	ok := 0
	for i := range proxies {
		if errs[i] != nil {
			t.Errorf("harvest %d (%s) error = %v", i, proxies[i], errs[i])
			continue
		}
		summed += results[i].Elapsed
		ok++
		t.Logf("harvest %d (%s) took %s ct=%.12s...", i, proxies[i], results[i].Elapsed, results[i].XKpsdkCt)
		assertHarvest(t, "concurrent", results[i])
	}
	t.Logf("harvested %d/%d tokens in %s wall-clock (summed sequential would be %s)", ok, len(proxies), wall, summed)
}

func newTestHarvester(t *testing.T) *Harvester {
	t.Helper()

	deadline := 60
	if v := os.Getenv("KASADA_DEADLINE"); v != "" {
		if n, err := time.ParseDuration(v + "s"); err == nil {
			deadline = int(n.Seconds())
		}
	}
	headless := true
	if os.Getenv("KASADA_HEADLESS") == "0" {
		headless = false
	}
	maxConc := 5
	if v := os.Getenv("KASADA_MAXCONC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConc = n
		}
	}

	launchStart := time.Now()
	h, err := NewHarvester(HarvesterConfig{
		Site:           "ticketmaster",
		PageName:       "login",
		Email:          envOr("KASADA_EMAIL", "harvest@example.com"),
		UserAgent:      os.Getenv("KASADA_UA"),
		Headless:       &headless,
		Verbose:        os.Getenv("KASADA_VERBOSE") == "1",
		Deadline:       deadline,
		MaxConcurrency: maxConc,
	})
	if err != nil {
		t.Fatalf("NewHarvester() error = %v", err)
	}
	t.Logf("browser launched in %s", time.Since(launchStart))
	return h
}

func splitProxies(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func assertHarvest(t *testing.T, label string, res *HarvestResult) {
	t.Helper()
	if res.XKpsdkCt == "" {
		t.Errorf("harvest %s: expected x-kpsdk-ct, got empty", label)
	}
	if res.XKpsdkCd == "" {
		t.Errorf("harvest %s: expected x-kpsdk-cd, got empty", label)
	}
	if len(res.Headers) == 0 {
		t.Errorf("harvest %s: expected at least one x-kpsdk-* header", label)
	}
}
