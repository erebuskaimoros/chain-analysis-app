package services

import (
	"context"

	"chain-analysis-app/internal/app"
)

// LabelService reads address labels and their sources.
type LabelService struct {
	legacy *app.App
}

func (s *LabelService) ForAddress(ctx context.Context, address string) ([]app.AddressLabel, error) {
	return s.legacy.AddressLabels(ctx, address)
}

func (s *LabelService) Sources(ctx context.Context) ([]app.LabelSource, error) {
	return s.legacy.LabelSources(ctx)
}
