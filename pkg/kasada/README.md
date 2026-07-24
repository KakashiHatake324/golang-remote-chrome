# Kasada Harvester

Harvest Kasada anti-bot headers (`x-kpsdk-ct`, `x-kpsdk-cd`, `x-kpsdk-v`, …) from a
real headless Chrome, then reuse them in your own HTTP client.

The `Harvester` launches **one** warm Chrome and keeps it alive. Every harvest
runs in its own isolated browser context and forwards all browser traffic
through a per-harvest [tls-client](https://github.com/bogdanfinn/tls-client)
bound to a proxy you choose. Two consequences:

- The token is issued against the **same egress IP** you'll reuse for the real
  API calls (Kasada tokens are IP + UA bound).
- Many harvests can run **concurrently** in the one browser without paying the
  ~5s Chrome launch cost each time.

For supported sites, the harvester loads a lightweight Kasada interstitial
(`/fp`) instead of the full protected page and reads the token straight off the
`ips.js` response headers — so a harvest is small, fast, and parallel-friendly.

---

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/KakashiHatake324/golang-remote-chrome/pkg/kasada"
)

func main() {
	h, err := kasada.NewHarvester(kasada.HarvesterConfig{
		Site:     "ticketmaster",
		PageName: "login",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	res, err := h.Harvest(context.Background(), "142.173.206.135:19948:user:pass")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("took %s\n", res.Elapsed)
	for k, v := range res.Headers {
		fmt.Printf("%s: %s\n", k, v)
	}
}
```

`res.Headers` is ready to splat onto an outgoing request. Send those requests
through **the same proxy** you passed to `Harvest`.

---

## Harvest many at once

The per-harvest time (~18s) is almost entirely network/challenge *waiting*, so
running harvests in parallel multiplies throughput. Give `HarvestMany` one proxy
per token you want:

```go
proxies := []string{
	"142.173.206.135:19948:user:pass",
	"142.173.200.204:18487:user:pass",
	"142.173.200.194:18477:user:pass",
}

results, errs := h.HarvestMany(context.Background(), proxies)
for i := range proxies {
	if errs[i] != nil {
		log.Printf("proxy %d failed: %v", i, errs[i])
		continue
	}
	log.Printf("token %d in %s: %s", i, results[i].Elapsed, results[i].XKpsdkCt)
}
```

`results[i]` / `errs[i]` line up with `proxies[i]`. A per-proxy failure (e.g. a
blocked datacenter IP) is isolated into `errs[i]` and does not fail the batch.
Parallelism is capped by `MaxConcurrency` (default 5).

> Each concurrent harvest is a live Chrome tab. 5–10 is a sane range; benchmark
> before pushing higher, as each tab uses real CPU/memory.

---

## Configuration

`HarvesterConfig`:

| Field | Type | Default | Notes |
|---|---|---|---|
| `Site` | `string` | — | Required. Site key, e.g. `"ticketmaster"`. |
| `PageName` | `string` | — | Required. Page/flow key, e.g. `"login"`. |
| `Email` | `string` | `harvest@example.com` | Only used by trigger-based flows. |
| `UserAgent` | `string` | Chrome 133 UA | Should match `TLSProfile`. |
| `Headless` | `*bool` | `true` | Set to `false` to watch the browser. |
| `Verbose` | `bool` | `false` | Verbose Chrome/CDP logging. |
| `Deadline` | `int` | `45` | Per-harvest timeout, seconds. |
| `BlockResources` | `*bool` | `true` | Drop images/fonts/analytics. Never blocks Kasada. |
| `TLSProfile` | `profiles.ClientProfile` | `Chrome_133` | tls-client fingerprint. |
| `MaxConcurrency` | `int` | `5` | Parallel cap for `HarvestMany`. |

Pointer fields (`Headless`, `BlockResources`) let you distinguish "unset"
(use default) from an explicit `false`:

```go
headless := false
cfg := kasada.HarvesterConfig{
	Site:      "ticketmaster",
	PageName:  "login",
	Headless:  &headless,
	Deadline:  60,
}
```

### `HarvestResult`

| Field | Description |
|---|---|
| `Headers` | All captured `x-kpsdk-*` headers, ready to attach to requests. |
| `XKpsdkCt` | Client token. |
| `XKpsdkCd` | Proof-of-work payload (from the SDK, or computed locally). |
| `XKpsdkV` | SDK version, e.g. `j-1.2.522`. |
| `XKpsdkH` | Per-request signature (only from the trigger flow — see below). |
| `KpsdkST` | Server timestamp used to derive `cd`. |
| `Proxy` | The proxy used for this harvest. |
| `Elapsed` | Wall-clock time for this harvest. |

---

## Proxy formats

`Harvest`/`HarvestMany` accept any of these; all are normalized to a proxy URL
for the TLS client:

```
http://user:pass@host:port      // (or https://, socks5://) passed through
user:pass@host:port
host:port:user:pass
host:port                        // no auth
```

---

## How it works

1. `NewHarvester` launches one headless Chrome with **no** upstream proxy. Every
   request is intercepted at the Fetch `Request` stage.
2. `Harvest(proxy)` creates a fresh isolated browser context (its own cookie
   jar), builds a Chrome-fingerprinted TLS client bound to `proxy`, and navigates
   to the site's lightweight Kasada interstitial.
3. The intercept handler forwards each request through the TLS client (so the
   browser egresses via `proxy`) and fulfills Chrome with the response.
4. When `ips.js` gets its token, the response carries `x-kpsdk-st` +
   `x-kpsdk-ct`. The harvester captures those off the wire and computes
   `x-kpsdk-cd` locally — the harvest completes immediately.
5. The isolated context is disposed; the browser stays warm for the next call.

### Fast path vs. trigger path

- **Fast path (default for supported sites):** loads the `/fp` interstitial and
  reads the token from response headers. Fast, light, concurrency-friendly.
  Produces `ct`, `cd`, `v`. **Does not** produce `x-kpsdk-h`.
- **Trigger path (fallback):** loads the full protected page, waits for the SDK,
  fires a site-specific request, and reads the SDK-injected **request** headers
  (`ct`, `cd`, `v`, `h`). Heavier and slower, but yields `x-kpsdk-h`.

If your target endpoint requires `x-kpsdk-h`, use the trigger path by clearing
the flow's `BootstrapURL` in `sites.go` (or add a config toggle).

---

## Adding a new site

Sites live in `sites.go`. Add an entry to `siteFlows`:

```go
var siteFlows = map[string]map[string]siteFlow{
	"mysite": {
		"checkout": {
			// Full protected page (fallback / trigger path).
			NavigateURL: "https://www.mysite.com/checkout",
			// Optional: lightweight Kasada interstitial for the fast path.
			// Hit any protected path once and copy the ips.js zone path + /fp.
			BootstrapURL: "https://www.mysite.com/<uuid>/<uuid>/fp?x-kpsdk-v=j-1.2.522",
			// Substring identifying the Kasada script (used by the trigger path).
			IPSMatch: "ips.js",
			// Substring identifying the request to harvest (trigger path).
			HarvestMatch: "some-protected-endpoint",
			// JS that fires the trigger request in the page (trigger path).
			BuildTrigger: func(email string) string {
				return `(() => { fetch("/some-protected-endpoint").catch(()=>{}); return true; })()`
			},
		},
	},
}
```

Notes:
- For the **fast path**, only `BootstrapURL` (plus `Site`/`PageName`) is needed.
  Find the `/fp` URL by hitting a protected page once and copying the Kasada zone
  path (the two UUIDs) from the injected `ips.js` `src`.
- For the **trigger path**, `NavigateURL`, `IPSMatch`, `HarvestMatch`, and
  `BuildTrigger` are used.
- The two `/fp` UUIDs are the site's stable Kasada zone. If the site rotates
  them, refresh the constant.

---

## Running the live tests

The tests hit real Chrome and the network, so they're skipped unless
`KASADA_LIVE=1`.

```bash
# Sequential warm-reuse (two harvests):
KASADA_LIVE=1 KASADA_PROXY=host:port:user:pass \
  go test ./pkg/kasada/ -run TestHarvesterWarmReuse -count=1 -v

# Concurrent (one token per proxy):
KASADA_LIVE=1 \
  KASADA_PROXIES="p1,p2,p3,p4,p5" \
  KASADA_MAXCONC=5 \
  go test ./pkg/kasada/ -run TestHarvesterConcurrent -count=1 -v
```

Test env vars:

| Var | Meaning |
|---|---|
| `KASADA_LIVE=1` | Required to run the live tests. |
| `KASADA_PROXY` | Single proxy (warm-reuse test). |
| `KASADA_PROXIES` | Comma-separated proxies (concurrent test). |
| `KASADA_PROXY2` | Second proxy for the warm-reuse test. |
| `KASADA_MAXCONC` | Parallel cap (default 5). |
| `KASADA_DEADLINE` | Per-harvest timeout, seconds. |
| `KASADA_UA` | Override user agent. |
| `KASADA_EMAIL` | Email for trigger-based flows. |
| `KASADA_HEADLESS=0` | Watch the browser. |
| `KASADA_VERBOSE=1` | Verbose logging. |

---

## Gotchas

- **Reuse the harvest proxy** for the downstream API calls. A token minted on
  IP A will be rejected from IP B.
- **Datacenter IPs** are gated hard by Kasada — expect occasional per-proxy
  failures and a higher latency floor. Residential/mobile proxies are faster and
  more reliable.
- **`Close()`** kills the browser. Always `defer h.Close()`.
- **One `Harvester`, many harvests.** Don't create a new `Harvester` per token;
  that throws away the whole point (the warm browser).
