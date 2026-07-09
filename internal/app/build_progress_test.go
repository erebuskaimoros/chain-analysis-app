package app

import (
	"context"
	"testing"
)

func TestBuildProgressRegistryLifecycle(t *testing.T) {
	registry := newProgressRegistry()

	if _, ok := registry.lookup("missing"); ok {
		t.Fatalf("expected lookup miss for unknown token")
	}

	progress := registry.start("tok-1")
	if progress == nil {
		t.Fatalf("expected progress entry")
	}

	progress.set("fetching seed history", 2, 10, "thor1abc")
	snap, ok := registry.lookup("tok-1")
	if !ok {
		t.Fatalf("expected lookup hit")
	}
	if snap.Stage != "fetching seed history" || snap.Done != 2 || snap.Total != 10 || snap.Finished {
		t.Fatalf("unexpected snapshot %#v", snap)
	}

	progress.finish()
	snap, _ = registry.lookup("tok-1")
	if !snap.Finished {
		t.Fatalf("expected finished snapshot, got %#v", snap)
	}
}

func TestBuildProgressNilSafety(t *testing.T) {
	var progress *buildProgress
	progress.set("stage", 1, 2, "")
	progress.finish()
	if snap := progress.snapshot(); snap.Stage != "" {
		t.Fatalf("expected zero snapshot from nil progress, got %#v", snap)
	}

	if got := buildProgressFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil progress from bare context")
	}

	ctx := withBuildProgress(context.Background(), &buildProgress{})
	if got := buildProgressFromContext(ctx); got == nil {
		t.Fatalf("expected progress from context")
	}
}
