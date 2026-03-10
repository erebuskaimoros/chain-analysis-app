package dto

import "chain-analysis-app/internal/app"

type HealthResponse = app.HealthSnapshot
type ActorResponse = app.Actor
type ActorListResponse struct {
	Actors []app.Actor `json:"actors"`
}

type AnnotationListResponse struct {
	Annotations []app.AddressAnnotation `json:"annotations"`
}

type BlocklistResponse struct {
	Addresses []app.BlocklistedAddress `json:"addresses"`
}

type ActionLookupResponse = app.ActionLookupResult

type ActorGraphRequest = app.ActorTrackerRequest
type ActorGraphExpandRequest = app.ActorTrackerExpandRequest
type AddressExplorerRequest = app.AddressExplorerRequest

type ActorGraphResponse struct {
	Query             app.ActorTrackerQuery  `json:"query"`
	Actors            []app.Actor            `json:"actors"`
	Stats             map[string]any         `json:"stats"`
	Warnings          []string               `json:"warnings"`
	Nodes             []app.FlowNode         `json:"nodes"`
	Edges             []app.FlowEdge         `json:"edges"`
	SupportingActions []app.SupportingAction `json:"supporting_actions"`
}

type LiveHoldingsRefreshRequest struct {
	Nodes []app.FlowNode `json:"nodes"`
}

type LiveHoldingsRefreshResponse struct {
	Nodes       []app.FlowNode `json:"nodes"`
	Warnings    []string       `json:"warnings"`
	RefreshedAt string         `json:"refreshed_at"`
}

type AddressExplorerResponse = app.AddressExplorerResponse

type ActorGraphRunsResponse struct {
	Runs []app.GraphRun `json:"runs"`
}

type AddressExplorerRunsResponse struct {
	Runs []app.AddressExplorerRun `json:"runs"`
}

type AddressAnnotationUpsertRequest struct {
	Address string `json:"address"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
}

type BlocklistUpsertRequest struct {
	Address string `json:"address"`
	Reason  string `json:"reason"`
}
