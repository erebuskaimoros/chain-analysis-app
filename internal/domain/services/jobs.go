package services

import "chain-analysis-app/internal/app"

// JobService starts and tracks background analysis jobs.
type JobService struct {
	legacy *app.App
}

func (s *JobService) StartActorGraphBuild(req app.ActorTrackerRequest) app.JobSnapshot {
	return s.legacy.StartActorGraphBuildJob(req)
}

func (s *JobService) StartActorGraphExpand(req app.ActorTrackerExpandRequest) app.JobSnapshot {
	return s.legacy.StartActorGraphExpandJob(req)
}

func (s *JobService) StartAddressExplorer(req app.AddressExplorerRequest) app.JobSnapshot {
	return s.legacy.StartAddressExplorerJob(req)
}

func (s *JobService) StartLiveHoldings(nodes []app.FlowNode, force bool) app.JobSnapshot {
	return s.legacy.StartLiveHoldingsJob(nodes, force)
}

func (s *JobService) Get(id string, includePartial bool) (app.JobSnapshot, bool) {
	return s.legacy.Job(id, includePartial)
}

func (s *JobService) Cancel(id string) bool {
	return s.legacy.CancelJob(id)
}
