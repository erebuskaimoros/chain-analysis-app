package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestLedgerGaps(t *testing.T) {
	cases := []struct {
		name    string
		covered []ledgerInterval
		from    int64
		to      int64
		want    []ledgerInterval
	}{
		{"nothing covered", nil, 10, 20, []ledgerInterval{{10, 20}}},
		{"fully covered", []ledgerInterval{{0, 100}}, 10, 20, nil},
		{"covered head", []ledgerInterval{{0, 14}}, 10, 20, []ledgerInterval{{15, 20}}},
		{"covered tail", []ledgerInterval{{15, 30}}, 10, 20, []ledgerInterval{{10, 14}}},
		{"hole in middle newest first", []ledgerInterval{{10, 12}, {16, 20}}, 10, 20, []ledgerInterval{{13, 15}}},
		{"two holes newest first", []ledgerInterval{{12, 13}, {16, 17}}, 10, 20, []ledgerInterval{{18, 20}, {14, 15}, {10, 11}}},
		{"unrelated interval", []ledgerInterval{{50, 60}}, 10, 20, []ledgerInterval{{10, 20}}},
	}
	for _, tc := range cases {
		if got := ledgerGaps(tc.covered, tc.from, tc.to); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestMarkLedgerCoveredMergesOverlappingAndAdjacentIntervals(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	now := time.Now()
	for _, iv := range []ledgerInterval{{10, 20}, {30, 40}, {21, 29}, {35, 50}, {100, 110}} {
		if err := markLedgerCovered(ctx, db, "s", "addr", iv.From, iv.To, now); err != nil {
			t.Fatalf("mark %v: %v", iv, err)
		}
	}
	got, err := loadLedgerCoverage(ctx, db, "s", "addr")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []ledgerInterval{{10, 50}, {100, 110}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// midgardWindowServer serves /actions for one address from a fixed action set,
// filtered by fromTimestamp/timestamp, and records each requested window.
type midgardWindowServer struct {
	mu       sync.Mutex
	actions  []midgardAction // newest first
	requests [][2]int64
}

func (s *midgardWindowServer) handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/actions" {
		http.NotFound(w, r)
		return
	}
	from, _ := strconv.ParseInt(r.URL.Query().Get("fromTimestamp"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("timestamp"), 10, 64)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	s.mu.Lock()
	s.requests = append(s.requests, [2]int64{from, to})
	s.mu.Unlock()
	var window []midgardAction
	for _, action := range s.actions {
		ts := parseMidgardActionTime(action.Date).Unix()
		if ts >= from && ts <= to {
			window = append(window, action)
		}
	}
	if offset > len(window) {
		offset = len(window)
	}
	end := min(offset+midgardActionsPageLimit, len(window))
	_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: window[offset:end]})
}

func (s *midgardWindowServer) takeRequests() [][2]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.requests
	s.requests = nil
	return out
}

func newLedgerTestApp(t *testing.T, server *httptest.Server) *App {
	t.Helper()
	app, err := New(Config{
		DBPath:            filepath.Join(t.TempDir(), "ledger.db"),
		ThornodeEndpoints: []string{server.URL},
		MidgardEndpoints:  []string{server.URL},
		RequestTimeout:    5 * time.Second,
		MidgardTimeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

func dayStart(day int) time.Time {
	return time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC)
}

func TestFetchMidgardActionsFetchesOnlyUncoveredRanges(t *testing.T) {
	const address = "thor1ledgerseed000000000000000000000000000000"
	upstream := &midgardWindowServer{}
	for day := 4; day >= 1; day-- {
		ts := dayStart(day).Add(12 * time.Hour)
		upstream.actions = append(upstream.actions, testTHORSendAction(
			strconv.FormatInt(ts.UnixNano(), 10), strconv.Itoa(1000+day), "TX-DAY-"+strconv.Itoa(day),
			address, "thor1recipient000000000000000000000000000000", "100"))
	}
	server := httptest.NewServer(http.HandlerFunc(upstream.handler))
	defer server.Close()
	app := newLedgerTestApp(t, server)
	ctx := context.Background()

	first, truncated, err := app.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, sourceProtocolTHOR, address, dayStart(1), dayStart(3), 5)
	if err != nil || truncated {
		t.Fatalf("first fetch: err=%v truncated=%v", err, truncated)
	}
	if len(first) != 2 {
		t.Fatalf("expected days 1-2 in [day1, day3], got %d actions", len(first))
	}
	if reqs := upstream.takeRequests(); len(reqs) != 1 || reqs[0] != [2]int64{dayStart(1).Unix(), dayStart(3).Unix()} {
		t.Fatalf("expected one full-window request, got %v", reqs)
	}

	again, _, err := app.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, sourceProtocolTHOR, address, dayStart(1), dayStart(3), 5)
	if err != nil {
		t.Fatalf("repeat fetch: %v", err)
	}
	if !reflect.DeepEqual(again, first) {
		t.Fatalf("repeat fetch differs: %v vs %v", again, first)
	}
	if reqs := upstream.takeRequests(); len(reqs) != 0 {
		t.Fatalf("expected no upstream requests for a covered window, got %v", reqs)
	}

	shifted, _, err := app.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, sourceProtocolTHOR, address, dayStart(2), dayStart(5), 5)
	if err != nil {
		t.Fatalf("shifted fetch: %v", err)
	}
	if len(shifted) != 3 {
		t.Fatalf("expected days 2-4 in [day2, day5], got %d actions", len(shifted))
	}
	reqs := upstream.takeRequests()
	if len(reqs) != 1 || reqs[0] != [2]int64{dayStart(3).Unix() + 1, dayStart(5).Unix()} {
		t.Fatalf("expected only the uncovered (day3, day5] range to be fetched, got %v", reqs)
	}
	if got := shifted[0].Height; got != "1004" {
		t.Fatalf("expected newest action first, got height %s", got)
	}
}

