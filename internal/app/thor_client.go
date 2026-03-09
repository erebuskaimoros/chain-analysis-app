package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type ThorClient struct {
	endpoints []string
	client    *http.Client
	rrCounter atomic.Uint64
}

type RequestAttemptMeta struct {
	Endpoint            string
	URL                 string
	Path                string
	Attempt             int
	StatusCode          int
	Duration            time.Duration
	Result              string
	Error               string
	WillRetry           bool
	RetryableStatus     bool
	RetryAfter          string
	XRateLimitLimit     string
	XRateLimitRemaining string
	XRateLimitReset     string
	RateLimitLimit      string
	RateLimitRemaining  string
	RateLimitReset      string
	CFRay               string
	CFMitigated         string
}

func NewThorClient(endpoints []string, timeout time.Duration) *ThorClient {
	return &ThorClient{
		endpoints: endpoints,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *ThorClient) GetJSON(ctx context.Context, path string, out any) error {
	return c.getJSON(ctx, path, out, nil)
}

func (c *ThorClient) GetJSONObserved(ctx context.Context, path string, out any, observer func(RequestAttemptMeta)) error {
	return c.getJSON(ctx, path, out, observer)
}

func (c *ThorClient) getJSON(ctx context.Context, path string, out any, observer func(RequestAttemptMeta)) error {
	cleanPath := path
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	var lastErr error
	for _, endpoint := range c.endpointsForPath(cleanPath) {
		raw := endpoint + cleanPath
		_, err := url.Parse(raw)
		if err != nil {
			lastErr = fmt.Errorf("invalid URL %q: %w", raw, err)
			if observer != nil {
				observer(RequestAttemptMeta{
					Endpoint: endpoint,
					URL:      raw,
					Path:     cleanPath,
					Result:   "url_error",
					Error:    lastErr.Error(),
				})
			}
			continue
		}

		attempts := requestAttemptsForPath(cleanPath)
		for attempt := 1; attempt <= attempts; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
			if err != nil {
				lastErr = err
				break
			}
			req.Header.Set("Accept", "application/json")
			req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")

			started := time.Now()
			resp, err := c.client.Do(req)
			if err != nil {
				lastErr = err
				isTimeout := isTimeoutError(err)
				willRetry := attempt < attempts && !isTimeout
				if observer != nil {
					observer(RequestAttemptMeta{
						Endpoint:  endpoint,
						URL:       raw,
						Path:      cleanPath,
						Attempt:   attempt,
						Duration:  time.Since(started),
						Result:    "transport_error",
						Error:     err.Error(),
						WillRetry: willRetry,
					})
				}
				if willRetry && sleepWithContext(ctx, backoffForAttempt(attempt)) {
					continue
				}
				break
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				lastErr = err
				willRetry := attempt < attempts
				if observer != nil {
					observer(requestMetaFromResponse(resp, endpoint, raw, cleanPath, attempt, time.Since(started), "read_error", err.Error(), willRetry, false))
				}
				if willRetry && sleepWithContext(ctx, backoffForAttempt(attempt)) {
					continue
				}
				break
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				retryable := shouldRetryStatus(resp.StatusCode)
				willRetry := attempt < attempts && retryable
				if isCloudflareChallenge(resp, body) {
					lastErr = fmt.Errorf("GET %s failed: status=%d cloudflare challenge", raw, resp.StatusCode)
					if observer != nil {
						observer(requestMetaFromResponse(resp, endpoint, raw, cleanPath, attempt, time.Since(started), "http_error", lastErr.Error(), false, retryable))
					}
					break
				}
				lastErr = fmt.Errorf("GET %s failed: status=%d body=%s", raw, resp.StatusCode, trimForLog(string(body), 200))
				if observer != nil {
					observer(requestMetaFromResponse(resp, endpoint, raw, cleanPath, attempt, time.Since(started), "http_error", lastErr.Error(), willRetry, retryable))
				}
				if willRetry && sleepWithContext(ctx, backoffForAttempt(attempt)) {
					continue
				}
				break
			}

			if err := json.Unmarshal(body, out); err != nil {
				lastErr = fmt.Errorf("decode %s: %w", raw, err)
				if observer != nil {
					observer(requestMetaFromResponse(resp, endpoint, raw, cleanPath, attempt, time.Since(started), "decode_error", lastErr.Error(), false, false))
				}
				break
			}
			if observer != nil {
				observer(requestMetaFromResponse(resp, endpoint, raw, cleanPath, attempt, time.Since(started), "success", "", false, false))
			}
			return nil
		}
	}

	return fmt.Errorf("all thornode endpoints failed: %w", lastErr)
}

func requestMetaFromResponse(resp *http.Response, endpoint, raw, path string, attempt int, duration time.Duration, result, errText string, willRetry bool, retryable bool) RequestAttemptMeta {
	meta := RequestAttemptMeta{
		Endpoint:            endpoint,
		URL:                 raw,
		Path:                path,
		Attempt:             attempt,
		StatusCode:          resp.StatusCode,
		Duration:            duration,
		Result:              result,
		Error:               errText,
		WillRetry:           willRetry,
		RetryableStatus:     retryable,
		RetryAfter:          strings.TrimSpace(resp.Header.Get("Retry-After")),
		XRateLimitLimit:     strings.TrimSpace(resp.Header.Get("X-RateLimit-Limit")),
		XRateLimitRemaining: strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")),
		XRateLimitReset:     strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")),
		RateLimitLimit:      strings.TrimSpace(resp.Header.Get("RateLimit-Limit")),
		RateLimitRemaining:  strings.TrimSpace(resp.Header.Get("RateLimit-Remaining")),
		RateLimitReset:      strings.TrimSpace(resp.Header.Get("RateLimit-Reset")),
		CFRay:               strings.TrimSpace(resp.Header.Get("cf-ray")),
		CFMitigated:         strings.TrimSpace(resp.Header.Get("cf-mitigated")),
	}
	return meta
}

func (c *ThorClient) endpointsForPath(path string) []string {
	if len(c.endpoints) < 2 {
		return c.endpoints
	}
	return c.rotatedEndpoints()
}

func (c *ThorClient) rotatedEndpoints() []string {
	n := len(c.endpoints)
	if n < 2 {
		return c.endpoints
	}
	idx := int(c.rrCounter.Add(1)-1) % n
	rotated := make([]string, n)
	for i := 0; i < n; i++ {
		rotated[i] = c.endpoints[(idx+i)%n]
	}
	return rotated
}

func requestAttemptsForPath(_ string) int {
	return 2
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "Client.Timeout exceeded")
}

func shouldRetryStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func backoffForAttempt(attempt int) time.Duration {
	ms := 250 * attempt
	if ms > 2000 {
		ms = 2000
	}
	return time.Duration(ms) * time.Millisecond
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isCloudflareChallenge(resp *http.Response, body []byte) bool {
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("cf-mitigated")), "challenge") {
		return true
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "text/html") {
		return false
	}
	text := strings.ToLower(trimForLog(string(body), 600))
	return strings.Contains(text, "just a moment") || strings.Contains(text, "cloudflare") || strings.Contains(text, "__cf_chl")
}

func trimForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
