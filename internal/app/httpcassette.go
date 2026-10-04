package app

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type cassetteMode string

const (
	cassetteReplay cassetteMode = "replay"
	cassetteRecord cassetteMode = "record"
)

const cassetteRedacted = "REDACTED"

// Query parameters whose values are credentials. Matching is case-insensitive.
var cassetteSecretParams = map[string]struct{}{
	"apikey":       {},
	"api_key":      {},
	"key":          {},
	"token":        {},
	"access_token": {},
	"x-api-key":    {},
}

// Response headers the app inspects (content sniffing, Cloudflare challenge
// detection, rate-limit health). Everything else is dropped from recordings.
var cassetteKeptHeaders = []string{
	"Content-Type",
	"Retry-After",
	"Cf-Mitigated",
	"X-Ratelimit-Limit",
	"X-Ratelimit-Remaining",
	"X-Ratelimit-Reset",
	"Ratelimit-Limit",
	"Ratelimit-Remaining",
	"Ratelimit-Reset",
}

type cassetteEntry struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	RequestBody string            `json:"request_body,omitempty"`
	Status      int               `json:"status,omitempty"`
	Header      map[string]string `json:"header,omitempty"`
	Body        string            `json:"body,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// httpCassette is an http.RoundTripper that records upstream exchanges and
// replays them by request identity, so golden tests run offline and
// deterministically. Request identity is the method, the URL with credentials
// redacted and query parameters sorted, and the redacted request body.
type httpCassette struct {
	path    string
	mode    cassetteMode
	next    http.RoundTripper
	secrets []string

	mu       sync.Mutex
	entries  map[string]cassetteEntry
	byPath   map[string][]string
	served   int
	aliased  int
	recorded int
	misses   []string
}

// newHTTPCassette opens the cassette at path. Replay mode loads existing
// entries and fails if the file is missing; record mode starts empty and
// forwards requests to next (http.DefaultTransport when nil).
func newHTTPCassette(path string, mode cassetteMode, next http.RoundTripper, secrets []string) (*httpCassette, error) {
	if next == nil {
		next = http.DefaultTransport
	}
	c := &httpCassette{
		path:    path,
		mode:    mode,
		next:    next,
		entries: map[string]cassetteEntry{},
		byPath:  map[string][]string{},
	}
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); len(secret) >= 6 {
			c.secrets = append(c.secrets, secret)
		}
	}
	if mode == cassetteReplay {
		if err := c.load(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *httpCassette) RoundTrip(req *http.Request) (*http.Response, error) {
	requestBody, err := readAndRestoreRequestBody(req)
	if err != nil {
		return nil, err
	}
	entry := cassetteEntry{
		Method:      req.Method,
		URL:         c.redact(redactCassetteURL(req.URL)),
		RequestBody: c.redact(requestBody),
	}
	key := cassetteKey(entry)

	if c.mode == cassetteReplay {
		c.mu.Lock()
		recorded, ok := c.entries[key]
		if ok {
			c.served++
		} else if alias, found := c.siblingHostEntry(entry); found {
			// Failover endpoints rotate per request, so replay may start at a
			// sibling host of the one recorded; equivalent endpoints share API
			// paths, so serve the sibling's recording.
			recorded, ok = alias, true
			c.aliased++
		} else {
			c.misses = append(c.misses, entry.Method+" "+entry.URL)
		}
		c.mu.Unlock()
		if !ok {
			return nil, fmt.Errorf("cassette miss: %s %s", entry.Method, entry.URL)
		}
		return recorded.response(req)
	}

	resp, err := c.next.RoundTrip(req)
	if err != nil {
		// Cancellations reflect this run's timing, not upstream behavior, so
		// they are not recorded.
		if req.Context().Err() == nil {
			entry.Error = c.redact(err.Error())
			c.store(key, entry)
		}
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	entry.Status = resp.StatusCode
	entry.Header = keptCassetteHeaders(resp.Header)
	// Upstreams sometimes echo credentials in error bodies; never persist them.
	entry.Body = c.redact(string(body))
	c.store(key, entry)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func (c *httpCassette) store(key string, entry cassetteEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		c.recorded++
	}
	c.entries[key] = entry
}

// Save writes recorded entries as gzip-compressed JSON sorted by key.
func (c *httpCassette) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.entries))
	for key := range c.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([]cassetteEntry, 0, len(keys))
	for _, key := range keys {
		ordered = append(ordered, c.entries[key])
	}
	raw, err := json.MarshalIndent(ordered, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(c.path, buf.Bytes(), 0o644)
}

func (c *httpCassette) load() error {
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("load cassette %s: %w", c.path, err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("load cassette %s: %w", c.path, err)
	}
	defer zr.Close()
	var entries []cassetteEntry
	if err := json.NewDecoder(zr).Decode(&entries); err != nil {
		return fmt.Errorf("load cassette %s: %w", c.path, err)
	}
	for _, entry := range entries {
		key := cassetteKey(entry)
		c.entries[key] = entry
		pathKey := cassettePathKey(entry)
		c.byPath[pathKey] = append(c.byPath[pathKey], key)
	}
	for pathKey := range c.byPath {
		sort.Strings(c.byPath[pathKey])
	}
	return nil
}

// siblingHostEntry returns a recording of the same request sent to another
// host. Callers hold c.mu.
func (c *httpCassette) siblingHostEntry(entry cassetteEntry) (cassetteEntry, bool) {
	keys := c.byPath[cassettePathKey(entry)]
	if len(keys) == 0 {
		return cassetteEntry{}, false
	}
	return c.entries[keys[0]], true
}

// Stats reports how many requests were served from or recorded to the
// cassette, and the requests replay could not serve. Served includes
// requests answered from a sibling host's recording.
func (c *httpCassette) Stats() (served, recorded int, misses []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.served + c.aliased, c.recorded, append([]string(nil), c.misses...)
}

func (c *httpCassette) redact(s string) string {
	for _, secret := range c.secrets {
		s = strings.ReplaceAll(s, secret, cassetteRedacted)
	}
	return s
}

func (e cassetteEntry) response(req *http.Request) (*http.Response, error) {
	if e.Error != "" {
		return nil, errors.New(e.Error)
	}
	header := http.Header{}
	for name, value := range e.Header {
		header.Set(name, value)
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", e.Status, http.StatusText(e.Status)),
		StatusCode:    e.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(e.Body)),
		ContentLength: int64(len(e.Body)),
		Request:       req,
	}, nil
}

func cassetteKey(e cassetteEntry) string {
	sum := sha256.Sum256([]byte(e.RequestBody))
	return e.Method + " " + e.URL + " " + hex.EncodeToString(sum[:8])
}

// cassettePathKey identifies a request independent of scheme and host.
func cassettePathKey(e cassetteEntry) string {
	requestURI := e.URL
	if parsed, err := url.Parse(e.URL); err == nil {
		requestURI = parsed.RequestURI()
	}
	sum := sha256.Sum256([]byte(e.RequestBody))
	return e.Method + " " + requestURI + " " + hex.EncodeToString(sum[:8])
}

func redactCassetteURL(u *url.URL) string {
	clone := *u
	query := clone.Query()
	for name := range query {
		if _, secret := cassetteSecretParams[strings.ToLower(name)]; secret {
			query.Set(name, cassetteRedacted)
		}
	}
	clone.RawQuery = query.Encode()
	return clone.String()
}

func keptCassetteHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for _, name := range cassetteKeptHeaders {
		if value := h.Get(name); value != "" {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readAndRestoreRequestBody(req *http.Request) (string, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return "", nil
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return string(body), nil
}
