package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV1CasesLifecycleAndExport(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := call(http.MethodPost, "/api/v1/cases", `{"title":" "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a blank title, got %d: %s", rec.Code, rec.Body.String())
	}
	rec := call(http.MethodPost, "/api/v1/cases", `{"title":"Bitget hack","notes_md":"Reported."}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created struct{ ID int64 }
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	base := fmt.Sprintf("/api/v1/cases/%d", created.ID)

	if rec := call(http.MethodPost, base+"/items", `{"kind":"tx","ref":"0xa732e09aab76a571d768c2b5b4e2f0e1e5b1a9c3d4e5f60718293a4b5c6d7e8f"}`); rec.Code != http.StatusCreated {
		t.Fatalf("add item: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPost, base+"/items", `{"kind":"trace_run","ref":"7"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing trace run, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(http.MethodPut, base, `{"title":"Bitget hack 2026-09-28","notes_md":"Updated."}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "2026-09-28") {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	rec = call(http.MethodGet, base+"/export?format=md", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/markdown") ||
		!strings.Contains(rec.Body.String(), "https://thorchain.net/tx/A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F") {
		t.Fatalf("markdown export: %d %s %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	rec = call(http.MethodGet, base+"/export?format=csv", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("csv export: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := call(http.MethodGet, base+"/export?format=pdf", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown format, got %d", rec.Code)
	}

	rec = call(http.MethodGet, base, "")
	var got struct {
		Items []struct{ ID int64 }
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Items) != 1 {
		t.Fatalf("expected one item, got %s", rec.Body.String())
	}
	if rec := call(http.MethodDelete, fmt.Sprintf("%s/items/%d", base, got.Items[0].ID), ""); rec.Code != http.StatusOK {
		t.Fatalf("delete item: %d", rec.Code)
	}
	if rec := call(http.MethodDelete, base, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete case: %d", rec.Code)
	}
	if rec := call(http.MethodGet, base, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rec.Code)
	}
}

func TestV1LedgerCoverageAndJobValidation(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/coverage?address=thor1abc&start=2026-09-01T00:00:00Z", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sources":[]`) {
		t.Fatalf("coverage: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/coverage?address=thor1abc&start=yesterday", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a bad start, got %d", rec.Code)
	}
	for path, body := range map[string]string{"/api/v1/jobs/address-profile": `{}`, "/api/v1/jobs/labels-import": `{"source":"ofac"}`} {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}
