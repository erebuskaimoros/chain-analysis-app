package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// etherscanStub answers Etherscan-style balance requests through respond and
// counts them; everything else is an empty pool list or 404.
type etherscanStub struct {
	balanceCalls atomic.Int64
	nodesCalls   atomic.Int64
	respond      func(call int64, w http.ResponseWriter)
}

func (s *etherscanStub) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/pools":
		_ = json.NewEncoder(w).Encode([]MidgardPool{})
	case r.URL.Path == "/thorchain/nodes":
		s.nodesCalls.Add(1)
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	case r.URL.Query().Get("action") == "balance":
		s.respond(s.balanceCalls.Add(1), w)
	default:
		http.NotFound(w, r)
	}
}

func okEtherscanBalance(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "1", "message": "OK", "result": "500000000000000000"})
}

func newJobsTestApp(t *testing.T, stub *etherscanStub) (*App, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(stub.handler))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	app, err := New(Config{
		DBPath:            filepath.Join(dir, "jobs.db"),
		LastRunLogPath:    filepath.Join(dir, "logs", "last-run.log"),
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		EtherscanAPIURL:   server.URL,
		EtherscanAPIKey:   "test-key",
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app, dir
}

func watchedETHNode() FlowNode {
	return FlowNode{ID: "external_address:0xwatch", Kind: "external_address", Chain: "ETH", Metrics: map[string]any{"address": "0xwatch"}}
}

func waitForJob(t *testing.T, app *App, id string) JobSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		snap, ok := app.Job(id, false)
		if !ok {
			t.Fatalf("job %s not found", id)
		}
		if snap.Status != JobRunning {
			return snap
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return JobSnapshot{}
}

func liveHoldingsJobNode(t *testing.T, snap JobSnapshot) map[string]any {
	t.Helper()
	result, ok := snap.Result.(LiveHoldingsJobResult)
	if !ok || len(result.Nodes) != 1 {
		t.Fatalf("unexpected live holdings result %#v (status %s, error %s)", snap.Result, snap.Status, snap.Error)
	}
	return result.Nodes[0].Metrics
}

func TestLiveHoldingsJobStopsOnBannedProviderWithoutRetrying(t *testing.T) {
	stub := &etherscanStub{respond: func(_ int64, w http.ResponseWriter) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}}
	app, _ := newJobsTestApp(t, stub)

	snap := waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, false).ID)
	metrics := liveHoldingsJobNode(t, snap)
	if metrics["live_holdings_status"] != "error" || metrics["live_holdings_error_kind"] != string(providerErrBanned) {
		t.Fatalf("expected a final banned error, got %v", metrics)
	}
	if got := stub.balanceCalls.Load(); got != 1 {
		t.Fatalf("expected one request to the banning provider, got %d", got)
	}

	// The ban is final for this node and recorded, so a second job reuses it.
	waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, false).ID)
	if got := stub.balanceCalls.Load(); got != 1 {
		t.Fatalf("expected the recorded ban to be reused without new requests, got %d", got)
	}
}

func TestLiveHoldingsJobRetriesRateLimitAfterRetryAfter(t *testing.T) {
	var firstFailedAt atomic.Int64
	var retriedAt atomic.Int64
	stub := &etherscanStub{respond: func(call int64, w http.ResponseWriter) {
		if call == 1 {
			firstFailedAt.Store(time.Now().UnixMilli())
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		retriedAt.CompareAndSwap(0, time.Now().UnixMilli())
		okEtherscanBalance(w)
	}}
	app, _ := newJobsTestApp(t, stub)

	snap := waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, false).ID)
	metrics := liveHoldingsJobNode(t, snap)
	if metrics["live_holdings_status"] != "available" {
		t.Fatalf("expected the rate-limited lookup to succeed on retry, got %v", metrics)
	}
	if _, stale := metrics["live_holdings_error_kind"]; stale {
		t.Fatalf("expected the error kind to clear after success, got %v", metrics)
	}
	if gap := retriedAt.Load() - firstFailedAt.Load(); gap < 900 {
		t.Fatalf("expected the retry to wait for Retry-After (1s), waited %dms", gap)
	}
}

func TestLiveHoldingsJobReusesSnapshotsUnlessForced(t *testing.T) {
	stub := &etherscanStub{respond: func(_ int64, w http.ResponseWriter) { okEtherscanBalance(w) }}
	app, _ := newJobsTestApp(t, stub)

	waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, false).ID)
	reused := waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, false).ID)
	if got := stub.balanceCalls.Load(); got != 1 {
		t.Fatalf("expected a fresh snapshot to be reused, got %d requests", got)
	}
	if metrics := liveHoldingsJobNode(t, reused); metrics["live_holdings_status"] != "available" {
		t.Fatalf("expected reused snapshot metrics, got %v", metrics)
	}
	waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, true).ID)
	if got := stub.balanceCalls.Load(); got != 2 {
		t.Fatalf("expected force to refetch, got %d requests", got)
	}
}

func TestProtocolBondIndexesAreSharedAcrossLookups(t *testing.T) {
	stub := &etherscanStub{respond: func(_ int64, w http.ResponseWriter) { okEtherscanBalance(w) }}
	app, _ := newJobsTestApp(t, stub)
	for i := 0; i < 3; i++ {
		if _, _, _, err := app.fetchProtocolBondIndexes(context.Background(), sourceProtocolTHOR); err != nil {
			t.Fatalf("fetch bond indexes: %v", err)
		}
	}
	if got := stub.nodesCalls.Load(); got != 1 {
		t.Fatalf("expected one /thorchain/nodes fetch, got %d", got)
	}
}

func TestLiveHoldingsJobKeepsBuildLogAndWritesItsOwnLog(t *testing.T) {
	stub := &etherscanStub{respond: func(_ int64, w http.ResponseWriter) { okEtherscanBalance(w) }}
	app, _ := newJobsTestApp(t, stub)
	if err := os.MkdirAll(filepath.Dir(app.cfg.LastRunLogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app.cfg.LastRunLogPath, []byte("build log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap := waitForJob(t, app, app.StartLiveHoldingsJob([]FlowNode{watchedETHNode()}, false).ID)
	if got, _ := os.ReadFile(app.cfg.LastRunLogPath); string(got) != "build log\n" {
		t.Fatalf("live holdings job overwrote the build log: %q", got)
	}
	if snap.LogPath == "" {
		t.Fatal("expected the job to report its own log path")
	}
	if info, err := os.Stat(snap.LogPath); err != nil || info.Size() == 0 {
		t.Fatalf("expected a per-job log at %s: %v", snap.LogPath, err)
	}
}

func TestJobRunnerReportsFailureAndCancellation(t *testing.T) {
	runner := newJobRunner(t.TempDir())
	failed := runner.start(JobActorGraphBuild, time.Minute, "", func(ctx context.Context) (any, error) {
		return nil, errors.New("boom")
	})
	blocked := runner.start(JobActorGraphBuild, time.Minute, "", func(ctx context.Context) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if !runner.cancel(blocked.ID) {
		t.Fatal("expected cancel to find the job")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f, _ := runner.get(failed.ID)
		b, _ := runner.get(blocked.ID)
		if f.Status != JobRunning && b.Status != JobRunning {
			if f.Status != JobFailed || f.Error != "boom" {
				t.Fatalf("expected failed job, got %#v", f)
			}
			if b.Status != JobCanceled {
				t.Fatalf("expected canceled job, got %#v", b)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("jobs did not finish")
}