func TestFetchMidgardActionsRefetchesRecentTail(t *testing.T) {
	const address = "thor1ledgertail00000000000000000000000000000"
	upstream := &midgardWindowServer{}
	server := httptest.NewServer(http.HandlerFunc(upstream.handler))
	defer server.Close()
	app := newLedgerTestApp(t, server)
	ctx := context.Background()

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	if _, _, err := app.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, sourceProtocolTHOR, address, start, end, 5); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	upstream.takeRequests()
	if _, _, err := app.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, sourceProtocolTHOR, address, start, end, 5); err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	reqs := upstream.takeRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected the recent tail to be fetched again, got %v", reqs)
	}
	if reqs[0][0] < end.Add(-ledgerTailLag).Unix()-1 || reqs[0][1] != end.Unix() {
		t.Fatalf("expected only the last %s to be re-fetched, got %v", ledgerTailLag, reqs)
	}
}

func TestBackfillLedgerFromQueryCaches(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// openTestDB applies migrations but not the app-level backfill, so the
	// retired tables still exist. Seed them as the old cache code wrote them.
	const address = "thor1backfill000000000000000000000000000000"
	actions := []midgardAction{testTHORSendAction("1790000000000000000", "500", "TX-BACKFILL", address, "thor1recipient000000000000000000000000000000", "7")}
	rawActions, _ := json.Marshal(actions)
	cachedAt := time.Unix(1_790_500_000, 0).UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO midgard_action_cache(address, start_ts, end_ts, max_pages, truncated, actions_json, action_count, cached_at)
		VALUES (?, ?, ?, 5, 0, ?, 1, ?)
	`, thorActionCachePrefix+address, 1_789_000_000, 1_791_000_000, string(rawActions), cachedAt); err != nil {
		t.Fatalf("seed action cache: %v", err)
	}
	transfers := []externalTransfer{{Chain: "ETH", Asset: "ETH.ETH", AmountRaw: "5", From: "0xa", To: "0xwatch", TxID: "0xT", Time: time.Unix(1_789_500_000, 0).UTC()}}
	rawTransfers, _ := json.Marshal(transfers)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO external_transfer_cache(provider, chain, address, start_ts, end_ts, max_pages, truncated, transfers_json, transfer_count, cached_at)
		VALUES ('etherscan', 'ETH', '0xwatch', ?, ?, 2, 0, ?, 1, ?)
	`, 1_789_000_000, 1_790_000_000, string(rawTransfers), cachedAt); err != nil {
		t.Fatalf("seed transfer cache: %v", err)
	}

	if err := backfillLedgerFromQueryCaches(ctx, db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	source := ledgerActionSource(sourceProtocolTHOR)
	gotActions, err := queryLedgerActions(ctx, db, source, address, 1_789_000_000, 1_791_000_000)
	if err != nil || len(gotActions) != 1 || gotActions[0].Height != "500" {
		t.Fatalf("expected backfilled action, got %v err=%v", gotActions, err)
	}
	coverage, _ := loadLedgerCoverage(ctx, db, source, address)
	// Coverage ends ledgerTailLag before the cache write time.
	wantEnd := time.Unix(1_790_500_000, 0).Add(-ledgerTailLag).Unix()
	if !reflect.DeepEqual(coverage, []ledgerInterval{{1_789_000_000, wantEnd}}) {
		t.Fatalf("unexpected action coverage %v", coverage)
	}
	transferSource := ledgerTransferSource("etherscan", "ETH")
	gotTransfers, err := queryLedgerTransfers(ctx, db, transferSource, "0xwatch", 1_789_000_000, 1_790_000_000)
	if err != nil || len(gotTransfers) != 1 || gotTransfers[0].TxID != "0xT" {
		t.Fatalf("expected backfilled transfer, got %v err=%v", gotTransfers, err)
	}
	for _, table := range []string{"midgard_action_cache", "external_transfer_cache"} {
		if exists, _ := sqliteTableExists(ctx, db, table); exists {
			t.Fatalf("expected %s to be dropped", table)
		}
	}
	if err := backfillLedgerFromQueryCaches(ctx, db); err != nil {
		t.Fatalf("second backfill should be a no-op: %v", err)
	}
}
