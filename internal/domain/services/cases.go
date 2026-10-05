package services

import (
	"context"
	"time"

	"chain-analysis-app/internal/app"
)

// CaseService exposes investigation cases and their exports.
type CaseService struct {
	legacy *app.App
}

func (s *CaseService) List(ctx context.Context) ([]app.Case, error) { return s.legacy.ListCases(ctx) }

func (s *CaseService) Get(ctx context.Context, id int64) (app.Case, error) {
	return s.legacy.GetCase(ctx, id)
}

func (s *CaseService) Create(ctx context.Context, req app.CaseUpsertRequest) (app.Case, error) {
	return s.legacy.CreateCase(ctx, req)
}

func (s *CaseService) Update(ctx context.Context, id int64, req app.CaseUpsertRequest) (app.Case, error) {
	return s.legacy.UpdateCase(ctx, id, req)
}

func (s *CaseService) Delete(ctx context.Context, id int64) error {
	return s.legacy.DeleteCase(ctx, id)
}

func (s *CaseService) AddItem(ctx context.Context, id int64, req app.CaseItemRequest) (app.CaseItem, error) {
	return s.legacy.AddCaseItem(ctx, id, req)
}

func (s *CaseService) DeleteItem(ctx context.Context, id, itemID int64) error {
	return s.legacy.DeleteCaseItem(ctx, id, itemID)
}

func (s *CaseService) ExportMarkdown(ctx context.Context, id int64) (string, error) {
	return s.legacy.ExportCaseMarkdown(ctx, id)
}

func (s *CaseService) ExportCSV(ctx context.Context, id int64) ([]byte, error) {
	return s.legacy.ExportCaseCSV(ctx, id)
}

// InvestigationService exposes address profiles, label imports and ledger
// coverage, which the CLI and MCP server use.
type InvestigationService struct {
	legacy *app.App
}

func (s *InvestigationService) StartAddressProfile(req app.AddressProfileRequest) app.JobSnapshot {
	return s.legacy.StartAddressProfileJob(req)
}

func (s *InvestigationService) StartLabelsImport(source, path string) app.JobSnapshot {
	return s.legacy.StartLabelsImportJob(source, path)
}

func (s *InvestigationService) LedgerCoverage(ctx context.Context, address string, start, end time.Time) (app.LedgerCoverageReport, error) {
	return s.legacy.LedgerCoverage(ctx, address, start, end)
}
