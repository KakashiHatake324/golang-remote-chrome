package browserforward

import "strings"

// DefaultBlockedURLs are heavy/irrelevant resources dropped during harvesting to
// lighten the page. Deliberately excludes anything anti-bot related (Kasada
// ips.js/p.js, reCAPTCHA api.js/anchor/reload) so the flow behaves normally.
var DefaultBlockedURLs = []string{
	"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.svg", "*.ico",
	"*.woff", "*.woff2", "*.ttf", "*.otf",
	"*.mp4", "*.webm",
	"*google-analytics.com*", "*googletagmanager.com*",
	"*doubleclick.net*", "*googlesyndication.com*",
	"*facebook.net*", "*facebook.com/tr*",
}

// IsBlockedURL reports whether a URL matches one of the default block patterns.
func IsBlockedURL(rawURL string) bool {
	for _, pat := range DefaultBlockedURLs {
		if MatchWildcard(pat, rawURL) {
			return true
		}
	}
	return false
}

// MatchWildcard does a simple '*'-glob match anchored at both ends.
func MatchWildcard(pattern, s string) bool {
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
