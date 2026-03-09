package app

import (
	"testing"
	"time"
)

func TestTrackerThrottlePolicy(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		chain          string
		wantConc       int
		wantMinSpacing time.Duration
	}{
		{
			name:           "nodereal bsc",
			provider:       "nodereal",
			chain:          "BSC",
			wantConc:       1,
			wantMinSpacing: 350 * time.Millisecond,
		},
		{
			name:           "trongrid tron",
			provider:       "trongrid",
			chain:          "TRON",
			wantConc:       1,
			wantMinSpacing: 1100 * time.Millisecond,
		},
		{
			name:           "etherscan throttled",
			provider:       "etherscan",
			chain:          "ETH",
			wantConc:       1,
			wantMinSpacing: 250 * time.Millisecond,
		},
		{
			name:           "unthrottled default",
			provider:       "utxo",
			chain:          "BTC",
			wantConc:       0,
			wantMinSpacing: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotConc, gotMinSpacing := trackerThrottlePolicy(tc.provider, tc.chain)
			if gotConc != tc.wantConc {
				t.Fatalf("concurrency mismatch: got %d want %d", gotConc, tc.wantConc)
			}
			if gotMinSpacing != tc.wantMinSpacing {
				t.Fatalf("min spacing mismatch: got %s want %s", gotMinSpacing, tc.wantMinSpacing)
			}
		})
	}
}
