package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestActionSourceProtocolsForSeedRoutesNativeAndSharedAddresses(t *testing.T) {
	app := &App{
		mid:     NewThorClient([]string{"http://thor.example"}, time.Second),
		mayaMid: NewThorClient([]string{"http://maya.example"}, time.Second),
	}

	cases := []struct {
		name string
		seed frontierAddress
		want []string
	}{
		{
			name: "thor native stays on thor",
			seed: normalizeFrontierAddress("thor1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqef57d0"),
			want: []string{sourceProtocolTHOR},
		},
		{
			name: "maya native stays on maya",
			seed: normalizeFrontierAddress("maya1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqp6g7mk"),
			want: []string{sourceProtocolMAYA},
		},
		{
			name: "shared evm routes to both engines",
			seed: normalizeFrontierAddress("0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3"),
			want: []string{sourceProtocolTHOR, sourceProtocolMAYA},
		},
		{
			name: "shared btc routes to both engines",
			seed: normalizeFrontierAddress("bc1qmqzgaqlqpgymj0v7z5ll7qupskk3d88vpszhgs"),
			want: []string{sourceProtocolTHOR, sourceProtocolMAYA},
		},
		{
			name: "xrd routes to maya only",
			seed: normalizeFrontierAddress("account_rdx1c9k5t6m0c9k5t6m0c9k5t6m0c9k5t6m0c9k5t6m0c9k5"),
			want: []string{sourceProtocolMAYA},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := app.actionSourceProtocolsForSeed(tc.seed); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("actionSourceProtocolsForSeed(%#v) = %#v, want %#v", tc.seed, got, tc.want)
			}
		})
	}
}

func TestMidgardActionCacheKeysStayProtocolScoped(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	start := time.Unix(0, 0).UTC()
	end := time.Unix(1_800_000_000, 0).UTC()
	startTS, endTS := actionHistoryQueryBounds(start, end)
	address := "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3"

	thorAction := annotateMidgardActions([]midgardAction{
		testTHORSendAction("1700000000000000000", "10", "THOR-TX", address, "thor1recipient000000000000000000000000000000", "1"),
	}, sourceProtocolTHOR)
	mayaAction := annotateMidgardActions([]midgardAction{
		{
			Date:   "1700000001000000000",
			Height: "11",
			Type:   "send",
			Status: "success",
			In: []midgardActionLeg{{
				Address: address,
				TxID:    "MAYA-TX",
				Coins:   []midgardActionCoin{{Asset: "MAYA.CACAO", Amount: "2"}},
			}},
			Out: []midgardActionLeg{{
				Address: "maya1recipient000000000000000000000000000000",
				TxID:    "MAYA-TX",
				Coins:   []midgardActionCoin{{Asset: "MAYA.CACAO", Amount: "2"}},
			}},
		},
	}, sourceProtocolMAYA)

	if err := insertMidgardActionCache(ctx, db, protocolActionCacheKey(sourceProtocolTHOR, address), startTS, endTS, 1, false, thorAction); err != nil {
		t.Fatalf("insert thor cache: %v", err)
	}
	if err := insertMidgardActionCache(ctx, db, protocolActionCacheKey(sourceProtocolMAYA, address), startTS, endTS, 1, false, mayaAction); err != nil {
		t.Fatalf("insert maya cache: %v", err)
	}

	gotTHOR, _, foundTHOR, err := lookupMidgardActionCache(ctx, db, protocolActionCacheKey(sourceProtocolTHOR, address), startTS, endTS, 1)
	if err != nil {
		t.Fatalf("lookup thor cache: %v", err)
	}
	if !foundTHOR || len(gotTHOR) != 1 || cleanTxID(gotTHOR[0].In[0].TxID) != "THOR-TX" {
		t.Fatalf("expected thor-scoped cache hit, got %#v found=%v", gotTHOR, foundTHOR)
	}

	gotMAYA, _, foundMAYA, err := lookupMidgardActionCache(ctx, db, protocolActionCacheKey(sourceProtocolMAYA, address), startTS, endTS, 1)
	if err != nil {
		t.Fatalf("lookup maya cache: %v", err)
	}
	if !foundMAYA || len(gotMAYA) != 1 || cleanTxID(gotMAYA[0].In[0].TxID) != "MAYA-TX" {
		t.Fatalf("expected maya-scoped cache hit, got %#v found=%v", gotMAYA, foundMAYA)
	}
}

func TestFetchMidgardActionsForAddressMergesTHORAndMAYAHistoryForSharedSeed(t *testing.T) {
	const address = "0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3"

	thor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: annotateMidgardActions([]midgardAction{
				testTHORSendAction("1700000000000000000", "10", "THOR-TX", address, "thor1recipient000000000000000000000000000000", "1"),
			}, sourceProtocolTHOR),
		})
	}))
	defer thor.Close()

	maya := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(midgardActionsResponse{
			Actions: []midgardAction{{
				Date:   "1700000001000000000",
				Height: "11",
				Type:   "send",
				Status: "success",
				In: []midgardActionLeg{{
					Address: address,
					TxID:    "MAYA-TX",
					Coins:   []midgardActionCoin{{Asset: "MAYA.CACAO", Amount: "2"}},
				}},
				Out: []midgardActionLeg{{
					Address: "maya1recipient000000000000000000000000000000",
					TxID:    "MAYA-TX",
					Coins:   []midgardActionCoin{{Asset: "MAYA.CACAO", Amount: "2"}},
				}},
			}},
		})
	}))
	defer maya.Close()

	app, err := New(Config{
		DBPath:               filepath.Join(t.TempDir(), "shared-history.db"),
		StaticDir:            "internal/web/static",
		ThornodeEndpoints:    []string{thor.URL},
		MidgardEndpoints:     []string{thor.URL},
		MayaMidgardEndpoints: []string{maya.URL},
		RequestTimeout:       5 * time.Second,
		MidgardTimeout:       5 * time.Second,
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
		1,
	)
	if err != nil {
		t.Fatalf("fetch merged history: %v", err)
	}
	if truncated {
		t.Fatal("expected merged THOR+MAYA history to remain non-truncated")
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %#v", actions)
	}

	gotProtocols := []string{
		sourceProtocolFromAction(actions[0]),
		sourceProtocolFromAction(actions[1]),
	}
	if !reflect.DeepEqual(gotProtocols, []string{sourceProtocolMAYA, sourceProtocolTHOR}) {
		t.Fatalf("unexpected merged source protocols %#v", gotProtocols)
	}
}
