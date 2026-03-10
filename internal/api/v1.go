package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chain-analysis-app/internal/api/dto"
	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/domain/services"
)

type V1 struct {
	services *services.Container
}

func NewV1(svcs *services.Container) *V1 {
	return &V1{services: svcs}
}

func (h *V1) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", h.handleHealth)
	mux.HandleFunc("GET /api/v1/actions/{txid}", h.handleActionLookup)
	mux.HandleFunc("GET /api/v1/actors", h.handleActors)
	mux.HandleFunc("POST /api/v1/actors", h.handleActors)
	mux.HandleFunc("PUT /api/v1/actors/{id}", h.handleActorByID)
	mux.HandleFunc("DELETE /api/v1/actors/{id}", h.handleActorByID)
	mux.HandleFunc("GET /api/v1/annotations", h.handleAnnotations)
	mux.HandleFunc("PUT /api/v1/annotations", h.handleAnnotations)
	mux.HandleFunc("DELETE /api/v1/annotations", h.handleAnnotations)
	mux.HandleFunc("GET /api/v1/blocklist", h.handleBlocklist)
	mux.HandleFunc("POST /api/v1/blocklist", h.handleBlocklist)
	mux.HandleFunc("DELETE /api/v1/blocklist/{address}", h.handleBlocklistDelete)
	mux.HandleFunc("POST /api/v1/analysis/actor-graph", h.handleActorGraph)
	mux.HandleFunc("POST /api/v1/analysis/actor-graph/expand", h.handleActorGraphExpand)
	mux.HandleFunc("POST /api/v1/analysis/actor-graph/live-holdings", h.handleActorGraphLiveHoldings)
	mux.HandleFunc("POST /api/v1/analysis/address-explorer", h.handleAddressExplorer)
	mux.HandleFunc("GET /api/v1/runs/actor-graph", h.handleActorGraphRuns)
	mux.HandleFunc("DELETE /api/v1/runs/actor-graph/{id}", h.handleActorGraphRunDelete)
	mux.HandleFunc("GET /api/v1/runs/address-explorer", h.handleAddressExplorerRuns)
	mux.HandleFunc("DELETE /api/v1/runs/address-explorer/{id}", h.handleAddressExplorerRunDelete)
}

func (h *V1) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.services.Health.Get(r.Context()))
}

func (h *V1) handleActionLookup(w http.ResponseWriter, r *http.Request) {
	result, err := h.services.ActorGraph.LookupAction(r.Context(), r.PathValue("txid"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.ActionLookupResponse(result))
}

func (h *V1) handleActors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		actors, err := h.services.Actors.List(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.ActorListResponse{Actors: actors})
	case http.MethodPost:
		var req app.ActorUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		actor, err := h.services.Actors.Upsert(r.Context(), 0, req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, actor)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *V1) handleActorByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "actor id")
	if err != nil {
		writeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req app.ActorUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		actor, err := h.services.Actors.Upsert(r.Context(), id, req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, actor)
	case http.MethodDelete:
		if err := h.services.Actors.Delete(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *V1) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.services.Annotations.ListAnnotations(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.AnnotationListResponse{Annotations: items})
	case http.MethodPut:
		var req dto.AddressAnnotationUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		if err := h.services.Annotations.UpsertAnnotation(r.Context(), req.Address, req.Kind, req.Value); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		var req dto.AddressAnnotationUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		if err := h.services.Annotations.DeleteAnnotation(r.Context(), req.Address, req.Kind); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *V1) handleBlocklist(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.services.Annotations.ListBlocklist(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dto.BlocklistResponse{Addresses: items})
	case http.MethodPost:
		var req dto.BlocklistUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		if err := h.services.Annotations.AddToBlocklist(r.Context(), req.Address, req.Reason); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *V1) handleBlocklistDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.services.Annotations.RemoveFromBlocklist(r.Context(), r.PathValue("address")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *V1) handleActorGraph(w http.ResponseWriter, r *http.Request) {
	var req app.ActorTrackerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	resp, err := h.services.ActorGraph.Build(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := h.services.Runs.CreateActorGraphRun(r.Context(), req, joinActorNames(resp.Actors), len(resp.Nodes), len(resp.Edges)); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.ActorGraphResponse{
		Query:             resp.Query,
		Actors:            resp.Actors,
		Stats:             resp.Stats,
		Warnings:          resp.Warnings,
		Nodes:             resp.Nodes,
		Edges:             resp.Edges,
		SupportingActions: resp.SupportingActions,
	})
}

func (h *V1) handleActorGraphExpand(w http.ResponseWriter, r *http.Request) {
	var req app.ActorTrackerExpandRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	resp, err := h.services.ActorGraph.Expand(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.ActorGraphResponse{
		Query:             resp.Query,
		Actors:            resp.Actors,
		Stats:             resp.Stats,
		Warnings:          resp.Warnings,
		Nodes:             resp.Nodes,
		Edges:             resp.Edges,
		SupportingActions: resp.SupportingActions,
	})
}

func (h *V1) handleActorGraphLiveHoldings(w http.ResponseWriter, r *http.Request) {
	var req dto.LiveHoldingsRefreshRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	warnings, err := h.services.ActorGraph.RefreshLiveHoldings(r.Context(), req.Nodes)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.LiveHoldingsRefreshResponse{
		Nodes:       req.Nodes,
		Warnings:    warnings,
		RefreshedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (h *V1) handleAddressExplorer(w http.ResponseWriter, r *http.Request) {
	var req app.AddressExplorerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	resp, err := h.services.AddressExplorer.Build(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	if resp.Mode == "graph" {
		if _, err := h.services.Runs.CreateAddressExplorerRun(r.Context(), req, firstNonEmpty(resp.RunLabel, shortAddress(resp.Address)), len(resp.Nodes), len(resp.Edges)); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *V1) handleActorGraphRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.services.Runs.ListActorGraphRuns(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.ActorGraphRunsResponse{Runs: runs})
}

func (h *V1) handleActorGraphRunDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "run id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.services.Runs.DeleteActorGraphRun(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *V1) handleAddressExplorerRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.services.Runs.ListAddressExplorerRuns(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.AddressExplorerRunsResponse{Runs: runs})
}

func (h *V1) handleAddressExplorerRunDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "run id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.services.Runs.DeleteAddressExplorerRun(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeJSONBody(r *http.Request, out any) error {
	if r == nil || r.Body == nil {
		return errors.New("request body is required")
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case err == nil:
		status = http.StatusInternalServerError
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	case isBadRequest(err):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
}

func isBadRequest(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "required") ||
		strings.Contains(text, "duplicate") ||
		strings.Contains(text, "invalid") ||
		strings.Contains(text, "decode request body")
}

func parsePathInt64(raw, label string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("%s is required", label)
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", label)
	}
	return id, nil
}

func joinActorNames(actors []app.Actor) string {
	names := make([]string, 0, len(actors))
	for _, actor := range actors {
		if strings.TrimSpace(actor.Name) == "" {
			continue
		}
		names = append(names, actor.Name)
	}
	return strings.Join(names, ", ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func shortAddress(address string) string {
	address = strings.TrimSpace(address)
	if len(address) <= 16 {
		return address
	}
	return address[:8] + "..." + address[len(address)-6:]
}

func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}
