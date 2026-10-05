package services

import (
	"context"

	"chain-analysis-app/internal/app"
)

// TraceService exposes follow-the-funds traces and their saved runs.
type TraceService struct {
	legacy *app.App
}

func (s *TraceService) Start(req app.TraceRequest) (app.JobSnapshot, error) {
	if err := s.legacy.ValidateTraceRequest(req); err != nil {
		return app.JobSnapshot{}, err
	}
	return s.legacy.StartTraceJob(req), nil
}

func (s *TraceService) List(ctx context.Context) ([]app.TraceRun, error) {
	return s.legacy.ListTraceRuns(ctx)
}

func (s *TraceService) Get(ctx context.Context, id int64) (app.TraceRun, error) {
	return s.legacy.GetTraceRun(ctx, id)
}

func (s *TraceService) Delete(ctx context.Context, id int64) error {
	return s.legacy.DeleteTraceRun(ctx, id)
}
