package api

import (
	"errors"
	"net/http"
)

func (h *V1) handleActorMonitor(w http.ResponseWriter, r *http.Request) {
	actorID, err := parsePathInt64(r.PathValue("id"), "actor id")
	if err != nil {
		writeError(w, err)
		return
	}
	monitor, err := h.services.Monitor.Monitor(r.Context(), actorID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, monitor)
}

func (h *V1) handleActorWatch(w http.ResponseWriter, r *http.Request) {
	actorID, err := parsePathInt64(r.PathValue("id"), "actor id")
	if err != nil {
		writeError(w, err)
		return
	}
	var req struct {
		Watch bool `json:"watch"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.services.Monitor.SetWatch(r.Context(), actorID, req.Watch); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "watch": req.Watch})
}

func (h *V1) handleActorViewed(w http.ResponseWriter, r *http.Request) {
	actorID, err := parsePathInt64(r.PathValue("id"), "actor id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.services.Monitor.MarkViewed(r.Context(), actorID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *V1) handleStartActorRefreshJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActorID int64 `json:"actor_id"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.ActorID <= 0 {
		writeError(w, errors.New("actor_id is required"))
		return
	}
	writeJSON(w, http.StatusAccepted, h.services.Monitor.StartRefresh(req.ActorID))
}
