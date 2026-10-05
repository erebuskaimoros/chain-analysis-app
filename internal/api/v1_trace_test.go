package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV1TraceRejectsInvalidRequests(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	cases := map[string]string{
		"no seeds":           `{"start_time":"2026-09-28T03:00:00Z"}`,
		"unknown policy":     `{"seeds":[{"address":"thor1abc"}],"policy":"lifo"}`,
		"unknown direction":  `{"seeds":[{"address":"thor1abc"}],"direction":"sideways"}`,
		"amount, no asset":   `{"seeds":[{"address":"thor1abc"}],"amount":5}`,
		"two seeds + amount": `{"seeds":[{"address":"a"},{"address":"b"}],"amount":5,"asset":"BTC.BTC"}`,
	}
	for name, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/trace", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestV1TraceRunsListAndMissingRun(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil))
	var runs []map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &runs) != nil || len(runs) != 0 {
		t.Fatalf("expected an empty run list, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/traces/42", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s missing run: expected 404, got %d: %s", method, rec.Code, rec.Body.String())
		}
	}
}
