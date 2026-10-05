package api

import (
	"errors"
	"net/http"
	"strings"
)

func (h *V1) handleLabels(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	if address == "" {
		writeError(w, errors.New("address is required"))
		return
	}
	labels, err := h.services.Labels.ForAddress(r.Context(), address)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"address": address, "labels": labels})
}

func (h *V1) handleLabelSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.services.Labels.Sources(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}
