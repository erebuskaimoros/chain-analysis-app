package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBuildActorGraphAppliesServiceTimeout(t *testing.T) {
	const seed = "thor1servicetimeoutseed000000000000000000000000"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/actions":
			<-r.Context().Done()
			http.Error(w, r.Context().Err().Error(), http.StatusGatewayTimeout)
		case "/thorchain/inbound_addresses":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/thorchain/nodes":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/pools":
			_ = json.NewEncoder(w).Encode([]MidgardPool{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := openTestDB(t)
	defer db.Close()

	actor, err := upsertActor(context.Background(), db, 0, ActorUpsertRequest{
		Name:  "Timeout Seed",
		Color: "#123456",
		Addresses: []ActorAddressInput{
			{Address: seed, ChainHint: "THOR", Label: "Seed"},
		},
	})
	if err != nil {
		t.Fatalf("upsert actor: %v", err)
	}

	app := &App{
		cfg: Config{
			RequestTimeout:        50 * time.Millisecond,
			MidgardTimeout:        time.Second,
			LastRunLogPath:        "data/logs/test-last-run.log",
			BuildCommit:           "test",
			BuildVersion:          "test",
			BuildTime:             "test",
			LegacyActionEndpoints: nil,
		},
		db:              db,
		thor:            NewThorClient([]string{server.URL}, 500*time.Millisecond),
		mid:             NewThorClient([]string{server.URL}, 500*time.Millisecond),
		httpClient:      server.Client(),
		trackerHealth:   newTrackerHealthStore(),
		trackerFeatures: newTrackerFeatureStore(),
	}

	started := time.Now()
	resp, err := app.BuildActorGraph(context.Background(), ActorTrackerRequest{
		ActorIDs:  []int64{actor.ID},
		StartTime: time.Unix(0, 0).UTC().Format(time.RFC3339),
		EndTime:   time.Unix(1_800_000_000, 0).UTC().Format(time.RFC3339),
		MaxHops:   1,
	})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("build actor graph: %v", err)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("expected service timeout to stop build promptly, took %s", elapsed)
	}
	if resp.Query.CoverageSatisfied {
		t.Fatalf("expected timed-out coverage to be marked incomplete, got query %#v", resp.Query)
	}
	if len(resp.Warnings) == 0 {
		t.Fatalf("expected timeout-limited build to return warnings, got %#v", resp)
	}
}
