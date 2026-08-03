package browserforward

import (
	"regexp"
	"strings"
)

var chromeVersionRe = regexp.MustCompile(`Chrome/(\d+(?:\.\d+){0,3})`)

// BuildUAMetadata derives Client Hints (userAgentMetadata) from a user-agent
// string so that navigator.userAgentData and the Sec-CH-UA request headers stay
// consistent with the overridden UA. Returns nil for non-Chrome UAs (in which
// case only the UA string is overridden).
//
// The GREASE brand values cannot be validated by servers (they are intentionally
// randomized per Chrome build), so the exact greasing does not matter — internal
// consistency between UA, Client Hints, and userAgentData does.
func BuildUAMetadata(ua string) map[string]any {
	m := chromeVersionRe.FindStringSubmatch(ua)
	if m == nil {
		return nil
	}
	fullVersion := m[1]
	major := fullVersion
	if i := strings.IndexByte(fullVersion, '.'); i >= 0 {
		major = fullVersion[:i]
	}

	brands := []map[string]any{
		{"brand": "Not(A:Brand", "version": "99"},
		{"brand": "Google Chrome", "version": major},
		{"brand": "Chromium", "version": major},
	}
	fullVersionList := []map[string]any{
		{"brand": "Not(A:Brand", "version": "99.0.0.0"},
		{"brand": "Google Chrome", "version": fullVersion},
		{"brand": "Chromium", "version": fullVersion},
	}

	platform, platformVersion := UAPlatform(ua)

	arch, bitness := "x86", "64"
	if platform == "macOS" {
		arch = "arm"
	}

	return map[string]any{
		"brands":          brands,
		"fullVersionList": fullVersionList,
		"fullVersion":     fullVersion,
		"platform":        platform,
		"platformVersion": platformVersion,
		"architecture":    arch,
		"bitness":         bitness,
		"model":           "",
		"mobile":          false,
		"wow64":           false,
	}
}

// NavigatorPlatform maps a UA string to the legacy navigator.platform value
// (e.g. "Win32", "MacIntel"). Keeping this consistent with the UA is important:
// a mismatch (e.g. UA says Windows but navigator.platform is "MacIntel") is a
// trivial automation/spoofing tell.
func NavigatorPlatform(ua string) string {
	p, _ := UAPlatform(ua)
	switch p {
	case "Windows":
		return "Win32"
	case "macOS":
		return "MacIntel"
	case "Android":
		return "Linux armv8l"
	default:
		return "Linux x86_64"
	}
}

// UAPlatform maps a UA string to a Client Hints platform and a plausible
// platformVersion.
func UAPlatform(ua string) (string, string) {
	switch {
	case strings.Contains(ua, "Windows"):
		// Windows froze the UA NT version at 10.0; the Client Hint distinguishes
		// 10 vs 11. 15.0.0 is Windows 11, which is the common case today.
		return "Windows", "15.0.0"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		return "macOS", "14.5.0"
	case strings.Contains(ua, "Android"):
		return "Android", "14.0.0"
	default:
		return "Linux", ""
	}
}
