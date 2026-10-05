package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// providerErrorKind classifies an upstream failure by what a caller should do
// about it.
type providerErrorKind string

const (
	// providerErrRateLimited: back off until RetryAfter, then retry.
	providerErrRateLimited providerErrorKind = "rate_limited"
	// providerErrBanned: the upstream refuses this client (403, Cloudflare
	// challenge); retrying soon only extends the ban.
	providerErrBanned providerErrorKind = "banned"
	// providerErrTransient: 5xx, timeouts, resets; retry later.
	providerErrTransient providerErrorKind = "transient"
	// providerErrPermanent: the request itself is wrong (4xx, bad payload).
	providerErrPermanent providerErrorKind = "permanent"
	// providerErrConfig: missing key, bad host, or other local setup problem.
	providerErrConfig providerErrorKind = "config"
	// providerErrCircuitOpen: not attempted because the provider is backing off.
	providerErrCircuitOpen providerErrorKind = "circuit_open"
)

// providerError wraps an upstream failure with its kind. Error() returns the
// wrapped text unchanged so existing message-based checks keep working.
type providerError struct {
	Kind       providerErrorKind
	Status     int
	RetryAfter time.Duration
	Err        error
}

func (e *providerError) Error() string { return e.Err.Error() }
func (e *providerError) Unwrap() error { return e.Err }

func newHTTPStatusProviderError(err error, status int, header http.Header, body string) *providerError {
	kind := classifyHTTPStatus(status, body)
	return &providerError{Kind: kind, Status: status, RetryAfter: parseRetryAfter(header), Err: err}
}

func newTransportProviderError(err error) *providerError {
	return &providerError{Kind: classifyTransportError(err), Err: err}
}

func classifyHTTPStatus(status int, body string) providerErrorKind {
	text := strings.ToLower(body)
	switch {
	case status == http.StatusTooManyRequests:
		return providerErrRateLimited
	case status == http.StatusForbidden, strings.Contains(text, "the gates of asgard are closed"), strings.Contains(text, "cloudflare challenge"):
		return providerErrBanned
	case status == http.StatusUnauthorized:
		return providerErrConfig
	case status == http.StatusRequestTimeout, status >= 500:
		return providerErrTransient
	default:
		return providerErrPermanent
	}
}

func classifyTransportError(err error) providerErrorKind {
	if err == nil {
		return providerErrTransient
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return providerErrConfig
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "no such host"), strings.Contains(text, "unsupported protocol scheme"),
		strings.Contains(text, "certificate"), strings.Contains(text, "x509"):
		return providerErrConfig
	default:
		return providerErrTransient
	}
}

// parseRetryAfter reads Retry-After as seconds or an HTTP date.
func parseRetryAfter(header http.Header) time.Duration {
	if header == nil {
		return 0
	}
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return d
		}
	}
	return 0
}

// classifyProviderError returns the kind of err and any retry delay. Errors
// not produced by the HTTP helpers are classified by context and message.
func classifyProviderError(err error) (providerErrorKind, time.Duration) {
	if err == nil {
		return "", 0
	}
	var perr *providerError
	if errors.As(err, &perr) {
		return perr.Kind, perr.RetryAfter
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || isTimeoutError(err) {
		return providerErrTransient, 0
	}
	if errors.Is(err, errExternalTrackerUnavailable) {
		return providerErrConfig, 0
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "status=429"):
		return providerErrRateLimited, 0
	case strings.Contains(text, "status=403"), strings.Contains(text, "the gates of asgard are closed"), strings.Contains(text, "cloudflare challenge"):
		return providerErrBanned, 0
	case strings.Contains(text, "status=5"), strings.Contains(text, "connection reset"), strings.Contains(text, "connection refused"),
		strings.Contains(text, "unexpected eof"), strings.Contains(text, "tls handshake timeout"):
		return providerErrTransient, 0
	case strings.Contains(text, "no such host"):
		return providerErrConfig, 0
	}
	return providerErrPermanent, 0
}

// isRetryableProviderKind reports whether a later attempt can succeed without
// a configuration change.
func isRetryableProviderKind(kind providerErrorKind) bool {
	switch kind {
	case providerErrRateLimited, providerErrTransient, providerErrCircuitOpen:
		return true
	default:
		return false
	}
}
