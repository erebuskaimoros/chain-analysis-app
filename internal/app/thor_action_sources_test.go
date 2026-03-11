package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestFetchMidgardActionsForAddressMergesLegacyTHORHistoryDespiteMidgardOnlyCache(t *testing.T) {
	const address = "thor16qnm285eez48r4u9whedq4qunydu2ucmzchz7p"

	recent := testTHORSendAction(
		"1706436517773653418",
		"6520659",
		"5E1A1215C81FA83171EA3C58B5127471C32FE50912A5904645181490B89C15C9",
		address,
		"thor1recentrecipient0000000000000000000000000",
		"31018731723188",
	)
	legacy := testTHORSendAction(
		"1637020800000000000",
		"4473241",
		"33683E29BAF72550E4DCA33A5AE596869FAA3DA8F8A5F6D5D1859E22BBC9781D",
		address,
		"thor1lh9wa6csnfw76rp7zwy8sve2va9xxwytu5kxyj",
		"250000000000000",
	)

	midgardCalls := 0
	midgard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		midgardCalls++
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{recent}})
	}))
	defer midgard.Close()

	legacyCalls := 0
	legacySource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		legacyCalls++
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{recent, legacy}})
	}))
	defer legacySource.Close()

	app, err := New(Config{
		DBPath:                filepath.Join(t.TempDir(), "legacy-thor-history.db"),
		StaticDir:             "internal/web/static",
		ThornodeEndpoints:     []string{midgard.URL},
		MidgardEndpoints:      []string{midgard.URL},
		LegacyActionEndpoints: []string{legacySource.URL},
		RequestTimeout:        5 * time.Second,
		MidgardTimeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	start := time.Unix(0, 0).UTC()
	end := time.Unix(1_800_000_000, 0).UTC()
	startTS, endTS := actionHistoryQueryBounds(start, end)
	if err := insertMidgardActionCache(context.Background(), app.db, address, startTS, endTS, 1, false, []midgardAction{recent}); err != nil {
		t.Fatalf("insert stale cache: %v", err)
	}

	actions, truncated, err := app.fetchMidgardActionsForAddress(context.Background(), address, start, end, 1)
	if err != nil {
		t.Fatalf("fetch merged THOR history: %v", err)
	}
	if truncated {
		t.Fatal("expected non-truncated merged history")
	}
	if len(actions) != 2 {
		t.Fatalf("expected merged history to contain 2 unique actions, got %#v", actions)
	}
	if got := actions[0].Height; got != recent.Height {
		t.Fatalf("expected newest action first, got height %q", got)
	}
	if got := actions[1].Height; got != legacy.Height {
		t.Fatalf("expected legacy pre-fork action second, got height %q", got)
	}
	if legacyCalls == 0 {
		t.Fatal("expected legacy source to be queried")
	}

	midgardCalls = 0
	legacyCalls = 0
	cachedActions, cachedTruncated, err := app.fetchMidgardActionsForAddress(context.Background(), address, start, end, 1)
	if err != nil {
		t.Fatalf("fetch cached merged THOR history: %v", err)
	}
	if cachedTruncated {
		t.Fatal("expected cached merged history to remain non-truncated")
	}
	if len(cachedActions) != 2 {
		t.Fatalf("expected cached merged history to contain 2 actions, got %#v", cachedActions)
	}
	if midgardCalls != 0 || legacyCalls != 0 {
		t.Fatalf("expected merged cache hit to avoid upstream calls, midgard=%d legacy=%d", midgardCalls, legacyCalls)
	}
}

func TestFetchMidgardActionsForAddressReturnsPartialMergedHistoryWhenLegacyPaginationFails(t *testing.T) {
	const address = "thor1xpxjq73gyy2y6yfh6a5l32ygrw23n4feanzr6z"

	page0 := make([]midgardAction, 0, midgardActionsPageLimit)
	for i := 0; i < midgardActionsPageLimit; i++ {
		page0 = append(page0, testTHORSendAction(
			strconv.FormatInt(time.Unix(1_700_000_000+int64(i), 0).UTC().UnixNano(), 10),
			strconv.Itoa(4_700_000+i),
			"LEGACY-"+strconv.Itoa(i),
			address,
			"thor1legacyrecipient0000000000000000000000000",
			strconv.Itoa(1_000_000+i),
		))
	}
	midgardOnly := []midgardAction{page0[0]}

	midgard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: midgardOnly})
	}))
	defer midgard.Close()

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		if offset := r.URL.Query().Get("offset"); offset != "" && offset != "0" {
			http.Error(w, "legacy page timeout", http.StatusGatewayTimeout)
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: page0,
			Meta: midgardActionsMeta{
				NextPageToken: "next-page",
			},
		})
	}))
	defer legacy.Close()

	app, err := New(Config{
		DBPath:                filepath.Join(t.TempDir(), "partial-legacy-history.db"),
		StaticDir:             "internal/web/static",
		ThornodeEndpoints:     []string{midgard.URL},
		MidgardEndpoints:      []string{midgard.URL},
		LegacyActionEndpoints: []string{legacy.URL},
		RequestTimeout:        5 * time.Second,
		MidgardTimeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	actions, truncated, err := app.fetchMidgardActionsForAddress(
		context.Background(),
		address,
		time.Unix(0, 0).UTC(),
		time.Unix(1_800_000_000, 0).UTC(),
		2,
	)
	if err != nil {
		t.Fatalf("expected partial merged history, got error: %v", err)
	}
	if !truncated {
		t.Fatal("expected partial merged history to be marked truncated")
	}
	if len(actions) != midgardActionsPageLimit {
		t.Fatalf("expected %d merged actions from successful pages, got %d", midgardActionsPageLimit, len(actions))
	}
	if got := actions[0].Height; got == "" {
		t.Fatal("expected merged actions to remain canonicalized and non-empty")
	}
}

func TestFetchMidgardActionsForAddressPagedReturnsPartialMergedHistoryWhenLegacyPaginationFails(t *testing.T) {
	const address = "thor1xpxjq73gyy2y6yfh6a5l32ygrw23n4feanzr6z"

	page0 := make([]midgardAction, 0, midgardActionsPageLimit)
	for i := 0; i < midgardActionsPageLimit; i++ {
		page0 = append(page0, testTHORSendAction(
			strconv.FormatInt(time.Unix(1_700_100_000+int64(i), 0).UTC().UnixNano(), 10),
			strconv.Itoa(4_800_000+i),
			"PAGED-"+strconv.Itoa(i),
			address,
			"thor1legacypagedrecipient000000000000000000",
			strconv.Itoa(2_000_000+i),
		))
	}

	midgard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		offset := r.URL.Query().Get("offset")
		if offset == "0" {
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{page0[0]}})
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{}})
	}))
	defer midgard.Close()

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		if offset := r.URL.Query().Get("offset"); offset != "" && offset != "0" {
			http.Error(w, "legacy page timeout", http.StatusGatewayTimeout)
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: page0,
			Meta: midgardActionsMeta{
				NextPageToken: "next-page",
			},
		})
	}))
	defer legacy.Close()

	app, err := New(Config{
		DBPath:                filepath.Join(t.TempDir(), "partial-legacy-paged-history.db"),
		StaticDir:             "internal/web/static",
		ThornodeEndpoints:     []string{midgard.URL},
		MidgardEndpoints:      []string{midgard.URL},
		LegacyActionEndpoints: []string{legacy.URL},
		RequestTimeout:        5 * time.Second,
		MidgardTimeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	actions, truncated, err := app.fetchMidgardActionsForAddressPaged(
		context.Background(),
		address,
		time.Unix(0, 0).UTC(),
		time.Unix(1_800_000_000, 0).UTC(),
		0,
		2,
	)
	if err != nil {
		t.Fatalf("expected partial paged merged history, got error: %v", err)
	}
	if !truncated {
		t.Fatal("expected partial paged merged history to be marked truncated")
	}
	if len(actions) != midgardActionsPageLimit {
		t.Fatalf("expected %d paged merged actions from successful pages, got %d", midgardActionsPageLimit, len(actions))
	}
}

func TestFetchLegacyActionsForAddressUsesOffsetPagination(t *testing.T) {
	const address = "thor1xpxjq73gyy2y6yfh6a5l32ygrw23n4feanzr6z"

	var (
		mu      sync.Mutex
		queries []map[string]string
	)

	page0 := make([]midgardAction, 0, midgardActionsPageLimit)
	for i := 0; i < midgardActionsPageLimit; i++ {
		page0 = append(page0, testTHORSendAction(
			strconv.FormatInt(time.Unix(1_700_000_000+int64(i), 0).UTC().UnixNano(), 10),
			strconv.Itoa(4_700_000+i),
			"PAGE0-"+strconv.Itoa(i),
			address,
			"thor1legacyrecipient0000000000000000000000000",
			strconv.Itoa(1_000_000+i),
		))
	}
	page1 := testTHORSendAction(
		"1699999949000000000",
		"4699949",
		"PAGE1",
		address,
		"thor1legacyrecipient0000000000000000000000000",
		"999999",
	)

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		query := map[string]string{
			"address":       r.URL.Query().Get("address"),
			"fromTimestamp": r.URL.Query().Get("fromTimestamp"),
			"timestamp":     r.URL.Query().Get("timestamp"),
			"limit":         r.URL.Query().Get("limit"),
			"offset":        r.URL.Query().Get("offset"),
			"nextPageToken": r.URL.Query().Get("nextPageToken"),
		}
		mu.Lock()
		queries = append(queries, query)
		mu.Unlock()

		if query["offset"] == "0" {
			_ = json.NewEncoder(w).Encode(midgardActionsResponse{
				Actions: page0,
				Meta: midgardActionsMeta{
					NextPageToken: "next-page",
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{Actions: []midgardAction{page1}})
	}))
	defer legacy.Close()

	app, err := New(Config{
		DBPath:                filepath.Join(t.TempDir(), "legacy-offset-query-shape.db"),
		StaticDir:             "internal/web/static",
		ThornodeEndpoints:     []string{legacy.URL},
		MidgardEndpoints:      []string{legacy.URL},
		LegacyActionEndpoints: []string{legacy.URL},
		RequestTimeout:        5 * time.Second,
		MidgardTimeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.Close()

	actions, truncated, err := app.fetchLegacyActionsForAddress(
		context.Background(),
		address,
		time.Unix(0, 0).UTC(),
		time.Unix(1_800_000_000, 0).UTC(),
		2,
	)
	if err != nil {
		t.Fatalf("fetch legacy actions: %v", err)
	}
	if truncated {
		t.Fatal("expected two full offset pages to remain non-truncated")
	}
	if len(actions) != midgardActionsPageLimit+1 {
		t.Fatalf("expected %d actions across two pages, got %d", midgardActionsPageLimit+1, len(actions))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("expected 2 upstream requests, got %d", len(queries))
	}
	if queries[0]["fromTimestamp"] != "0" {
		t.Fatalf("expected first page to include fromTimestamp=0, got %#v", queries[0])
	}
	if queries[0]["offset"] != "0" {
		t.Fatalf("expected first page to use offset=0, got %#v", queries[0])
	}
	if queries[1]["fromTimestamp"] != "0" {
		t.Fatalf("expected offset follow-up page to keep fromTimestamp, got %#v", queries[1])
	}
	if queries[1]["offset"] != strconv.Itoa(midgardActionsPageLimit) {
		t.Fatalf("expected second page to use offset=%d, got %#v", midgardActionsPageLimit, queries[1])
	}
	if queries[1]["nextPageToken"] != "" {
		t.Fatalf("expected offset follow-up page to omit nextPageToken, got %#v", queries[1])
	}
}

func testTHORSendAction(date, height, txID, from, to, amount string) midgardAction {
	return midgardAction{
		Date:   date,
		Height: height,
		Type:   "send",
		Status: "success",
		In: []midgardActionLeg{{
			Address: from,
			TxID:    txID,
			Coins:   []midgardActionCoin{{Asset: "THOR.RUNE", Amount: amount}},
		}},
		Out: []midgardActionLeg{{
			Address: to,
			TxID:    txID,
			Coins:   []midgardActionCoin{{Asset: "THOR.RUNE", Amount: amount}},
		}},
	}
}

func TestSortMidgardActionsNewestFirstUsesHeightTieBreak(t *testing.T) {
	actions := []midgardAction{
		testTHORSendAction("1700000000000000000", "10", "A", "thor1a", "thor1b", "1"),
		testTHORSendAction("1700000000000000000", "11", "B", "thor1a", "thor1b", "1"),
	}
	sortMidgardActionsNewestFirst(actions)
	if got := actions[0].Height; got != "11" {
		t.Fatalf("expected higher height to sort first on identical timestamps, got %q", got)
	}
}

func TestActionHistoryQueryBoundsClampsNegativeStart(t *testing.T) {
	start, end := actionHistoryQueryBounds(time.Unix(-1, 0).UTC(), time.Unix(5, 0).UTC())
	if start != 0 || end != 5 {
		t.Fatalf("unexpected bounds start=%d end=%d", start, end)
	}
}

func BenchmarkMergeMidgardActions(b *testing.B) {
	primary := make([]midgardAction, 0, 100)
	secondary := make([]midgardAction, 0, 100)
	for i := 0; i < 100; i++ {
		ts := strconv.FormatInt(time.Unix(1_700_000_000+int64(i), 0).UTC().UnixNano(), 10)
		height := strconv.Itoa(1_000_000 + i)
		primary = append(primary, testTHORSendAction(ts, height, "P"+height, "thor1from", "thor1to", "1"))
		secondary = append(secondary, testTHORSendAction(ts, height, "L"+height, "thor1from", "thor1to", "1"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mergeMidgardActions(primary, secondary)
	}
}
