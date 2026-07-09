package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"chain-analysis-app/internal/app"
)

func TestV1GraphStateCRUD(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	createBody := bytes.NewBufferString(`{
		"kind": "actor-graph",
		"name": "Treasury investigation",
		"state": {"ui_state": {"selected_actor_ids": [1, 2]}, "graph": {"nodes": []}},
		"node_count": 12,
		"edge_count": 18
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/graph-states", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("unexpected create status %d: %s", createRec.Code, createRec.Body.String())
	}
	var created app.GraphStateSummary
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 || created.Name != "Treasury investigation" || created.Kind != "actor-graph" {
		t.Fatalf("unexpected create summary %#v", created)
	}
	if created.NodeCount != 12 || created.EdgeCount != 18 {
		t.Fatalf("unexpected counts in summary %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/graph-states?kind=actor-graph", nil)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("unexpected list status %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		States []app.GraphStateSummary `json:"states"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.States) != 1 || listResp.States[0].ID != created.ID {
		t.Fatalf("unexpected list response %#v", listResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/graph-states/%d", created.ID), nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("unexpected get status %d: %s", getRec.Code, getRec.Body.String())
	}
	var fetched app.GraphState
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	var payload struct {
		UIState struct {
			SelectedActorIDs []int `json:"selected_actor_ids"`
		} `json:"ui_state"`
	}
	if err := json.Unmarshal(fetched.State, &payload); err != nil {
		t.Fatalf("decode state payload: %v", err)
	}
	if len(payload.UIState.SelectedActorIDs) != 2 {
		t.Fatalf("state payload did not round-trip: %s", string(fetched.State))
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/graph-states/%d", created.ID), nil)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("unexpected delete status %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/graph-states/%d", created.ID), nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d: %s", missingRec.Code, missingRec.Body.String())
	}
}

func TestV1GraphStateRejectsInvalidRequests(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	cases := []struct {
		name string
		body string
	}{
		{name: "invalid kind", body: `{"kind":"bogus","name":"x","state":{"a":1}}`},
		{name: "missing name", body: `{"kind":"actor-graph","name":"  ","state":{"a":1}}`},
		{name: "missing state", body: `{"kind":"actor-graph","name":"x"}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/graph-states", bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

func TestV1ActorGraphProgressUnknownToken(t *testing.T) {
	handler, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/actor-graph/progress/nope", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown token, got %d: %s", rec.Code, rec.Body.String())
	}
}
