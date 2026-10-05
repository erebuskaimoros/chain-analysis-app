package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"chain-analysis-app/internal/api"
	"chain-analysis-app/internal/app"
	"chain-analysis-app/internal/domain/services"
)

func newAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	legacy, err := app.New(app.Config{DBPath: filepath.Join(t.TempDir(), "cactl.db"), RequestTimeout: time.Second, MidgardTimeout: time.Second})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	mux := http.NewServeMux()
	api.NewV1(services.New(legacy)).Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		server.Close()
		_ = legacy.Close()
	})
	return server
}

func runCactl(t *testing.T, url string, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), append([]string{"--url", url}, args...), &stdout, &stderr)
	return stdout.String(), err
}

func TestCactlCaseWorkflow(t *testing.T) {
	server := newAPIServer(t)
	out, err := runCactl(t, server.URL, "case", "create", "Bitget hack", "--notes", "Reported 2026-09-28.")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created app.Case
	if err := json.Unmarshal([]byte(out), &created); err != nil || created.ID == 0 {
		t.Fatalf("expected the created case as JSON, got %q (%v)", out, err)
	}
	id := strconv.FormatInt(created.ID, 10)
	if _, err := runCactl(t, server.URL, "case", "add", id, "address", "ETH|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3", "--note", "exploiter"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := runCactl(t, server.URL, "case", "add", id, "wallet", "x"); err == nil || !strings.Contains(err.Error(), "invalid case item kind") {
		t.Fatalf("expected the API's validation error, got %v", err)
	}
	report, err := runCactl(t, server.URL, "case", "export", id)
	if err != nil || !strings.HasPrefix(report, "# Bitget hack") || !strings.Contains(report, "exploiter") {
		t.Fatalf("expected the Markdown report, got %q (%v)", report, err)
	}
	csvOut, err := runCactl(t, server.URL, "case", "export", id, "--format", "csv")
	if err != nil || !strings.HasPrefix(csvOut, "trace_run_id,time,") {
		t.Fatalf("expected the CSV header, got %q (%v)", csvOut, err)
	}
	coverage, err := runCactl(t, server.URL, "ledger", "gaps", "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3")
	if err != nil || !strings.Contains(coverage, `"sources": []`) {
		t.Fatalf("expected an empty coverage report, got %q (%v)", coverage, err)
	}
}

func TestCactlRejectsBadInvocations(t *testing.T) {
	server := newAPIServer(t)
	for _, args := range [][]string{
		{"trace", "--start", "2026-09-28T03:00:00Z"},
		{"bogus"},
		{"actor", "refresh"},
		{"ledger", "gaps", "x", "--start", "yesterday"},
	} {
		if _, err := runCactl(t, server.URL, args...); err == nil {
			t.Fatalf("expected %v to fail", args)
		}
	}
}

func TestParseTraceFlagsBuildsSeeds(t *testing.T) {
	req, err := parseTraceFlags([]string{"--seed", "eth|0xabc", "--seed", "thor1xyz", "--tx", "0xA7", "--policy", "haircut", "--stop", ""}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(req.Seeds) != 3 || req.Seeds[0].Chain != "ETH" || req.Seeds[1].Chain != "" || req.Seeds[2].TxID != "0xA7" || req.Policy != "haircut" {
		t.Fatalf("unexpected request %+v", req)
	}
	if req.StopCategories == nil || len(req.StopCategories) != 0 {
		t.Fatalf("expected --stop \"\" to send no stop categories, got %#v", req.StopCategories)
	}
}
