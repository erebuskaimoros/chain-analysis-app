package app

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPCassetteRecordsAndReplaysWithRedaction(t *testing.T) {
	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "7")
		w.Header().Set("X-Unrelated", "dropped")
		_, _ = w.Write([]byte(`{"path":"` + r.URL.Path + `","body":"` + string(body) + `"}`))
	}))
	defer upstream.Close()

	path := filepath.Join(t.TempDir(), "cassette.json.gz")
	const secret = "s3cret-api-key"
	recorder, err := newHTTPCassette(path, cassetteRecord, nil, []string{secret})
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	client := &http.Client{Transport: recorder}

	get := func(c *http.Client, rawURL string) string {
		t.Helper()
		resp, err := c.Get(rawURL)
		if err != nil {
			t.Fatalf("GET %s: %v", rawURL, err)
		}
		defer resp.Body.Close()
		if got := resp.Header.Get("Retry-After"); got != "7" {
			t.Fatalf("expected Retry-After to survive, got %q", got)
		}
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	post := func(c *http.Client, rawURL, payload string) string {
		t.Helper()
		resp, err := c.Post(rawURL, "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("POST %s: %v", rawURL, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}

	queryKeyURL := upstream.URL + "/api?module=account&apikey=" + secret
	pathKeyURL := upstream.URL + "/v1/" + secret
	recordedQuery := get(client, queryKeyURL)
	recordedPath := get(client, pathKeyURL)
	recordedPost := post(client, upstream.URL+"/rpc", `{"method":"getBalance"}`)
	if upstreamHits != 3 {
		t.Fatalf("expected 3 upstream hits while recording, got %d", upstreamHits)
	}
	if err := recorder.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette: %v", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	plain, _ := io.ReadAll(zr)
	if bytes.Contains(plain, []byte(secret)) {
		t.Fatalf("cassette leaked the secret: %s", plain)
	}
	if bytes.Contains(plain, []byte("X-Unrelated")) {
		t.Fatalf("cassette kept a header outside the allowlist: %s", plain)
	}

	upstream.Close()
	// Replay with a different credential value: redaction makes request
	// identity independent of the key, so a placeholder key still matches.
	const placeholder = "replay-placeholder-key"
	player, err := newHTTPCassette(path, cassetteReplay, nil, []string{placeholder})
	if err != nil {
		t.Fatalf("new player: %v", err)
	}
	replayClient := &http.Client{Transport: player}
	if got := get(replayClient, strings.ReplaceAll(queryKeyURL, secret, placeholder)); got != recordedQuery {
		t.Fatalf("query-key replay mismatch: %q vs %q", got, recordedQuery)
	}
	// The upstream echoed the path-embedded key; the recording keeps the
	// response but with the credential redacted.
	wantPath := strings.ReplaceAll(recordedPath, secret, cassetteRedacted)
	if got := get(replayClient, strings.ReplaceAll(pathKeyURL, secret, placeholder)); got != wantPath {
		t.Fatalf("path-key replay mismatch: %q vs %q", got, wantPath)
	}
	if got := post(replayClient, upstream.URL+"/rpc", `{"method":"getBalance"}`); got != recordedPost {
		t.Fatalf("POST replay mismatch: %q vs %q", got, recordedPost)
	}
	if _, err := replayClient.Post(upstream.URL+"/rpc", "application/json", strings.NewReader(`{"method":"other"}`)); err == nil {
		t.Fatal("expected a miss for an unrecorded request body")
	}
	served, _, misses := player.Stats()
	if served != 3 || len(misses) != 1 || !strings.Contains(misses[0], "/rpc") {
		t.Fatalf("unexpected replay stats served=%d misses=%v", served, misses)
	}
}

func TestHTTPCassetteReplaysRecordedTransportErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cassette.json.gz")
	failing := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})
	recorder, err := newHTTPCassette(path, cassetteRecord, failing, nil)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	if _, err := (&http.Client{Transport: recorder}).Get("http://upstream.invalid/x"); err == nil {
		t.Fatal("expected recorded transport error")
	}
	if err := recorder.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	player, err := newHTTPCassette(path, cassetteReplay, nil, nil)
	if err != nil {
		t.Fatalf("new player: %v", err)
	}
	_, err = (&http.Client{Transport: player}).Get("http://upstream.invalid/x")
	if err == nil || !strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("expected replayed transport error, got %v", err)
	}
	if _, _, misses := player.Stats(); len(misses) != 0 {
		t.Fatalf("recorded error should not be a miss: %v", misses)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPCassetteReplaysFailoverSiblingHost(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"from":"primary","path":"` + r.URL.Path + `"}`))
	}))
	defer primary.Close()

	path := filepath.Join(t.TempDir(), "cassette.json.gz")
	recorder, err := newHTTPCassette(path, cassetteRecord, nil, nil)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	resp, err := (&http.Client{Transport: recorder}).Get(primary.URL + "/api/address/bc1qexample")
	if err != nil {
		t.Fatalf("record GET: %v", err)
	}
	resp.Body.Close()
	if err := recorder.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	player, err := newHTTPCassette(path, cassetteReplay, nil, nil)
	if err != nil {
		t.Fatalf("new player: %v", err)
	}
	resp, err = (&http.Client{Transport: player}).Get("https://sibling.example/api/address/bc1qexample")
	if err != nil {
		t.Fatalf("expected sibling-host replay, got %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `"from":"primary"`) {
		t.Fatalf("unexpected sibling replay body %s", body)
	}
	if _, err := (&http.Client{Transport: player}).Get("https://sibling.example/api/address/other"); err == nil {
		t.Fatal("expected a miss for a different path")
	}
	served, _, misses := player.Stats()
	if served != 1 || len(misses) != 1 {
		t.Fatalf("unexpected stats served=%d misses=%v", served, misses)
	}
}
