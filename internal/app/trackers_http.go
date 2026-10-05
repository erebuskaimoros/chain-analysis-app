package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func mapTrackerURLs(baseURLs []string, build func(string) string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(baseURLs))
	for _, baseURL := range baseURLs {
		candidate := strings.TrimSpace(build(strings.TrimSpace(baseURL)))
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func (a *App) rotateTrackerURLs(rawURLs []string) []string {
	filtered := mapTrackerURLs(rawURLs, func(rawURL string) string { return rawURL })
	if len(filtered) < 2 {
		return filtered
	}
	idx := int(a.trackerEndpointRR.Add(1)-1) % len(filtered)
	rotated := make([]string, len(filtered))
	for i := range filtered {
		rotated[i] = filtered[(idx+i)%len(filtered)]
	}
	return rotated
}

func (a *App) lookupTrackerBlockNumber(provider, chain, closest string, timestamp int64) (int64, bool) {
	if a == nil || a.trackerBlockNums == nil {
		return 0, false
	}
	return a.trackerBlockNums.get(provider, chain, closest, timestamp)
}

func (a *App) storeTrackerBlockNumber(provider, chain, closest string, timestamp, block int64) {
	if a == nil || a.trackerBlockNums == nil {
		return
	}
	a.trackerBlockNums.set(provider, chain, closest, timestamp, block)
}

func (a *App) getJSONAbsolute(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	return a.getJSONAbsoluteMulti(ctx, []string{rawURL}, headers, out)
}

func (a *App) getJSONAbsoluteMulti(ctx context.Context, rawURLs []string, headers map[string]string, out any) error {
	var lastErr error
	candidates := a.rotateTrackerURLs(rawURLs)
	for idx, rawURL := range candidates {
		if err := a.getJSONAbsoluteSingle(ctx, rawURL, headers, out); err != nil {
			lastErr = err
			if idx < len(candidates)-1 && !sleepWithContext(ctx, backoffForAttempt(idx+1)) {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errExternalTrackerUnavailable
}

func (a *App) getJSONAbsoluteSingle(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	if err := a.trackerCircuitError(ctx, rawURL); err != nil {
		return err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok && a.trackerThrottle != nil {
		release, err := a.trackerThrottle.acquire(ctx, meta.Provider, meta.Chain)
		if err != nil {
			a.recordTrackerTransportFailure(ctx, meta, err)
			return err
		}
		defer release()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		err = newTransportProviderError(err)
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.recordTrackerTransportFailure(ctx, meta, err)
		}
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := newHTTPStatusProviderError(fmt.Errorf("GET %s failed: status=%d body=%s", rawURL, resp.StatusCode, trimForLog(string(body), 200)), resp.StatusCode, resp.Header, string(body))
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, httpErr)
		}
		return httpErr
	}
	if out == nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
		}
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok {
		a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
	}
	return nil
}

func (a *App) getTextAbsoluteMulti(ctx context.Context, rawURLs []string, headers map[string]string) (string, error) {
	var lastErr error
	candidates := a.rotateTrackerURLs(rawURLs)
	for idx, rawURL := range candidates {
		body, err := a.getTextAbsoluteSingle(ctx, rawURL, headers)
		if err != nil {
			lastErr = err
			if idx < len(candidates)-1 && !sleepWithContext(ctx, backoffForAttempt(idx+1)) {
				return "", ctx.Err()
			}
			continue
		}
		return body, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errExternalTrackerUnavailable
}

func (a *App) getTextAbsoluteSingle(ctx context.Context, rawURL string, headers map[string]string) (string, error) {
	if err := a.trackerCircuitError(ctx, rawURL); err != nil {
		return "", err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok && a.trackerThrottle != nil {
		release, err := a.trackerThrottle.acquire(ctx, meta.Provider, meta.Chain)
		if err != nil {
			a.recordTrackerTransportFailure(ctx, meta, err)
			return "", err
		}
		defer release()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html, text/plain;q=0.9, */*;q=0.8")
	req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		err = newTransportProviderError(err)
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.recordTrackerTransportFailure(ctx, meta, err)
		}
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := newHTTPStatusProviderError(fmt.Errorf("GET %s failed: status=%d body=%s", rawURL, resp.StatusCode, trimForLog(string(body), 200)), resp.StatusCode, resp.Header, string(body))
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, httpErr)
		}
		return "", httpErr
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok {
		a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
	}
	return string(body), nil
}

func (a *App) postJSONAbsolute(ctx context.Context, rawURL string, headers map[string]string, payload any, out any) error {
	return a.postJSONAbsoluteMulti(ctx, []string{rawURL}, headers, payload, out)
}

func (a *App) postJSONAbsoluteMulti(ctx context.Context, rawURLs []string, headers map[string]string, payload any, out any) error {
	var lastErr error
	candidates := a.rotateTrackerURLs(rawURLs)
	for idx, rawURL := range candidates {
		if err := a.postJSONAbsoluteSingle(ctx, rawURL, headers, payload, out); err != nil {
			lastErr = err
			if idx < len(candidates)-1 && !sleepWithContext(ctx, backoffForAttempt(idx+1)) {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errExternalTrackerUnavailable
}

func (a *App) postJSONAbsoluteSingle(ctx context.Context, rawURL string, headers map[string]string, payload any, out any) error {
	if err := a.trackerCircuitError(ctx, rawURL); err != nil {
		return err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok && a.trackerThrottle != nil {
		release, err := a.trackerThrottle.acquire(ctx, meta.Provider, meta.Chain)
		if err != nil {
			a.recordTrackerTransportFailure(ctx, meta, err)
			return err
		}
		defer release()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		err = newTransportProviderError(err)
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.recordTrackerTransportFailure(ctx, meta, err)
		}
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := newHTTPStatusProviderError(fmt.Errorf("POST %s failed: status=%d body=%s", rawURL, resp.StatusCode, trimForLog(string(body), 200)), resp.StatusCode, resp.Header, string(body))
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, httpErr)
		}
		return httpErr
	}
	if out == nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
		}
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok {
		a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
	}
	return nil
}

func (a *App) postRPCJSON(ctx context.Context, rawURLs []string, method string, params []any, out any) error {
	var env solanaRPCEnvelope
	if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}, &env); err != nil {
		return err
	}
	if env.Error != nil {
		return fmt.Errorf("rpc %s failed: %v", method, env.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}

// trackerCircuitError fails fast while the request's provider is backing off,
// without contacting the upstream.
func (a *App) trackerCircuitError(ctx context.Context, rawURL string) error {
	meta, ok := trackerRequestMetaFromContext(ctx)
	if !ok || a.trackerHealth == nil {
		return nil
	}
	until, reason := a.trackerHealth.circuitOpenUntil(meta.Provider, meta.Chain)
	if until.IsZero() {
		return nil
	}
	return &providerError{
		Kind:       providerErrCircuitOpen,
		RetryAfter: time.Until(until),
		Err:        fmt.Errorf("%s skipped: %s/%s backing off (%s) until %s", rawURL, meta.Provider, meta.Chain, reason, until.Format(time.RFC3339)),
	}
}

// recordTrackerTransportFailure records a request that got no response. When
// our own context ended (a lookup budget expired while queued or in flight),
// the provider did nothing wrong, so it does not count toward its circuit.
func (a *App) recordTrackerTransportFailure(ctx context.Context, meta trackerRequestMeta, err error) {
	if ctx.Err() != nil {
		return
	}
	a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, 0, nil, err)
}
