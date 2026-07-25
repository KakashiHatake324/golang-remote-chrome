package kasada

import (
	"fmt"
	"strings"
)

// kpsdkReadyScript reports whether the Kasada SDK has initialized on the page.
const kpsdkReadyScript = `(typeof window.KPSDK !== 'undefined') || (document.cookie.indexOf('KP_UIDz') !== -1)`

// siteFlow describes how to harvest Kasada headers for a specific site/page.
type siteFlow struct {
	// NavigateURL is the full protected page to load (heavy; used as fallback).
	NavigateURL string
	// BootstrapURL, when set, is a lightweight Kasada interstitial ("/fp") on the
	// target origin that only loads ips.js. Loading it instead of the full page
	// keeps per-session traffic tiny, which is what makes concurrent harvesting
	// viable. The Kasada token is issued by ips.js regardless of the page around
	// it, and is captured from response headers.
	BootstrapURL string
	// IPSMatch is a substring identifying the Kasada script response URL.
	IPSMatch string
	// HarvestMatch is a substring identifying the trigger request to harvest.
	HarvestMatch string
	// BuildTrigger returns JS that performs the trigger request in the page.
	BuildTrigger func(email string) string
}

// siteFlows maps "site" -> "page" -> flow.
var siteFlows = map[string]map[string]siteFlow{
	"ticketmaster": {
		"login": {
			NavigateURL:  ticketmasterLoginURL,
			BootstrapURL: ticketmasterBootstrapURL,
			IPSMatch:     "ips.js",
			HarvestMatch: "account-status-validation",
			BuildTrigger: ticketmasterLoginTrigger,
		},
	},
}

const ticketmasterLoginURL = "https://auth.ticketmaster.com/as/authorization.oauth2?client_id=8bf7204a7e97.web.ticketmaster.us&response_type=code&scope=openid%20profile%20phone%20email%20tm&redirect_uri=https://identity.ticketmaster.com/exchange&visualPresets=tm&lang=en-us&placementId=mytmlogin&hideLeftPanel=false&integratorId=prd1741.iccp&intSiteToken=tm-us"

// ticketmasterBootstrapURL is the Kasada interstitial ("/fp") for TM's auth
// origin. The two UUIDs are TM's stable Kasada zone path; hitting it with only
// x-kpsdk-v returns a fresh challenge (fresh KP_UIDz/x-kpsdk-im embedded in the
// ips.js <script src>), so ips.js bootstraps and issues a token standalone.
const ticketmasterBootstrapURL = "https://auth.ticketmaster.com/149e9513-01fa-4fb0-aad4-566afd725d1b/2d206a39-8ed7-437e-a3be-862e0f06eea3/fp?x-kpsdk-v=j-1.2.522"

// resolveSiteFlow looks up the flow for a site/page combination.
func resolveSiteFlow(site, page string) (siteFlow, error) {
	pages, ok := siteFlows[strings.ToLower(strings.TrimSpace(site))]
	if !ok {
		return siteFlow{}, fmt.Errorf("kasada: unsupported site %q", site)
	}
	flow, ok := pages[strings.ToLower(strings.TrimSpace(page))]
	if !ok {
		return siteFlow{}, fmt.Errorf("kasada: unsupported page %q for site %q", page, site)
	}
	return flow, nil
}

// ticketmasterLoginTrigger fires the account-status-validation request. The KP
// SDK intercepts this fetch and injects the x-kpsdk-* headers we harvest.
func ticketmasterLoginTrigger(email string) string {
	if email == "" {
		email = "harvest@example.com"
	}
	return fmt.Sprintf(`
	(() => {
		try {
			fetch("https://auth.ticketmaster.com/json/account-status-validation", {
				method: "POST",
				credentials: "include",
				headers: {
					"content-type": "application/json",
					"accept": "*/*",
					"accept-language": "en-us",
					"tm-site-token": "tm-us",
					"tm-client-id": "8bf7204a7e97.web.ticketmaster.us",
					"tm-integrator-id": "prd1741.iccp",
					"tm-oauth-type": "tm-auth",
					"tm-placement-id": "mytmlogin"
				},
				body: JSON.stringify({ email: %q, siteToken: "tm-us" })
			}).catch(() => {});
		} catch (e) {}
		return true;
	})()`, email)
}
