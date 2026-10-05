// Package client is a thin client for the chain-analysis /api/v1 API, shared
// by the cactl CLI and the cactl-mcp server.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"chain-analysis-app/internal/app"
)

// DefaultBaseURL is used when CHAIN_ANALYSIS_URL is unset.
const DefaultBaseURL = "http://localhost:8090"

// Client talks to one chain-analysis server.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// PollInterval is how often RunJob polls a running job.
	PollInterval time.Duration
}

// New returns a client for baseURL, or for CHAIN_ANALYSIS_URL (falling back
// to DefaultBaseURL) when baseURL is empty.
func New(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = os.Getenv("CHAIN_ANALYSIS_URL")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		HTTP:         &http.Client{Timeout: 60 * time.Second},
		PollInterval: time.Second,
	}
}

// APIError is a non-2xx response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("chain-analysis API returned %d: %s", e.Status, e.Message)
}

func (c *Client) raw(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach chain-analysis server at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var failure struct {
			Error string `json:"error"`
		}
		message := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &failure) == nil && failure.Error != "" {
			message = failure.Error
		}
		return nil, &APIError{Status: resp.StatusCode, Message: message}
	}
	return data, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	data, err := c.raw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

// Job is a background job's state as the API reports it.
type Job struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Status  string          `json:"status"`
	Stage   string          `json:"stage"`
	Message string          `json:"message"`
	Error   string          `json:"error"`
	Result  json.RawMessage `json:"result"`
}

// RunJob starts a job with a POST to path, polls it until it ends, and
// decodes its result into out. onProgress, if set, sees every poll.
func (c *Client) RunJob(ctx context.Context, path string, payload, out any, onProgress func(Job)) error {
	var job Job
	if err := c.do(ctx, http.MethodPost, path, payload, &job); err != nil {
		return err
	}
	for job.Status == "running" {
		if onProgress != nil {
			onProgress(job)
		}
		select {
		case <-ctx.Done():
			_ = c.do(context.Background(), http.MethodDelete, "/api/v1/jobs/"+url.PathEscape(job.ID), nil, nil)
			return ctx.Err()
		case <-time.After(c.PollInterval):
		}
		if err := c.do(ctx, http.MethodGet, "/api/v1/jobs/"+url.PathEscape(job.ID), nil, &job); err != nil {
			return err
		}
	}
	if job.Status != "succeeded" {
		return fmt.Errorf("%s job %s %s: %s", job.Kind, job.ID, job.Status, job.Error)
	}
	if out != nil && len(job.Result) > 0 {
		if err := json.Unmarshal(job.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", job.Kind, err)
		}
	}
	return nil
}

func (c *Client) Health(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	return out, c.do(ctx, http.MethodGet, "/api/v1/health", nil, &out)
}

// Trace runs a trace and returns its result, which the server also saves.
func (c *Client) Trace(ctx context.Context, req app.TraceRequest, onProgress func(Job)) (app.TraceResponse, error) {
	var out app.TraceResponse
	return out, c.RunJob(ctx, "/api/v1/jobs/trace", req, &out, onProgress)
}

func (c *Client) TraceRun(ctx context.Context, id int64) (app.TraceRun, error) {
	var out app.TraceRun
	return out, c.do(ctx, http.MethodGet, "/api/v1/traces/"+strconv.FormatInt(id, 10), nil, &out)
}

func (c *Client) Actors(ctx context.Context) ([]app.Actor, error) {
	var out struct {
		Actors []app.Actor `json:"actors"`
	}
	return out.Actors, c.do(ctx, http.MethodGet, "/api/v1/actors", nil, &out)
}

// ResolveActor finds an actor by numeric ID or by name (case-insensitive).
func (c *Client) ResolveActor(ctx context.Context, ref string) (app.Actor, error) {
	actors, err := c.Actors(ctx)
	if err != nil {
		return app.Actor{}, err
	}
	ref = strings.TrimSpace(ref)
	id, idErr := strconv.ParseInt(ref, 10, 64)
	for _, actor := range actors {
		if (idErr == nil && actor.ID == id) || strings.EqualFold(actor.Name, ref) {
			return actor, nil
		}
	}
	return app.Actor{}, fmt.Errorf("no actor %q", ref)
}

