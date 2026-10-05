package api

import (
	"errors"
	"net/http"
	"strings"

	"chain-analysis-app/internal/api/dto"
	"chain-analysis-app/internal/app"
)

func (h *V1) handleStartActorGraphJob(w http.ResponseWriter, r *http.Request) {
	var req app.ActorTrackerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.services.Jobs.StartActorGraphBuild(req))
}

func (h *V1) handleStartActorGraphExpandJob(w http.ResponseWriter, r *http.Request) {
	var req app.ActorTrackerExpandRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.services.Jobs.StartActorGraphExpand(req))
}

func (h *V1) handleStartAddressExplorerJob(w http.ResponseWriter, r *http.Request) {
	var req app.AddressExplorerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, h.services.Jobs.StartAddressExplorer(req))
}

func (h *V1) handleStartLiveHoldingsJob(w http.ResponseWriter, r *http.Request) {
	var req dto.LiveHoldingsJobRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if len(req.Nodes) == 0 {
		writeError(w, errors.New("at least one node is required"))
		return
	}
	nodes := make([]app.FlowNode, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		nodes = append(nodes, app.FlowNode{
			ID:      strings.TrimSpace(node.ID),
			Kind:    strings.TrimSpace(node.Kind),
			Chain:   strings.TrimSpace(node.Chain),
			Metrics: node.Metrics,
		})
	}
	writeJSON(w, http.StatusAccepted, h.services.Jobs.StartLiveHoldings(nodes, req.Force))
}

func (h *V1) handleJob(w http.ResponseWriter, r *http.Request) {
	includePartial := r.URL.Query().Get("partial") == "1"
	snap, ok := h.services.Jobs.Get(r.PathValue("id"), includePartial)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (h *V1) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if !h.services.Jobs.Cancel(r.PathValue("id")) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
