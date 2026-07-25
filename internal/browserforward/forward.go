// Package browserforward provides the shared machinery for browser-based token
// harvesters: forwarding intercepted browser requests through a fingerprinted
// TLS client bound to a proxy, plus proxy formatting and Client-Hint metadata.
// It is used by the Kasada and reCAPTCHA harvesters.
package browserforward

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// hopByHopResponseHeaders are response headers that must not be forwarded back
// into Chrome verbatim. content-encoding/content-length are dropped because the
// TLS client already decompresses the body, so the bytes we hand to Chrome are
// plain and their length differs from the origin's.
var hopByHopResponseHeaders = map[string]struct{}{
	"content-encoding":  {},
	"content-length":    {},
	"transfer-encoding": {},
	"connection":        {},
	"keep-alive":        {},
	"proxy-connection":  {},
}

// hopByHopRequestHeaders are request headers we should not forward as-is.
var hopByHopRequestHeaders = map[string]struct{}{
	"content-length":    {},
	"connection":        {},
	"proxy-connection":  {},
	"transfer-encoding": {},
}

// BuildTLSClient constructs a fingerprinted HTTP client that egresses through
// the given proxy. Redirects are NOT followed so that 3xx responses are handed
// straight back to Chrome, which then re-issues (and we re-intercept) the
// follow-up request. This keeps Chrome's cookie/redirect handling authoritative.
func BuildTLSClient(proxy string, profile profiles.ClientProfile, timeoutSeconds int) (tls_client.HttpClient, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	opts := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profile),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithTimeoutSeconds(timeoutSeconds),
	}

	if proxy != "" {
		proxyURL, err := FormatProxy(proxy)
		if err != nil {
			return nil, err
		}
		opts = append(opts, tls_client.WithProxyUrl(proxyURL))
	}

	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
}

// FormatProxy normalizes the many proxy string shapes into a URL the TLS client
// accepts (scheme://user:pass@host:port). Accepts:
//   - http://user:pass@host:port (or https/socks5) — returned unchanged
//   - user:pass@host:port
//   - host:port:user:pass
//   - host:port
func FormatProxy(proxy string) (string, error) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return "", fmt.Errorf("browserforward: empty proxy")
	}

	if strings.Contains(proxy, "://") {
		if _, err := url.Parse(proxy); err != nil {
			return "", fmt.Errorf("browserforward: invalid proxy url %q: %w", proxy, err)
		}
		return proxy, nil
	}

	if strings.Contains(proxy, "@") {
		return "http://" + proxy, nil
	}

	parts := strings.Split(proxy, ":")
	switch len(parts) {
	case 2: // host:port
		return fmt.Sprintf("http://%s:%s", parts[0], parts[1]), nil
	case 4: // host:port:user:pass
		return fmt.Sprintf("http://%s:%s@%s:%s", parts[2], parts[3], parts[0], parts[1]), nil
	default:
		return "", fmt.Errorf("browserforward: unrecognized proxy format %q", proxy)
	}
}

// Forward performs the intercepted request through the given TLS client and
// returns the status, response headers (as Chrome-ready name/value pairs) and
// the (already decompressed) body.
func Forward(client tls_client.HttpClient, method, rawURL string, reqHeaders map[string]any, body string) (int, []map[string]string, []byte, error) {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := fhttp.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return 0, nil, nil, err
	}

	req.Header = fhttp.Header{}
	for k, v := range reqHeaders {
		lk := strings.ToLower(k)
		if _, drop := hopByHopRequestHeaders[lk]; drop {
			continue
		}
		req.Header.Set(k, fmt.Sprint(v))
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, err
	}

	headers := make([]map[string]string, 0, len(resp.Header))
	for name, values := range resp.Header {
		if _, drop := hopByHopResponseHeaders[strings.ToLower(name)]; drop {
			continue
		}
		for _, value := range values {
			headers = append(headers, map[string]string{"name": name, "value": value})
		}
	}

	return resp.StatusCode, headers, respBody, nil
}

// EncodeBody base64-encodes bytes for Fetch.fulfillRequest.
func EncodeBody(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
