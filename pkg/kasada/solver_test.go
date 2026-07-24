package kasada

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestHarvestTicketmasterLogin performs a live harvest against Ticketmaster's
// login flow. It launches a real Chrome and hits the network, so it is skipped
// unless KASADA_LIVE=1 is set.
//
// Optional env vars:
//
//	KASADA_LIVE=1                     enable this test
//	KASADA_PROXY=ip:port:user:pass    proxy (recommended to avoid blocks)
//	KASADA_UA="Mozilla/5.0 ..."       user agent
//	KASADA_EMAIL=you@example.com      email for the trigger request
//	KASADA_HEADLESS=0                 set to 0 to watch the browser
//	KASADA_DEADLINE=60                seconds before giving up
func TestHarvestTicketmasterLogin(t *testing.T) {
	if os.Getenv("KASADA_LIVE") != "1" {
		t.Skip("set KASADA_LIVE=1 to run the live Ticketmaster harvest test")
	}

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

	ua := os.Getenv("KASADA_UA")
	if ua == "" {
		ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(deadline+30)*time.Second)
	defer cancel()

	solver := &SolveKasada{
		Context:     ctx,
		Site:        "ticketmaster",
		PageName:    "login",
		Email:       envOr("KASADA_EMAIL", "harvest@example.com"),
		UserAgent:   ua,
		ProxyString: os.Getenv("KASADA_PROXY"),
		Deadline:    deadline,
		Headless:    &headless,
		Verbose:     true,
	}

	if err := solver.HandleKasada(); err != nil {
		t.Fatalf("HandleKasada() error = %v", err)
	}

	headers := solver.Headers()
	t.Logf("harvested headers: %#v", headers)

	if solver.XKpsdkCt == "" {
		t.Errorf("expected x-kpsdk-ct to be harvested, got empty")
	}
	if solver.XKpsdkCd == "" {
		t.Errorf("expected x-kpsdk-cd to be harvested, got empty")
	}
	if solver.XKpsdkV == "" {
		t.Errorf("expected x-kpsdk-v to be harvested, got empty")
	}

	if len(solver.Harvested) == 0 {
		t.Errorf("expected at least one x-kpsdk-* header in Harvested map")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
