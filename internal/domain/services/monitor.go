package services

import (
	"context"

	"chain-analysis-app/internal/app"
)

// MonitorService exposes actor monitoring.
type MonitorService struct {
	legacy *app.App
}

func (s *MonitorService) Monitor(ctx context.Context, actorID int64) (app.ActorMonitor, error) {
	return s.legacy.ActorMonitor(ctx, actorID)
}

func (s *MonitorService) SetWatch(ctx context.Context, actorID int64, watch bool) error {
	return s.legacy.SetActorWatch(ctx, actorID, watch)
}

func (s *MonitorService) MarkViewed(ctx context.Context, actorID int64) error {
	return s.legacy.MarkActorViewed(ctx, actorID)
}

func (s *MonitorService) StartRefresh(actorID int64) app.JobSnapshot {
	return s.legacy.StartActorRefreshJob(actorID)
}
