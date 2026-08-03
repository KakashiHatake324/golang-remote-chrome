# reCAPTCHA v3 Harvester

Mints reCAPTCHA **v3** (and Enterprise) tokens from a real headless Chrome. It
reuses the same design as the Kasada harvester: one warm browser, an isolated
context per harvest, and every request forwarded through a per-harvest,
TLS-fingerprinted client bound to your proxy.

> Only token generation is supported (v3 / invisible-style, via
> `grecaptcha.execute`). It does **not** solve reCAPTCHA v2 image challenges.

## How it works

For each harvest:

1. A fresh isolated browser context is created (clean cookies/storage).
2. The browser navigates to `https://<domain>/`. That **top-level document is
   fulfilled locally** with a minimal page that only loads the reCAPTCHA API —
   so the browser's origin genuinely becomes the target domain (tokens are
   domain-bound) without downloading the real site.
3. Every other request (`recaptcha/api.js`, the `anchor`/`reload` frames,
   `gstatic`, …) is **forwarded through your proxy** using a Chrome TLS
   fingerprint, so Google computes the v3 score against the **same egress IP**
   you'll reuse the token with.
4. `grecaptcha.execute(sitekey, {action})` runs and the resulting
   `g-recaptcha-response` token is returned.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/KakashiHatake324/golang-remote-chrome/pkg/recaptcha"
)

