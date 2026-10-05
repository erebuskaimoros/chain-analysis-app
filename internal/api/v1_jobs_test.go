package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestV1JobsLifecycle(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	empty := httptest.NewRecorder()
	handler.ServeHTTP(empty, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/live-holdings", strings.NewReader(`{"nodes":[]}`)))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an empty node list, got %d %s", empty.Code, empty.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/nope", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown job, got %d", missing.Code)
	}

	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/api/v1/jobs/live-holdings",
		strings.NewReader(`{"nodes":[{"id":"pool:X","kind":"pool","chain":"THOR","metrics":{"pool":"BTC.BTC"}}]}`)))
	if start.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d %s", start.Code, start.Body.String())
	}
	var job struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &job); err != nil || job.ID == "" || job.Status != "running" {
		t.Fatalf("expected a running job, got %v %s", err, start.Body.String())
	}

	cancel := httptest.NewRecorder()
	handler.ServeHTTP(cancel, httptest.NewRequest(http.MethodDelete, "/api/v1/jobs/"+job.ID, nil))
	if cancel.Code != http.StatusOK {
		t.Fatalf("expected cancel to succeed, got %d", cancel.Code)
	}
	deadline := time.Now().Add(10 * time.Second)
	for job.Status == "running" {
		if time.Now().After(deadline) {
			t.Fatal("canceled job did not finish")
		}
		time.Sleep(20 * time.Millisecond)
		poll := httptest.NewRecorder()
		handler.ServeHTTP(poll, httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+job.ID, nil))
		if err := json.Unmarshal(poll.Body.Bytes(), &job); err != nil {
			t.Fatalf("decode poll: %v", err)
		}
	}
	if job.Status != "canceled" && job.Status != "failed" && job.Status != "succeeded" {
		t.Fatalf("unexpected final status %q", job.Status)
	}
}
