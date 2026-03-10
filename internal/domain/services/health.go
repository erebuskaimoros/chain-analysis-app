package services

import (
	"context"

	"chain-analysis-app/internal/app"
)

type HealthService struct {
	legacy *app.App
}

func (s *HealthService) Get(_ context.Context) app.HealthSnapshot {
	return s.legacy.HealthSnapshot()
}
