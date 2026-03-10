package app

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

type HealthSnapshot struct {
	OK                bool                `json:"ok"`
	Time              time.Time           `json:"time"`
	Build             BuildInfo           `json:"build"`
	ThornodeSources   []string            `json:"thornode_sources"`
	MidgardSources    []string            `json:"midgard_sources"`
	TrackerProviders  map[string]string   `json:"tracker_providers"`
	TrackerOverrides  map[string]string   `json:"tracker_overrides"`
	TrackerCandidates map[string][]string `json:"tracker_candidates"`
	TrackerHealth     map[string]any      `json:"tracker_health"`
	TrackerSources    map[string]any      `json:"tracker_sources"`
}

type ActionLookupResult struct {
	TxID    string          `json:"tx_id"`
	Actions []MidgardAction `json:"actions"`
}

func (a *App) ConfigSnapshot() Config {
	if a == nil {
		return Config{}
	}
	return a.cfg
}

func (a *App) HealthSnapshot() HealthSnapshot {
	if a == nil {
		return HealthSnapshot{
			OK:   false,
			Time: time.Now().UTC(),
		}
	}
	return HealthSnapshot{
		OK:   true,
		Time: time.Now().UTC(),
		Build: BuildInfo{
			Version:   a.cfg.BuildVersion,
			Commit:    a.cfg.BuildCommit,
			BuildTime: a.cfg.BuildTime,
		},
		ThornodeSources:   append([]string{}, a.cfg.ThornodeEndpoints...),
		MidgardSources:    append([]string{}, a.cfg.MidgardEndpoints...),
		TrackerProviders:  copyStringMap(a.cfg.ChainTrackerProviders),
		TrackerOverrides:  copyStringMap(a.cfg.ChainTrackerOverrides),
		TrackerCandidates: copyStringSliceMap(a.cfg.ChainTrackerCandidates),
		TrackerHealth:     a.trackerHealth.snapshot(),
		TrackerSources: map[string]any{
			"utxo":                  copyStringMap(a.cfg.UtxoTrackerURLs),
			"utxo_expanded":         expandChainURLMap(a.cfg.UtxoTrackerURLs),
			"cosmos":                copyStringMap(a.cfg.CosmosTrackerURLs),
			"cosmos_expanded":       expandChainURLMap(a.cfg.CosmosTrackerURLs),
			"etherscan":             a.cfg.EtherscanAPIURL,
			"etherscan_expanded":    a.cfg.etherscanAPIURLs(),
			"blockscout_urls":       copyStringMap(a.cfg.BlockscoutAPIURLs),
			"blockscout_expanded":   expandChainURLMap(a.cfg.BlockscoutAPIURLs),
			"avacloud_base":         a.cfg.AvaCloudBaseURL,
			"avacloud_expanded":     a.cfg.avaCloudBaseURLs(),
			"nodereal_bsc":          a.cfg.NodeRealBSCURL,
			"nodereal_bsc_expanded": a.cfg.nodeRealBSCURLs(),
			"solana_rpc":            a.cfg.SolanaRPCURL,
			"solana_rpc_expanded":   a.cfg.solanaRPCURLs(),
			"trongrid":              a.cfg.TronGridURL,
			"trongrid_expanded":     a.cfg.tronGridURLs(),
			"xrp_rpc":               a.cfg.XRPRPCURL,
			"xrp_rpc_expanded":      a.cfg.xrplRPCURLs(),
		},
	}
}

func (a *App) LookupActionByTxID(ctx context.Context, txID string) (ActionLookupResult, error) {
	if a == nil {
		return ActionLookupResult{}, fmt.Errorf("app is required")
	}
	txID = strings.ToUpper(strings.TrimSpace(txID))
	if txID == "" {
		return ActionLookupResult{}, fmt.Errorf("txid is required")
	}

	ctx, cancel := context.WithTimeout(ctx, a.cfg.RequestTimeout)
	defer cancel()

	var response midgardActionsResponse
	path := "/actions?txid=" + txID
	if err := a.mid.GetJSON(ctx, path, &response); err != nil {
		return ActionLookupResult{}, err
	}

	return ActionLookupResult{
		TxID:    txID,
		Actions: canonicalizeMidgardLookupActions(response.Actions),
	}, nil
}

func (a *App) ListActors(ctx context.Context) ([]Actor, error) {
	return listActors(ctx, a.db)
}

func (a *App) UpsertActor(ctx context.Context, actorID int64, req ActorUpsertRequest) (Actor, error) {
	return upsertActor(ctx, a.db, actorID, req)
}

func (a *App) DeleteActor(ctx context.Context, actorID int64) error {
	return deleteActor(ctx, a.db, actorID)
}

func (a *App) ListAddressAnnotations(ctx context.Context) ([]AddressAnnotation, error) {
	return listAddressAnnotations(ctx, a.db)
}

func (a *App) UpsertAddressAnnotation(ctx context.Context, address, kind, value string) error {
	return upsertAddressAnnotation(ctx, a.db, address, kind, value)
}

func (a *App) DeleteAddressAnnotation(ctx context.Context, address, kind string) error {
	return deleteAddressAnnotation(ctx, a.db, address, kind)
}

func (a *App) ListBlocklistedAddresses(ctx context.Context) ([]BlocklistedAddress, error) {
	return listBlocklistedAddresses(ctx, a.db)
}

func (a *App) AddToBlocklist(ctx context.Context, address, reason string) error {
	return addToBlocklist(ctx, a.db, address, reason)
}

func (a *App) RemoveFromBlocklist(ctx context.Context, address string) error {
	return removeFromBlocklist(ctx, a.db, address)
}

func (a *App) BuildActorGraph(ctx context.Context, req ActorTrackerRequest) (ActorTrackerResponse, error) {
	return a.buildActorTracker(ctx, req)
}

func (a *App) ExpandActorGraph(ctx context.Context, req ActorTrackerExpandRequest) (ActorTrackerResponse, error) {
	return a.expandActorTrackerOneHop(ctx, req)
}

func (a *App) RefreshLiveHoldings(ctx context.Context, nodes []FlowNode) ([]string, error) {
	return a.refreshActorTrackerLiveHoldings(ctx, nodes)
}

func (a *App) BuildAddressExplorer(ctx context.Context, req AddressExplorerRequest) (AddressExplorerResponse, error) {
	return a.buildAddressExplorer(ctx, req)
}

func (a *App) CreateActorGraphRun(ctx context.Context, req ActorTrackerRequest, actorNames string, nodeCount, edgeCount int) (int64, error) {
	return insertGraphRun(ctx, a.db, req, actorNames, nodeCount, edgeCount)
}

func (a *App) ListActorGraphRuns(ctx context.Context) ([]GraphRun, error) {
	return listGraphRuns(ctx, a.db)
}

func (a *App) DeleteActorGraphRun(ctx context.Context, id int64) error {
	return deleteGraphRun(ctx, a.db, id)
}

func (a *App) CreateAddressExplorerRun(ctx context.Context, req AddressExplorerRequest, summary string, nodeCount, edgeCount int) (int64, error) {
	return insertAddressExplorerRun(ctx, a.db, req, summary, nodeCount, edgeCount)
}

func (a *App) ListAddressExplorerRuns(ctx context.Context) ([]AddressExplorerRun, error) {
	return listAddressExplorerRuns(ctx, a.db)
}

func (a *App) DeleteAddressExplorerRun(ctx context.Context, id int64) error {
	return deleteAddressExplorerRun(ctx, a.db, id)
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyStringSliceMap(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string{}, values...)
	}
	return out
}