func (c *Client) ActorMonitor(ctx context.Context, actorID int64) (app.ActorMonitor, error) {
	var out app.ActorMonitor
	return out, c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/actors/%d/monitor", actorID), nil, &out)
}

// ActorRefresh records a new monitoring snapshot for an actor.
func (c *Client) ActorRefresh(ctx context.Context, actorID int64, onProgress func(Job)) (app.ActorSnapshot, error) {
	var out app.ActorSnapshot
	return out, c.RunJob(ctx, "/api/v1/jobs/actor-refresh", map[string]any{"actor_id": actorID}, &out, onProgress)
}

func (c *Client) Labels(ctx context.Context, address string) ([]app.AddressLabel, error) {
	var out struct {
		Labels []app.AddressLabel `json:"labels"`
	}
	return out.Labels, c.do(ctx, http.MethodGet, "/api/v1/labels?address="+url.QueryEscape(address), nil, &out)
}

// LabelsImport imports a label source from a path on the server's machine.
func (c *Client) LabelsImport(ctx context.Context, source, path string, onProgress func(Job)) (app.LabelImportResult, error) {
	var out app.LabelImportResult
	return out, c.RunJob(ctx, "/api/v1/jobs/labels-import", map[string]string{"source": source, "path": path}, &out, onProgress)
}

func (c *Client) AddressProfile(ctx context.Context, req app.AddressProfileRequest, onProgress func(Job)) (app.AddressProfile, error) {
	var out app.AddressProfile
	return out, c.RunJob(ctx, "/api/v1/jobs/address-profile", req, &out, onProgress)
}

func (c *Client) LookupTx(ctx context.Context, txID string) (app.ActionLookupResult, error) {
	var out app.ActionLookupResult
	return out, c.do(ctx, http.MethodGet, "/api/v1/actions/"+url.PathEscape(strings.TrimSpace(txID)), nil, &out)
}

func (c *Client) Cases(ctx context.Context) ([]app.Case, error) {
	var out []app.Case
	return out, c.do(ctx, http.MethodGet, "/api/v1/cases", nil, &out)
}

func (c *Client) Case(ctx context.Context, id int64) (app.Case, error) {
	var out app.Case
	return out, c.do(ctx, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(id, 10), nil, &out)
}

func (c *Client) CreateCase(ctx context.Context, req app.CaseUpsertRequest) (app.Case, error) {
	var out app.Case
	return out, c.do(ctx, http.MethodPost, "/api/v1/cases", req, &out)
}

func (c *Client) AddCaseItem(ctx context.Context, caseID int64, req app.CaseItemRequest) (app.CaseItem, error) {
	var out app.CaseItem
	return out, c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/cases/%d/items", caseID), req, &out)
}

// CaseExport returns a case as Markdown (format "md") or as a flows CSV.
func (c *Client) CaseExport(ctx context.Context, caseID int64, format string) ([]byte, error) {
	return c.raw(ctx, http.MethodGet, fmt.Sprintf("/api/v1/cases/%d/export?format=%s", caseID, url.QueryEscape(format)), nil)
}

func (c *Client) LedgerCoverage(ctx context.Context, address string, start, end time.Time) (app.LedgerCoverageReport, error) {
	query := url.Values{"address": {address}}
	if !start.IsZero() {
		query.Set("start", start.UTC().Format(time.RFC3339))
	}
	if !end.IsZero() {
		query.Set("end", end.UTC().Format(time.RFC3339))
	}
	var out app.LedgerCoverageReport
	return out, c.do(ctx, http.MethodGet, "/api/v1/ledger/coverage?"+query.Encode(), nil, &out)
}
