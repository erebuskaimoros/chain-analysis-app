package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/domain/services"
)

func TestV1HealthRoute(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}

	var resp app.HealthSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok health snapshot, got %#v", resp)
	}
}

func TestV1ActorCRUD(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	body := bytes.NewBufferString(`{"name":"Desk","color":"#123456","addresses":[{"address":"thor1abc","label":"primary"}]}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/actors", body)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("unexpected create status %d: %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/actors", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("unexpected list status %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp struct {
		Actors []app.Actor `json:"actors"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Actors) != 1 {
		t.Fatalf("expected 1 actor, got %#v", listResp.Actors)
	}
	if listResp.Actors[0].Name != "Desk" {
		t.Fatalf("unexpected actor name %#v", listResp.Actors[0])
	}
}

func newTestHandler(t *testing.T) (http.Handler, func()) {
	t.Helper()

	legacy, err := app.New(app.Config{
		DBPath:         filepath.Join(t.TempDir(), "test.db"),
		StaticDir:      filepath.Join("..", "web", "static"),
		UIBuildDir:     filepath.Join("..", "web", "ui", "dist"),
		RequestTimeout: time.Second,
		MidgardTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	svcs := services.New(legacy)
	v1 := NewV1(svcs)
	mux := http.NewServeMux()
	v1.Register(mux)

	return mux, func() {
		_ = legacy.Close()
	}
}

func TestV1ActorDeleteMissingReturnsNotFound(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/actors/99", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestV1HealthContextToleratesRequestContext(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rec.Code)
	}
}
