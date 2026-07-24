package kasada

import (
	"fmt"
	"strings"
)

// kpsdkReadyScript reports whether the Kasada SDK has initialized on the page.
const kpsdkReadyScript = `(typeof window.KPSDK !== 'undefined') || (document.cookie.indexOf('KP_UIDz') !== -1)`

// siteFlow describes how to harvest Kasada headers for a specific site/page.
type siteFlow struct {
	// NavigateURL is the default page to load (overridable via RequestUrl).
	NavigateURL string
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
			IPSMatch:     "ips.js",
			HarvestMatch: "account-status-validation",
			BuildTrigger: ticketmasterLoginTrigger,
		},
	},
}

const ticketmasterLoginURL = "https://auth.ticketmaster.com/as/authorization.oauth2?client_id=8bf7204a7e97.web.ticketmaster.us&response_type=code&scope=openid%20profile%20phone%20email%20tm&redirect_uri=https://identity.ticketmaster.com/exchange&visualPresets=tm&lang=en-us&placementId=mytmlogin&hideLeftPanel=false&integratorId=prd1741.iccp&intSiteToken=tm-us"

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
