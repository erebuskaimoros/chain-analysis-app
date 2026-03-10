package services

import (
	"context"

	"chain-analysis-app/internal/app"
)

type ActorGraphService struct {
	legacy *app.App
}

func (s *ActorGraphService) Build(ctx context.Context, req app.ActorTrackerRequest) (app.ActorTrackerResponse, error) {
	return s.legacy.BuildActorGraph(ctx, req)
}

func (s *ActorGraphService) Expand(ctx context.Context, req app.ActorTrackerExpandRequest) (app.ActorTrackerResponse, error) {
	return s.legacy.ExpandActorGraph(ctx, req)
}

func (s *ActorGraphService) RefreshLiveHoldings(ctx context.Context, nodes []app.FlowNode) ([]string, error) {
	return s.legacy.RefreshLiveHoldings(ctx, nodes)
}

func (s *ActorGraphService) LookupAction(ctx context.Context, txID string) (app.ActionLookupResult, error) {
	return s.legacy.LookupActionByTxID(ctx, txID)
}

type AddressExplorerService struct {
	legacy *app.App
}

func (s *AddressExplorerService) Build(ctx context.Context, req app.AddressExplorerRequest) (app.AddressExplorerResponse, error) {
	return s.legacy.BuildAddressExplorer(ctx, req)
}

type RunService struct {
	legacy *app.App
}

func (s *RunService) CreateActorGraphRun(ctx context.Context, req app.ActorTrackerRequest, summary string, nodeCount, edgeCount int) (int64, error) {
	return s.legacy.CreateActorGraphRun(ctx, req, summary, nodeCount, edgeCount)
}

func (s *RunService) ListActorGraphRuns(ctx context.Context) ([]app.GraphRun, error) {
	return s.legacy.ListActorGraphRuns(ctx)
}

func (s *RunService) DeleteActorGraphRun(ctx context.Context, id int64) error {
	return s.legacy.DeleteActorGraphRun(ctx, id)
}

func (s *RunService) CreateAddressExplorerRun(ctx context.Context, req app.AddressExplorerRequest, summary string, nodeCount, edgeCount int) (int64, error) {
	return s.legacy.CreateAddressExplorerRun(ctx, req, summary, nodeCount, edgeCount)
}

func (s *RunService) ListAddressExplorerRuns(ctx context.Context) ([]app.AddressExplorerRun, error) {
	return s.legacy.ListAddressExplorerRuns(ctx)
}

func (s *RunService) DeleteAddressExplorerRun(ctx context.Context, id int64) error {
	return s.legacy.DeleteAddressExplorerRun(ctx, id)
}
