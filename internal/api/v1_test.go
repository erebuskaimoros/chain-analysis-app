package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("expected request id header on v1 response")
	}

	var resp app.HealthSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok health snapshot, got %#v", resp)
	}
	if _, ok := resp.LiquidityEngines["THOR"]; !ok {
		t.Fatalf("expected THOR engine in health snapshot, got %#v", resp.LiquidityEngines)
	}
	if _, ok := resp.LiquidityEngines["MAYA"]; !ok {
		t.Fatalf("expected MAYA engine in health snapshot, got %#v", resp.LiquidityEngines)
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

func TestV1ActorGraphReturnsGraphWhenRunSaveFails(t *testing.T) {
	v1 := &V1{
		buildActorGraphFn: func(ctx context.Context, req app.ActorTrackerRequest) (app.ActorTrackerResponse, error) {
			return app.ActorTrackerResponse{
				Actors: []app.Actor{{ID: 2, Name: "Desk"}},
				Nodes: []app.FlowNode{{
					ID:    "actor:2",
					Kind:  "actor",
					Label: "Desk",
				}},
				Edges: []app.FlowEdge{},
			}, nil
		},
		createActorGraphRunFn: func(ctx context.Context, req app.ActorTrackerRequest, summary string, nodeCount, edgeCount int) (int64, error) {
			return 0, errors.New("insert failed")
		},
	}

	mux := http.NewServeMux()
	v1.Register(mux)

	body := bytes.NewBufferString(`{"actor_ids":[2],"start_time":"2026-03-10T01:28","end_time":"2026-03-11T01:28","max_hops":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/actor-graph", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected actor graph to still return 200 when run save fails, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Actors []app.Actor    `json:"actors"`
		Nodes  []app.FlowNode `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Actors) != 1 || len(resp.Nodes) != 1 {
		t.Fatalf("expected graph payload, got %#v", resp)
	}
}

func TestV1LiveHoldingsUsesSlimNodePayload(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/thorchain/inbound_addresses":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/pools":
			_ = json.NewEncoder(w).Encode([]app.MidgardPool{
				{
					Asset:         "ETH.USDC",
					Status:        "available",
					AssetDepth:    "400000000",
					RuneDepth:     "200000000",
					AssetPriceUSD: "1",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	legacy, err := app.New(app.Config{
		DBPath:            filepath.Join(t.TempDir(), "test.db"),
		StaticDir:         filepath.Join("..", "web", "static"),
		UIBuildDir:        filepath.Join("..", "web", "ui", "dist"),
		RequestTimeout:    time.Second,
		MidgardTimeout:    time.Second,
		ThornodeEndpoints: []string{upstream.URL},
		MidgardEndpoints:  []string{upstream.URL},
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer legacy.Close()

	v1 := NewV1(services.New(legacy))
	mux := http.NewServeMux()
	v1.Register(mux)

	body := bytes.NewBufferString(`{
		"nodes": [
			{
				"id": "pool:eth-usdc",
				"kind": "pool",
				"chain": "THOR",
				"metrics": {
					"pool": "ETH.USDC",
					"source_protocol": "THOR",
					"ignored": "drop-me"
				}
			}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/actor-graph/live-holdings", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected one live-holdings node update, got %#v", resp.Nodes)
	}
	if _, exists := resp.Nodes[0]["kind"]; exists {
		t.Fatalf("expected live-holdings response to omit node topology fields, got %#v", resp.Nodes[0])
	}
	metrics, ok := resp.Nodes[0]["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("expected metrics object in response, got %#v", resp.Nodes[0]["metrics"])
	}
	if metrics["live_holdings_status"] != "available" {
		t.Fatalf("expected live holdings metrics in response, got %#v", metrics)
	}
}
