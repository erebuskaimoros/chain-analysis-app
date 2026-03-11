package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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
