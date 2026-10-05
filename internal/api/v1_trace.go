package api

import (
	"net/http"

	"chain-analysis-app/internal/app"
)

func (h *V1) handleStartTraceJob(w http.ResponseWriter, r *http.Request) {
	var req app.TraceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	job, err := h.services.Trace.Start(req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (h *V1) handleTraceRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.services.Trace.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *V1) handleTraceRun(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "trace id")
	if err != nil {
		writeError(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		if err := h.services.Trace.Delete(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	run, err := h.services.Trace.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
