package recaptcha

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// siteVerifyURL is Google's reCAPTCHA v3 verification endpoint.
const siteVerifyURL = "https://www.google.com/recaptcha/api/siteverify"

// VerifyResult is the decoded response from Google's siteverify endpoint.
type VerifyResult struct {
	Success bool `json:"success"`
	// Score is the v3 abuse score in [0.0, 1.0] (1.0 = very likely human).
	Score float64 `json:"score"`
	// Action is the action name the token was minted for.
	Action string `json:"action"`
	// ChallengeTS is the ISO-8601 timestamp of the challenge load.
	ChallengeTS string `json:"challenge_ts"`
	// Hostname is the site where the token was solved.
	Hostname string `json:"hostname"`
	// ErrorCodes holds any error-codes returned by Google.
	ErrorCodes []string `json:"error-codes"`
}

// Verify validates a token against Google's siteverify endpoint using your
// reCAPTCHA v3 secret key. This is a server-side call (no proxy / anti-bot), so
// it uses a plain HTTP client. remoteIP is optional — pass "" to omit it.
//
// Note: this endpoint is for standard reCAPTCHA v3. Enterprise keys are verified
// via the reCAPTCHA Enterprise createAssessment API instead.
func Verify(ctx context.Context, secret, token, remoteIP string) (*VerifyResult, error) {
	if secret == "" {
		return nil, fmt.Errorf("recaptcha: secret is required")
	}
	if token == "" {
		return nil, fmt.Errorf("recaptcha: token is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, siteVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recaptcha: siteverify status %d: %s", resp.StatusCode, string(body))
	}

	var out VerifyResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("recaptcha: decode siteverify response: %w (body: %s)", err, string(body))
	}
	return &out, nil
}

// Verify validates this result's token with your v3 secret key. See Verify.
func (r *Result) Verify(ctx context.Context, secret string) (*VerifyResult, error) {
	return Verify(ctx, secret, r.Token, "")
}
