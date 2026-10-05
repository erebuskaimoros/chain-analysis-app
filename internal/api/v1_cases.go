package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chain-analysis-app/internal/app"
)

func (h *V1) handleCases(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req app.CaseUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		created, err := h.services.Cases.Create(r.Context(), req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
		return
	}
	cases, err := h.services.Cases.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cases)
}

func (h *V1) handleCaseByID(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "case id")
	if err != nil {
		writeError(w, err)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req app.CaseUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		updated, err := h.services.Cases.Update(r.Context(), id, req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.services.Cases.Delete(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		found, err := h.services.Cases.Get(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, found)
	}
}

func (h *V1) handleCaseItems(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "case id")
	if err != nil {
		writeError(w, err)
		return
	}
	var req app.CaseItemRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	item, err := h.services.Cases.AddItem(r.Context(), id, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *V1) handleCaseItemDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "case id")
	if err != nil {
		writeError(w, err)
		return
	}
	itemID, err := parsePathInt64(r.PathValue("item_id"), "case item id")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.services.Cases.DeleteItem(r.Context(), id, itemID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *V1) handleCaseExport(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathInt64(r.PathValue("id"), "case id")
	if err != nil {
		writeError(w, err)
		return
	}
	switch format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))); format {
	case "", "md", "markdown":
		report, err := h.services.Cases.ExportMarkdown(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=case-%d.md", id))
		_, _ = w.Write([]byte(report))
	case "csv":
		data, err := h.services.Cases.ExportCSV(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=case-%d-flows.csv", id))
		_, _ = w.Write(data)
	default:
		writeError(w, fmt.Errorf("invalid export format %q: want md or csv", format))
	}
}

func (h *V1) handleStartAddressProfileJob(w http.ResponseWriter, r *http.Request) {
	var req app.AddressProfileRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Address) == "" {
		writeError(w, errors.New("address is required"))
		return
	}
	writeJSON(w, http.StatusAccepted, h.services.Investigation.StartAddressProfile(req))
}

func (h *V1) handleStartLabelsImportJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if strings.TrimSpace(req.Source) == "" || strings.TrimSpace(req.Path) == "" {
		writeError(w, errors.New("source and path are required"))
		return
	}
	writeJSON(w, http.StatusAccepted, h.services.Investigation.StartLabelsImport(req.Source, req.Path))
}

func parseQueryTime(r *http.Request, name string) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s: want RFC 3339", name)
	}
	return t, nil
}

func (h *V1) handleLedgerCoverage(w http.ResponseWriter, r *http.Request) {
	start, err := parseQueryTime(r, "start")
	if err != nil {
		writeError(w, err)
		return
	}
	end, err := parseQueryTime(r, "end")
	if err != nil {
		writeError(w, err)
		return
	}
	report, err := h.services.Investigation.LedgerCoverage(r.Context(), r.URL.Query().Get("address"), start, end)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