func main() {
	h, err := recaptcha.NewHarvester(recaptcha.Config{})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	res, err := h.Harvest(context.Background(), "host:port:user:pass", recaptcha.Target{
		Domain:  "example.com",           // must match the site key's registered domain
		SiteKey: "6Lc...your-v3-key...",
		Action:  "login",                  // optional
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("token (%s): %s\n", res.Elapsed, res.Token)
}
```

## Concurrent harvesting

Mint one token per proxy (all for the same target), bounded by
`MaxConcurrency`:

```go
proxies := []string{"p1...", "p2...", "p3..."}
results, errs := h.HarvestMany(ctx, proxies, target)
for i := range proxies {
	if errs[i] != nil {
		log.Printf("proxy %d failed: %v", i, errs[i])
		continue
	}
	fmt.Println(results[i].Token)
}
```

## Configuration

`Config` (the warm browser, set once):

| Field            | Default          | Purpose                                        |
| ---------------- | ---------------- | ---------------------------------------------- |
| `UserAgent`      | Chrome 133 UA    | UA string; Client Hints are derived to match.  |
| `Headless`       | `true`           | Set `&false` to watch the browser.             |
| `Deadline`       | `30`             | Per-harvest timeout, seconds.                  |
| `BlockResources` | `true`           | Drop heavy resources (images, fonts, media).   |
| `TLSProfile`     | `Chrome_133`     | TLS/HTTP fingerprint of the forwarding client. |
| `MaxConcurrency` | `5`              | Cap on parallel `HarvestMany` harvests.        |
| `ExtraFlags`     | –                | Extra Chrome launch flags.                     |
| `InitScript`     | –                | Extra JS run before page scripts (after stealth). |
| `DisableStealth` | `false`          | Turn off the built-in anti-detection init script. |
| `DisableHumanize`| `false`          | Skip the pre-execute mouse movement + dwell.   |
| `WebGLVendor` / `WebGLRenderer` | – | Spoof WebGL vendor/renderer (for headless Linux/SwiftShader). |

`Target` (per harvest):

| Field        | Purpose                                                     |
| ------------ | ----------------------------------------------------------- |
| `Domain`     | Origin the token binds to (`example.com` or a full URL).    |
| `SiteKey`    | reCAPTCHA site key (`data-sitekey` / render key).           |
| `Action`     | Action name for v3/Enterprise (optional; ignored for v2).   |
| `Version`    | `V3` (default), `V3Enterprise`, or `V2Invisible`.           |
| `Cookies`    | Cookies seeded into the context before navigation (optional). |

### Versions

| `Target.Version`      | API loaded            | Call                             | Verify via        |
| --------------------- | --------------------- | -------------------------------- | ----------------- |
| `V3` (default)        | `api.js?render=key`   | `grecaptcha.execute(key,{action})` | `siteverify`      |
| `V3Enterprise`        | `enterprise.js`       | `grecaptcha.enterprise.execute`  | `createAssessment` |
| `V2Invisible`         | `api.js?render=explicit` | render invisible widget + `execute` | `siteverify`  |

```go
res, err := h.Harvest(ctx, proxy, recaptcha.Target{
	Domain:  "example.com",
	SiteKey: "6Lc...",
	Version: recaptcha.V2Invisible, // or V3 (default) / V3Enterprise
})
```

> `V2Invisible` only returns a token when the risk check passes silently; it does
> **not** solve image challenges.

## Cookies

Each harvest runs in its own isolated context, so cookies never leak between
concurrent harvests.

Seed cookies (e.g. a target-domain session, or a `.google.com` `_GRECAPTCHA`
cookie to look like a returning visitor) via `Target.Cookies`. A cookie with an
empty `Domain` defaults to the target domain:

```go
res, err := h.Harvest(ctx, proxy, recaptcha.Target{
	Domain:  "example.com",
	SiteKey: "6Lc...",
	Cookies: []*chrome.Cookie{
		{Name: "session", Value: "abc123"},                 // -> example.com
		{Name: "_GRECAPTCHA", Value: "09A...", Domain: ".google.com"},
	},
})
```

After a successful harvest, `Result.Cookies` holds the context's full jar
(target-domain cookies plus google.com cookies like `_GRECAPTCHA`), so you can
persist and replay them on the next harvest.

## Getting consistent v3 scores

A v3 token is *score-based*: `success=true` only means the token is valid for the
domain/secret — the **score** (0.0–1.0) is Google's bot verdict, driven by IP
reputation, browser fingerprint, and behavior. The harvester does what it can on
fingerprint and behavior; **IP reputation is the dominant factor and is on you.**

Built in (on by default):

- **Host-OS-matched UA.** The default UA matches the OS you're actually running
  on (macOS/Windows/Linux). Canvas, WebGL, audio, and fonts are produced by the
  real OS and can't be spoofed convincingly — so claiming a *different* OS in the
  UA guarantees mismatches that v3 scores as automation (a common cause of a hard
  `0.00`). If you set a custom `UserAgent`, **match your host OS.**
- **Consistent UA + Client Hints + `navigator.platform`** (`userAgentData` /
  `Sec-CH-UA` / `navigator.platform` all agree with the UA), applied per harvest.
- **Whole-browser `navigator.webdriver=false`** via the launch flag.
- **Stealth init script** (`Config.DisableStealth` to turn off): patches
  `navigator.plugins`/`mimeTypes`/`languages`, `window.chrome`, and
  `permissions.query`. It intentionally leaves WebGL/canvas alone (real GPU values
  are consistent); set `WebGLVendor`/`WebGLRenderer` only on headless Linux where
  WebGL falls back to SwiftShader.
- **Humanization** (`Config.DisableHumanize` to turn off): a short path of
  *trusted* CDP mouse moves plus a dwell before `execute`, so the page isn't
  solved instantly with zero input.

What you must do for good scores:

1. **Use residential/mobile proxies and rotate them.** Datacenter IPs get flagged
   fast, and a single IP's score collapses after a few automated solves. Treat
   each IP as good for roughly one clean token.
2. **Don't hammer one IP.** Space requests out.
3. **Persist and replay the `_GRECAPTCHA` cookie** (via `Result.Cookies` →
   `Target.Cookies`) to look like a returning visitor on the same identity.
4. **Try `Headless: &false`** if scores stay low — headless still has tells beyond
   what the init script covers.

> Reality check: if `success=true` but the score is low, the token *works* — it's
> Google's risk model rating your IP/session. No client-side change fully
> compensates for a burned datacenter IP.

## Verifying a solve

To validate a harvested token end-to-end, call Google's `siteverify` with your
v3 **secret** key. This is a plain server-side call (no proxy):

```go
res, err := h.Harvest(ctx, proxy, target)
if err != nil {
	log.Fatal(err)
}

v, err := res.Verify(ctx, "your-v3-secret-key") // or recaptcha.Verify(ctx, secret, token, remoteIP)
if err != nil {
	log.Fatal(err)
}
fmt.Printf("success=%t score=%.2f action=%q hostname=%q errors=%v\n",
	v.Success, v.Score, v.Action, v.Hostname, v.ErrorCodes)
```

`VerifyResult.Score` is the v3 abuse score in `[0.0, 1.0]` (higher = more likely
human). `success=false` with an `invalid-input-response` /
`browser-error` / hostname-mismatch code usually means the domain doesn't match
the site key, the token expired, or the secret is wrong.

> `siteverify` is for standard reCAPTCHA v3. Enterprise keys are verified via the
> reCAPTCHA Enterprise `createAssessment` API instead.

## Proxy formats

Same as the Kasada harvester:

- `scheme://user:pass@host:port` (`http`/`https`/`socks5`)
- `user:pass@host:port`
- `host:port:user:pass`
- `host:port`

## Running the live test

```bash
RECAPTCHA_LIVE=1 \
RECAPTCHA_DOMAIN=example.com \
RECAPTCHA_SITEKEY=6Lc...your-v3-key... \
RECAPTCHA_ACTION=login \
RECAPTCHA_PROXY=host:port:user:pass \
RECAPTCHA_SECRET=your-v3-secret-key \
go test ./pkg/recaptcha/ -run TestHarvestV3 -v
```

Provide `RECAPTCHA_SECRET` to verify each harvested token via `siteverify` and
log its score (the test fails if a token doesn't verify). Set
`RECAPTCHA_PROXIES=p1,p2,p3` (instead of `RECAPTCHA_PROXY`) to run the concurrent
path, `RECAPTCHA_VERSION=v3-enterprise|v2-invisible` to pick the flavor, and
`RECAPTCHA_HEADLESS=0` to watch it work.

## Gotchas

- **Domain must match the site key.** reCAPTCHA validates the hostname against
  the key's allowed domains. A mismatch yields an "Invalid domain for site key"
  error (surfaced on the deadline) or an unverifiable token.
- **v3 is score-based.** A token always mints, but its score depends on the
  egress IP and behavior — route through the same proxy you'll use it with, and
  use residential/mobile IPs for higher scores.
- **Tokens are short-lived** (~2 minutes) and single-use. Harvest just-in-time.
