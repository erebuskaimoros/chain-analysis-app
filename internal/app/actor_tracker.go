package app

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultActorTrackerHops      = 4
	graphQueryLimitPerFrontier   = 1200
	graphIngestBatch             = int64(120)
	midgardActionsPageLimit      = 50
	midgardMaxPagesPerAddress    = 20
	midgardGraphPagesPerSeed     = 8
	midgardGraphPagesPerFirstHop = 8
	midgardGraphPagesPerHop      = 1
	midgardGraphMaxFetches       = 60
	midgardExpandPagesPerSeed    = 4
	actorTrackerExpandAddrCap    = 64
	midgardActionPageDelay       = 250 * time.Millisecond
	maxFrontierPerHop            = 20
	midgardConcurrentFetches     = 3
	midgard429Cooldown           = 2 * time.Second
	midgardHeightPadding         = int64(2)
	midgardRangeMergeGap         = int64(24)
	largeWindowFallbackLimit     = 45 * 24 * time.Hour
)

const asgardModuleAddress = "thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0"
const bondModuleAddress = "thor17gw75axcnr8747pkanye45pnrwk7p9c3cqncsv"

// graphExcludedAddresses are completely excluded from the graph — no nodes,
// no edges, no hop expansion. Segments involving these addresses are dropped.
var graphExcludedAddresses = map[string]bool{
	"thor1dheycdevq39qlkxs2a6wuuzyn4aqxhve4qxtxt": true, // Reserve
	asgardModuleAddress:                           true, // Asgard is swap transit; never a graph endpoint
}

// frontierBlacklist contains addresses that are too active to expand into
// further hops. They still appear as labeled nodes in the graph but are never
// used as seeds for the next hop frontier.
var frontierBlacklist = map[string]string{
	bondModuleAddress:   "Bond Module",
	asgardModuleAddress: "Asgard Module",
}

// knownAddressLabels provides display labels for well-known addresses that are
// not in the protocol directory. These addresses are still eligible for hop
// expansion unlike frontierBlacklist entries.
var knownAddressLabels = map[string]string{
	"thor1v8ppstuf6e3x0r4glqc68d5jqcs2tf38cg2q6y": "Synth Module",
	"thor17hwqt302e5f2xm4h95ma8wuggqkvfzgvsnh5z9": "Arb Bot",
	"thor1g98cy3n9mmjrpn0sxmn63lztelera37n8n67c0": "Asgard Module",
	"thor15q46zcln5qkmyt7azje3qyvlrfxzl8j2v9k6rw": "Scheduler Module",
	"0x0b354326e140bdfb605b90aff0fe2cb07d48f7a3":  "Treasury Eth Wallet",
	"TWS1onJnNTg8tJHomceqxBxTsUB1DHh7PV":          "ChangeNOW",
	// Rujira / TCY flow labels discovered from on-chain contract metadata.
	"thor1n5a08r0zvmqca39ka2tgwlkjy9ugalutk7fjpzptfppqcccnat2ska5t4g": "Rujira THORChain Swap",
	"thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8": "DCA into TCY",
	"thor1jshw3secvxhzfyza6aj530hrc73zave42zgs525n0xkc3e9d6wkqrm8j3y": "Rujira FIN TCY/BTC",
	"thor197g3d76rp4dsvfy5zz67h5fr3aj8vjmzezmfy9c8z7t9nh63wsms85amlw": "TCY Vault",
	"thor136rwqvwy3flttm9wfnc5xgnlr6mu5k8e2elgzs2hdhuwf50w3l2q0nu2qu": "CALC Manager",
	"thor1t2cnyn98xusxakgemsenn2p9n3ykd6accr2c0zg22nczh097ln7qeze20f": "CALC Scheduler",
	"thor17dxtxrne37gguxdeun4n36vqd5jmxxku5tr6gkuhhsh4lz9e8gksck4ygu": "CALC DAO",
	// Shared deployer repeated across audited/mainnet-targeted Rujira contract manifests.
	"thor1e0lmk5juawc46jwjwd0xfz587njej7ay5fh6cd": "Rujira Contract Deployer",
}

// knownCalcRepresentativePayouts preserves stable Treasury destinations for
// long-lived CALC strategies whose live Midgard process rows only expose
// msg.execute payloads.
var knownCalcRepresentativePayouts = map[string]string{
	"thor1f2cgnj7elhxk9f2uq8dufl6vm96rhzz3ve0t4x9z099untck2xfqj9qpe8": "thor10qh5272ktq4wes8ex343ky9rsuehcypddjh08k",
}

var actorTrackerEventTypes = []string{
	"add_liquidity",
	"withdraw",
	"withdraw_liquidity",
	"swap",
	"streaming_swap",
	"refund",
	"transfer",
	"outbound",
	"rune_pool_deposit",
	"rune_pool_withdraw",
	"trade_account_deposit",
	"trade_account_withdraw",
	"secured_asset_deposit",
	"secured_asset_withdraw",
	"bond",
	"rebond",
	"unbond",
	"leave",
	"slash",
	"rewards",
}

type protocolDirectory struct {
	AddressKinds    map[string]protocolAddress
	SupportedChains map[string]struct{}
}

type protocolAddress struct {
	Kind        string
	Chain       string
	Label       string
	NodeAddress string
}

type priceBook struct {
	RuneUSD       float64
	AssetUSD      map[string]float64
	PoolAssets    map[string]struct{}
	PoolSnapshots map[string]MidgardPool
	HasPoolData   bool
}

type graphBuilder struct {
	ownerMap             map[string][]int64
	actorsByID           map[int64]Actor
	addressRefOverrides  map[string]flowRef
	protocols            protocolDirectory
	prices               priceBook
	bondMemoNodeByTx     map[string]string
	calcPayoutByContract map[string]string
	allowedFlowTypes     map[string]bool
	minUSD               float64
	nodes                map[string]*FlowNode
	edges                map[string]*FlowEdge
	actions              map[string]*SupportingAction
	warnings             []string
	seenCanonicalKey     map[string]struct{}
	swapEmitted          int
	swapDeduped          int
	swapSuppressed       int
	swapUnresolved       int
	refundActionDrop     int
	refundXferDrop       int
	feeActionDrop        int
	feeXferDrop          int
	contractSubDrop      int
}

type queueItem struct {
	Address   string
	Chain     string
	Hop       int
	BaseDepth int
}

type frontierCandidate struct {
	address   string
	chain     string
	totalUSD  float64
	baseDepth int
}

type frontierAddress struct {
	Address string
	Chain   string
}

type flowRef struct {
	ID        string
	Key       string
	Kind      string
	Label     string
	Chain     string
	Stage     string
	Depth     int
	ActorIDs  []int64
	Shared    bool
	Collapsed bool
	Address   string
	Metrics   map[string]any
}

type projectedSegment struct {
	Source           flowRef
	Target           flowRef
	ActionClass      string
	ActionKey        string
	ActionLabel      string
	ActionDomain     string
	ValidatorAddress string
	ValidatorLabel   string
	SwapInAsset      string
	SwapInAmountRaw  string
	SwapOutAsset     string
	SwapOutAmountRaw string
	ContractType     string
	ContractProtocol string
	Asset            string
	AssetKind        string
	TokenStandard    string
	TokenAddress     string
	TokenSymbol      string
	TokenName        string
	TokenDecimals    int
	AmountRaw        string
	USDSpot          float64
	TxID             string
	Height           int64
	Time             time.Time
	Confidence       float64
	ActorIDs         []int64
	CanonicalKey     string
}

type midgardActionsResponse struct {
	Actions []midgardAction    `json:"actions"`
	Meta    midgardActionsMeta `json:"meta"`
}

type midgardActionsMeta struct {
	NextPageToken string `json:"nextPageToken"`
	PrevPageToken string `json:"prevPageToken"`
}

type midgardAction struct {
	Date     string                `json:"date"`
	Height   string                `json:"height"`
	Type     string                `json:"type"`
	Status   string                `json:"status"`
	In       []midgardActionLeg    `json:"in"`
	Out      []midgardActionLeg    `json:"out"`
	Pools    []string              `json:"pools"`
	Metadata midgardActionMetadata `json:"metadata"`
}

type midgardActionLeg struct {
	Address string              `json:"address"`
	TxID    string              `json:"txID"`
	Coins   []midgardActionCoin `json:"coins"`
}

type midgardActionCoin struct {
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
}

type midgardActionMetadata struct {
	Contract *midgardContractMetadata `json:"contract"`
	Bond     *midgardBondMetadata     `json:"bond"`
	Rebond   *midgardRebondMetadata   `json:"rebond"`
}

type midgardBondMetadata struct {
	Fee         string `json:"fee"`
	Memo        string `json:"memo"`
	NodeAddress string `json:"nodeAddress"`
	Provider    string `json:"provider"`
}

type midgardRebondMetadata struct {
	Memo           string `json:"memo"`
	NodeAddress    string `json:"nodeAddress"`
	NewBondAddress string `json:"newBondAddress"`
}

type midgardContractMetadata struct {
	ContractType string         `json:"contractType"`
	Funds        string         `json:"funds"`
	Msg          map[string]any `json:"msg"`
}

type heightRange struct {
	Start int64
	End   int64
}

func (a *App) buildActorTracker(ctx context.Context, req ActorTrackerRequest) (ActorTrackerResponse, error) {
	started := time.Now()
	query, err := normalizeActorTrackerRequest(req)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	logInfo(ctx, "actor_tracker_started", map[string]any{
		"actor_ids":  query.ActorIDs,
		"start_time": query.StartTime.Format(time.RFC3339),
		"end_time":   query.EndTime.Format(time.RFC3339),
		"max_hops":   query.MaxHops,
	})

	actors, err := getActorsByIDs(ctx, a.db, query.ActorIDs)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	if len(actors) == 0 {
		return ActorTrackerResponse{}, fmt.Errorf("at least one actor is required")
	}

	ownerMap, actorsByID, seedAddresses := actorOwnerMap(actors)
	if len(seedAddresses) == 0 {
		return ActorTrackerResponse{}, fmt.Errorf("selected actors do not have any addresses")
	}

	coverageAddresses := make([]string, 0, len(seedAddresses))
	for _, seed := range seedAddresses {
		coverageAddresses = append(coverageAddresses, encodeFrontierAddress(seed))
	}
	blocksScanned, coverageSatisfied, coverageWarnings, prefilterActions, prefilterTruncated, err := a.ensureActorTrackerCoverage(ctx, coverageAddresses, query.StartTime, query.EndTime)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	query.BlocksScanned = blocksScanned
	query.CoverageSatisfied = coverageSatisfied
	logInfo(ctx, "actor_tracker_coverage_ready", map[string]any{
		"blocks_scanned":       blocksScanned,
		"coverage_satisfied":   coverageSatisfied,
		"coverage_warnings":    len(coverageWarnings),
		"elapsed_ms":           time.Since(started).Milliseconds(),
		"requested_time_start": query.StartTime.Format(time.RFC3339),
		"requested_time_end":   query.EndTime.Format(time.RFC3339),
	})

	protocols, err := a.loadProtocolDirectory(ctx)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	prices, priceErr := a.buildPriceBook(ctx)
	logInfo(ctx, "actor_tracker_metadata_ready", map[string]any{
		"price_book_available": priceErr == nil,
		"elapsed_ms":           time.Since(started).Milliseconds(),
	})

	builder := &graphBuilder{
		ownerMap:             ownerMap,
		actorsByID:           actorsByID,
		protocols:            protocols,
		prices:               prices,
		bondMemoNodeByTx:     map[string]string{},
		calcPayoutByContract: map[string]string{},
		allowedFlowTypes:     flowTypeSet(query.FlowTypes),
		minUSD:               query.MinUSD,
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
		warnings:             append([]string{}, coverageWarnings...),
		seenCanonicalKey:     map[string]struct{}{},
	}
	if priceErr != nil {
		builder.warnings = append(builder.warnings, "spot USD normalization unavailable; falling back to asset-native values")
	}

	for _, actor := range actors {
		ref := flowRef{
			ID:       fmt.Sprintf("actor:%d", actor.ID),
			Key:      fmt.Sprintf("actor:%d", actor.ID),
			Kind:     "actor",
			Label:    actor.Name,
			Stage:    "actor",
			Depth:    0,
			ActorIDs: []int64{actor.ID},
			Metrics: map[string]any{
				"color": actor.Color,
				"notes": actor.Notes,
			},
		}
		builder.ensureNode(ref)
		for _, addr := range actor.Addresses {
			addrRef := builder.makeAddressRef(addr.Address, addr.ChainHint, 1)
			if addr.Label != "" {
				addrRef.Label = addr.Label
			}
			builder.ensureNode(addrRef)
			builder.addProjectedSegment(projectedSegment{
				Source:      ref,
				Target:      addrRef,
				ActionClass: "ownership",
				AmountRaw:   "0",
				Time:        time.Now().UTC(),
				Confidence:  1,
				ActorIDs:    []int64{actor.ID},
			})
		}
	}

	queue := make([]queueItem, 0, len(seedAddresses))
	queued := map[string]int{}
	for _, seed := range seedAddresses {
		norm := normalizeFrontierAddress(encodeFrontierAddress(seed))
		if norm.Address == "" {
			builder.warnings = append(builder.warnings, fmt.Sprintf("skipped invalid seed address %s", shortAddress(seed.Address)))
			continue
		}
		queue = append(queue, queueItem{Address: norm.Address, Chain: norm.Chain, Hop: 0, BaseDepth: 1})
		queued[frontierKey(norm.Chain, norm.Address)] = 0
	}

	seenMidgardActions := map[string]struct{}{}
	seenExternalTransfers := map[string]struct{}{}
	midgardActionCache := map[string][]midgardAction{}
	midgardActionTruncated := map[string]bool{}
	if prefilterActions != nil {
		for k, v := range prefilterActions {
			midgardActionCache[k] = v
		}
	}
	if prefilterTruncated != nil {
		for k, v := range prefilterTruncated {
			midgardActionTruncated[k] = v
		}
	}
	midgardTruncWarned := map[string]struct{}{}
	externalWarned := map[string]struct{}{}
	midgardRateLimited := false
	midgardFetchCount := 0
	visitedFrontier := map[string]int{}
	midgardSwapTxIDs := map[string]struct{}{}
	refundTxIDs := map[string]struct{}{}
	liquidityFeeTxIDs := map[string]struct{}{}
	calcStrategyTxIDs := map[string]struct{}{}
	calcStrategyProcessTxIDs := map[string]struct{}{}
	for len(queue) > 0 {
		// Drain current hop level into a wave for concurrent Midgard prefetch.
		currentHop := queue[0].Hop
		var wave []queueItem
		var remaining []queueItem
		for _, item := range queue {
			if item.Hop == currentHop {
				wave = append(wave, item)
			} else {
				remaining = append(remaining, item)
			}
		}
		queue = remaining

		// Prefetch Midgard actions for the entire wave concurrently.
		if !midgardRateLimited && midgardFetchCount < midgardGraphMaxFetches {
			a.prefetchMidgardBatch(ctx, wave, query.StartTime, query.EndTime, currentHop,
				midgardActionCache, midgardActionTruncated, &midgardFetchCount, &midgardRateLimited,
				midgardGraphMaxFetches, builder)
		}
		for _, item := range wave {
			mergeStringSet(midgardSwapTxIDs, collectMidgardSwapTxIDs(midgardActionCache[item.Address]))
		}

		// Collect next-hop candidates with cumulative USD values so we can
		// prioritise high-value connections and cap the frontier size.
		nextCandidates := map[string]*frontierCandidate{}

		collectCandidate := func(addr frontierAddress, usd float64, baseDepth int) {
			norm := normalizeFrontierAddress(encodeFrontierAddress(addr))
			if norm.Address == "" {
				return
			}
			key := frontierKey(norm.Chain, norm.Address)
			if prevHop, ok := queued[key]; ok && prevHop <= currentHop+1 {
				return
			}
			if c, exists := nextCandidates[key]; exists {
				c.totalUSD += usd
			} else {
				nextCandidates[key] = &frontierCandidate{
					address:   norm.Address,
					chain:     norm.Chain,
					totalUSD:  usd,
					baseDepth: baseDepth,
				}
			}
		}

		for _, item := range wave {
			itemKey := frontierKey(item.Chain, item.Address)
			if prev, ok := visitedFrontier[itemKey]; ok && prev <= item.Hop {
				continue
			}
			visitedFrontier[itemKey] = item.Hop

			actions := midgardActionCache[item.Address]
			truncated := midgardActionTruncated[item.Address]
			if truncated {
				if _, warned := midgardTruncWarned[item.Address]; !warned {
					builder.warnings = append(builder.warnings, fmt.Sprintf("midgard action flow truncated for %s after %d pages", shortAddress(item.Address), midgardGraphPagesForHop(item.Hop)))
					midgardTruncWarned[item.Address] = struct{}{}
				}
			}
			mergeStringSet(refundTxIDs, collectMidgardRefundTxIDs(actions))
			mergeStringSet(liquidityFeeTxIDs, collectMidgardLiquidityFeeTxIDs(actions))
			mergeStringSet(calcStrategyTxIDs, collectCalcStrategyTxIDs(actions))
			mergeStringSet(calcStrategyProcessTxIDs, collectCalcStrategyProcessTxIDs(actions))
			builder.recordCalcRepresentativePayouts(actions)

			externalTransfers, externalTruncated, externalWarning, extErr := a.fetchExternalTransfersForAddress(ctx, item.Chain, item.Address, query.StartTime, query.EndTime, max(1, midgardGraphPagesForHop(item.Hop)))
			builder.warnings = append(builder.warnings, a.hydrateBondMemoNodeCache(ctx, actions, builder.bondMemoNodeByTx)...)
			if externalWarning != "" {
				warnKey := firstNonEmpty(frontierKey(item.Chain, item.Address), externalWarning)
				if _, ok := externalWarned[warnKey]; !ok {
					builder.warnings = append(builder.warnings, externalWarning)
					externalWarned[warnKey] = struct{}{}
				}
			}
			if extErr != nil {
				warnKey := "fetch:" + frontierKey(item.Chain, item.Address)
				if _, ok := externalWarned[warnKey]; !ok {
					builder.warnings = append(builder.warnings, fmt.Sprintf("%s tracker fetch failed for %s", firstNonEmpty(item.Chain, "external"), shortAddress(item.Address)))
					externalWarned[warnKey] = struct{}{}
				}
				logError(ctx, "actor_tracker_external_fetch_failed", extErr, map[string]any{
					"address": item.Address,
					"chain":   item.Chain,
				})
				externalTransfers = nil
			}
			consumedExternalTransfers := map[string]struct{}{}
			// Midgard action-level movements provide address-to-address liquidity paths.
			for _, action := range actions {
				key := midgardActionKey(action)
				if key == "" {
					continue
				}
				if _, exists := seenMidgardActions[key]; exists {
					_, consumed := builder.stitchMidgardAction(action, externalTransfers)
					mergeStringSet(consumedExternalTransfers, consumed)
					continue
				}
				if skip, reason := shouldSkipMidgardActionForGraph(action, refundTxIDs, liquidityFeeTxIDs, calcStrategyTxIDs, calcStrategyProcessTxIDs); skip {
					switch reason {
					case "liquidity_fee_action", "liquidity_fee_associated":
						builder.feeActionDrop++
					case "contract_sub_execution":
						builder.contractSubDrop++
					default:
						builder.refundActionDrop++
					}
					seenMidgardActions[key] = struct{}{}
					continue
				}

				segments, nextAddresses, warnings, consumed := builder.projectMidgardActionWithExternal(action, item.BaseDepth, externalTransfers)
				builder.warnings = append(builder.warnings, warnings...)
				mergeStringSet(consumedExternalTransfers, consumed)
				if len(segments) == 0 {
					continue
				}
				seenMidgardActions[key] = struct{}{}
				for _, segment := range segments {
					builder.addProjectedSegment(segment)
				}
				if item.Hop >= query.MaxHops {
					continue
				}
				for _, next := range nextAddresses {
					segUSD := float64(0)
					for _, seg := range segments {
						if frontierKey(seg.Source.Chain, seg.Source.Address) == frontierKey(next.Chain, next.Address) ||
							frontierKey(seg.Target.Chain, seg.Target.Address) == frontierKey(next.Chain, next.Address) {
							segUSD += seg.USDSpot
						}
					}
					collectCandidate(next, segUSD, item.BaseDepth+3)
				}
			}
			if externalTruncated {
				warnKey := "truncated:" + frontierKey(item.Chain, item.Address)
				if _, ok := externalWarned[warnKey]; !ok {
					builder.warnings = append(builder.warnings, fmt.Sprintf("%s tracker flow truncated for %s", firstNonEmpty(item.Chain, "external"), shortAddress(item.Address)))
					externalWarned[warnKey] = struct{}{}
				}
			}
			for _, transfer := range externalTransfers {
				key := externalTransferKey(transfer)
				if key == "" {
					continue
				}
				if _, consumed := consumedExternalTransfers[key]; consumed {
					continue
				}
				if _, exists := seenExternalTransfers[key]; exists {
					continue
				}
				if skip, reason := shouldSkipExternalTransferForGraph(transfer, refundTxIDs, liquidityFeeTxIDs); skip {
					switch reason {
					case "liquidity_fee_associated":
						builder.feeXferDrop++
					default:
						builder.refundXferDrop++
					}
					seenExternalTransfers[key] = struct{}{}
					continue
				}
				segments, nextAddresses := builder.projectExternalTransfer(transfer, item.BaseDepth)
				if len(segments) == 0 {
					continue
				}
				seenExternalTransfers[key] = struct{}{}
				for _, segment := range segments {
					builder.addProjectedSegment(segment)
				}
				if item.Hop >= query.MaxHops {
					continue
				}
				for _, next := range nextAddresses {
					segUSD := float64(0)
					for _, seg := range segments {
						if frontierKey(seg.Source.Chain, seg.Source.Address) == frontierKey(next.Chain, next.Address) ||
							frontierKey(seg.Target.Chain, seg.Target.Address) == frontierKey(next.Chain, next.Address) {
							segUSD += seg.USDSpot
						}
					}
					collectCandidate(next, segUSD, item.BaseDepth+3)
				}
			}
		}

		// Sort candidates by total USD flow (descending) and cap frontier.
		if len(nextCandidates) > 0 {
			sorted := make([]*frontierCandidate, 0, len(nextCandidates))
			for _, c := range nextCandidates {
				sorted = append(sorted, c)
			}
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].totalUSD > sorted[j].totalUSD
			})
			if len(sorted) > maxFrontierPerHop {
				builder.warnings = append(builder.warnings, fmt.Sprintf(
					"hop %d frontier capped from %d to %d addresses (by USD flow priority)",
					currentHop+1, len(sorted), maxFrontierPerHop))
				sorted = sorted[:maxFrontierPerHop]
			}
			for _, c := range sorted {
				queued[frontierKey(c.chain, c.address)] = currentHop + 1
				queue = append(queue, queueItem{
					Address:   c.address,
					Chain:     c.chain,
					Hop:       currentHop + 1,
					BaseDepth: c.baseDepth,
				})
			}
		}
	}

	nodes := builder.nodeList()
	builder.warnings = append(builder.warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, builder.protocols)...)
	builder.applyNodeLabelsToValidatorMetadata(nodes)
	edges := builder.edgeList()
	actions := builder.actionList()

	stats := map[string]any{
		"actor_count":                        len(actors),
		"node_count":                         len(nodes),
		"edge_count":                         len(edges),
		"supporting_action_count":            len(actions),
		"coverage_satisfied":                 query.CoverageSatisfied,
		"swap_emitted":                       builder.swapEmitted,
		"swap_deduped":                       builder.swapDeduped,
		"swap_suppressed":                    builder.swapSuppressed,
		"swap_unresolved":                    builder.swapUnresolved,
		"refund_suppressed_actions":          builder.refundActionDrop,
		"refund_suppressed_transfers":        builder.refundXferDrop,
		"liquidity_fee_suppressed_actions":   builder.feeActionDrop,
		"liquidity_fee_suppressed_transfers": builder.feeXferDrop,
		"contract_sub_suppressed":            builder.contractSubDrop,
	}

	logInfo(ctx, "actor_tracker_completed", map[string]any{
		"nodes":                   len(nodes),
		"edges":                   len(edges),
		"actions":                 len(actions),
		"warnings":                len(builder.warnings),
		"elapsed_ms":              time.Since(started).Milliseconds(),
		"actor_count":             len(actors),
		"swap_emitted":            builder.swapEmitted,
		"swap_deduped":            builder.swapDeduped,
		"swap_suppressed":         builder.swapSuppressed,
		"swap_unresolved":         builder.swapUnresolved,
		"refund_actions":          builder.refundActionDrop,
		"refund_transfers":        builder.refundXferDrop,
		"liquidity_fee_actions":   builder.feeActionDrop,
		"liquidity_fee_transfers": builder.feeXferDrop,
		"contract_sub_executions": builder.contractSubDrop,
		"canonical_tracked":       len(builder.seenCanonicalKey),
	})

	return ActorTrackerResponse{
		Query:             query,
		Actors:            actors,
		Stats:             stats,
		Warnings:          uniqueStrings(builder.warnings),
		Nodes:             nodes,
		Edges:             edges,
		SupportingActions: actions,
	}, nil
}

func (a *App) expandActorTrackerOneHop(ctx context.Context, req ActorTrackerExpandRequest) (ActorTrackerResponse, error) {
	started := time.Now()
	query, err := normalizeActorTrackerRequest(ActorTrackerRequest{
		ActorIDs:         req.ActorIDs,
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		MaxHops:          1,
		FlowTypes:        req.FlowTypes,
		MinUSD:           req.MinUSD,
		CollapseExternal: req.CollapseExternal,
		DisplayMode:      req.DisplayMode,
	})
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	query.MaxHops = 1

	expandAddresses := normalizeAddressList(req.Addresses)
	if len(expandAddresses) == 0 {
		return ActorTrackerResponse{}, fmt.Errorf("at least one address is required for one-hop expansion")
	}

	var warnings []string
	if len(expandAddresses) > actorTrackerExpandAddrCap {
		warnings = append(warnings, fmt.Sprintf("address expansion capped at %d addresses", actorTrackerExpandAddrCap))
		expandAddresses = expandAddresses[:actorTrackerExpandAddrCap]
	}

	logInfo(ctx, "actor_tracker_expand_started", map[string]any{
		"actor_ids":       query.ActorIDs,
		"address_count":   len(expandAddresses),
		"start_time":      query.StartTime.Format(time.RFC3339),
		"end_time":        query.EndTime.Format(time.RFC3339),
		"flow_type_count": len(query.FlowTypes),
	})

	var actors []Actor
	if len(query.ActorIDs) > 0 {
		actors, err = getActorsByIDs(ctx, a.db, query.ActorIDs)
		if err != nil {
			return ActorTrackerResponse{}, err
		}
	}
	ownerMap, actorsByID, _ := actorOwnerMap(actors)

	coverageAddresses := make([]string, 0, len(expandAddresses))
	for _, seed := range expandAddresses {
		coverageAddresses = append(coverageAddresses, seed.Address)
	}
	blocksScanned, coverageSatisfied, coverageWarnings, prefilterActions, prefilterTruncated, err := a.ensureActorTrackerCoverage(ctx, coverageAddresses, query.StartTime, query.EndTime)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	query.BlocksScanned = blocksScanned
	query.CoverageSatisfied = coverageSatisfied

	protocols, err := a.loadProtocolDirectory(ctx)
	if err != nil {
		return ActorTrackerResponse{}, err
	}
	prices, priceErr := a.buildPriceBook(ctx)

	builder := &graphBuilder{
		ownerMap:             ownerMap,
		actorsByID:           actorsByID,
		protocols:            protocols,
		prices:               prices,
		bondMemoNodeByTx:     map[string]string{},
		calcPayoutByContract: map[string]string{},
		allowedFlowTypes:     flowTypeSet(query.FlowTypes),
		minUSD:               query.MinUSD,
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
		warnings:             append(append([]string{}, coverageWarnings...), warnings...),
		seenCanonicalKey:     map[string]struct{}{},
	}
	if priceErr != nil {
		builder.warnings = append(builder.warnings, "spot USD normalization unavailable; falling back to asset-native values")
	}

	seenMidgardActions := map[string]struct{}{}
	seenExternalTransfers := map[string]struct{}{}
	midgardSwapTxIDs := map[string]struct{}{}
	refundTxIDs := map[string]struct{}{}
	liquidityFeeTxIDs := map[string]struct{}{}
	calcStrategyTxIDs := map[string]struct{}{}
	calcStrategyProcessTxIDs := map[string]struct{}{}
	externalWarned := map[string]struct{}{}

	for _, seed := range expandAddresses {
		address := seed.Address
		var actions []midgardAction
		var truncated bool
		if cached, ok := prefilterActions[address]; ok {
			actions = cached
			truncated = prefilterTruncated[address]
		} else {
			var err2 error
			actions, truncated, err2 = a.fetchMidgardActionsForAddress(ctx, address, query.StartTime, query.EndTime, midgardExpandPagesPerSeed)
			if err2 != nil {
				builder.warnings = append(builder.warnings, fmt.Sprintf("midgard action flow fetch failed for %s", shortAddress(address)))
				logError(ctx, "actor_tracker_expand_midgard_fetch_failed", err2, map[string]any{
					"address": address,
				})
				actions = nil
				truncated = false
			}
		}
		mergeStringSet(midgardSwapTxIDs, collectMidgardSwapTxIDs(actions))
		if truncated {
			builder.warnings = append(builder.warnings, fmt.Sprintf("midgard action flow truncated for %s after %d pages", shortAddress(address), midgardExpandPagesPerSeed))
		}
		mergeStringSet(refundTxIDs, collectMidgardRefundTxIDs(actions))
		mergeStringSet(liquidityFeeTxIDs, collectMidgardLiquidityFeeTxIDs(actions))
		mergeStringSet(calcStrategyTxIDs, collectCalcStrategyTxIDs(actions))
		mergeStringSet(calcStrategyProcessTxIDs, collectCalcStrategyProcessTxIDs(actions))
		builder.recordCalcRepresentativePayouts(actions)

		externalTransfers, externalTruncated, externalWarning, extErr := a.fetchExternalTransfersForAddress(ctx, seed.Chain, address, query.StartTime, query.EndTime, max(1, midgardExpandPagesPerSeed))
		builder.warnings = append(builder.warnings, a.hydrateBondMemoNodeCache(ctx, actions, builder.bondMemoNodeByTx)...)
		if externalWarning != "" {
			warnKey := firstNonEmpty(frontierKey(seed.Chain, address), externalWarning)
			if _, ok := externalWarned[warnKey]; !ok {
				builder.warnings = append(builder.warnings, externalWarning)
				externalWarned[warnKey] = struct{}{}
			}
		}
		if extErr != nil {
			warnKey := "fetch:" + frontierKey(seed.Chain, address)
			if _, ok := externalWarned[warnKey]; !ok {
				builder.warnings = append(builder.warnings, fmt.Sprintf("%s tracker fetch failed for %s", firstNonEmpty(seed.Chain, "external"), shortAddress(address)))
				externalWarned[warnKey] = struct{}{}
			}
			logError(ctx, "actor_tracker_expand_external_fetch_failed", extErr, map[string]any{
				"address": address,
				"chain":   seed.Chain,
			})
			continue
		}
		consumedExternalTransfers := map[string]struct{}{}
		for _, action := range actions {
			key := midgardActionKey(action)
			if key == "" {
				continue
			}
			if _, exists := seenMidgardActions[key]; exists {
				_, consumed := builder.stitchMidgardAction(action, externalTransfers)
				mergeStringSet(consumedExternalTransfers, consumed)
				continue
			}
			if skip, reason := shouldSkipMidgardActionForGraph(action, refundTxIDs, liquidityFeeTxIDs, calcStrategyTxIDs, calcStrategyProcessTxIDs); skip {
				switch reason {
				case "liquidity_fee_action", "liquidity_fee_associated":
					builder.feeActionDrop++
				case "contract_sub_execution", "calc_strategy_sub_swap":
					builder.contractSubDrop++
				default:
					builder.refundActionDrop++
				}
				seenMidgardActions[key] = struct{}{}
				continue
			}

			segments, _, segmentWarnings, consumed := builder.projectMidgardActionWithExternal(action, 1, externalTransfers)
			builder.warnings = append(builder.warnings, segmentWarnings...)
			mergeStringSet(consumedExternalTransfers, consumed)
			if len(segments) == 0 {
				continue
			}
			seenMidgardActions[key] = struct{}{}
			for _, segment := range segments {
				builder.addProjectedSegment(segment)
			}
		}
		if externalTruncated {
			warnKey := "truncated:" + frontierKey(seed.Chain, address)
			if _, ok := externalWarned[warnKey]; !ok {
				builder.warnings = append(builder.warnings, fmt.Sprintf("%s tracker flow truncated for %s", firstNonEmpty(seed.Chain, "external"), shortAddress(address)))
				externalWarned[warnKey] = struct{}{}
			}
		}
		for _, transfer := range externalTransfers {
			key := externalTransferKey(transfer)
			if key == "" {
				continue
			}
			if _, consumed := consumedExternalTransfers[key]; consumed {
				continue
			}
			if _, exists := seenExternalTransfers[key]; exists {
				continue
			}
			if skip, reason := shouldSkipExternalTransferForGraph(transfer, refundTxIDs, liquidityFeeTxIDs); skip {
				switch reason {
				case "liquidity_fee_associated":
					builder.feeXferDrop++
				default:
					builder.refundXferDrop++
				}
				seenExternalTransfers[key] = struct{}{}
				continue
			}
			seenExternalTransfers[key] = struct{}{}
			segments, _ := builder.projectExternalTransfer(transfer, 1)
			for _, segment := range segments {
				builder.addProjectedSegment(segment)
			}
		}
	}

	nodes := builder.nodeList()
	builder.warnings = append(builder.warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, builder.protocols)...)
	builder.applyNodeLabelsToValidatorMetadata(nodes)
	edges := builder.edgeList()
	actions := builder.actionList()
	stats := map[string]any{
		"actor_count":                        len(actors),
		"node_count":                         len(nodes),
		"edge_count":                         len(edges),
		"supporting_action_count":            len(actions),
		"coverage_satisfied":                 query.CoverageSatisfied,
		"expanded_seed_count":                len(expandAddresses),
		"one_hop_expansion":                  true,
		"swap_emitted":                       builder.swapEmitted,
		"swap_deduped":                       builder.swapDeduped,
		"swap_suppressed":                    builder.swapSuppressed,
		"swap_unresolved":                    builder.swapUnresolved,
		"refund_suppressed_actions":          builder.refundActionDrop,
		"refund_suppressed_transfers":        builder.refundXferDrop,
		"liquidity_fee_suppressed_actions":   builder.feeActionDrop,
		"liquidity_fee_suppressed_transfers": builder.feeXferDrop,
		"contract_sub_suppressed":            builder.contractSubDrop,
	}

	logInfo(ctx, "actor_tracker_expand_completed", map[string]any{
		"address_count":           len(expandAddresses),
		"nodes":                   len(nodes),
		"edges":                   len(edges),
		"actions":                 len(actions),
		"warnings":                len(builder.warnings),
		"elapsed_ms":              time.Since(started).Milliseconds(),
		"swap_emitted":            builder.swapEmitted,
		"swap_deduped":            builder.swapDeduped,
		"swap_suppressed":         builder.swapSuppressed,
		"swap_unresolved":         builder.swapUnresolved,
		"refund_actions":          builder.refundActionDrop,
		"refund_transfers":        builder.refundXferDrop,
		"liquidity_fee_actions":   builder.feeActionDrop,
		"liquidity_fee_transfers": builder.feeXferDrop,
		"contract_sub_executions": builder.contractSubDrop,
	})

	return ActorTrackerResponse{
		Query:             query,
		Actors:            actors,
		Stats:             stats,
		Warnings:          uniqueStrings(builder.warnings),
		Nodes:             nodes,
		Edges:             edges,
		SupportingActions: actions,
	}, nil
}

func normalizeActorTrackerRequest(req ActorTrackerRequest) (ActorTrackerQuery, error) {
	now := time.Now().UTC()
	end := now
	if strings.TrimSpace(req.EndTime) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.EndTime))
		if err != nil {
			return ActorTrackerQuery{}, fmt.Errorf("invalid end_time: %w", err)
		}
		end = parsed.UTC()
	}

	start := end.Add(-7 * 24 * time.Hour)
	if strings.TrimSpace(req.StartTime) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.StartTime))
		if err != nil {
			return ActorTrackerQuery{}, fmt.Errorf("invalid start_time: %w", err)
		}
		start = parsed.UTC()
	}
	if !start.Before(end) {
		return ActorTrackerQuery{}, fmt.Errorf("start_time must be before end_time")
	}

	maxHops := req.MaxHops
	if maxHops < 1 {
		maxHops = defaultActorTrackerHops
	}
	if maxHops > 8 {
		maxHops = 8
	}

	displayMode := strings.TrimSpace(req.DisplayMode)
	if displayMode == "" {
		displayMode = "combined"
	}

	flowTypes := req.FlowTypes
	if len(flowTypes) == 0 {
		flowTypes = []string{"liquidity", "swaps", "bonds", "transfers"}
	}

	return ActorTrackerQuery{
		ActorIDs:         req.ActorIDs,
		StartTime:        start,
		EndTime:          end,
		MaxHops:          maxHops,
		FlowTypes:        flowTypes,
		MinUSD:           math.Max(0, req.MinUSD),
		CollapseExternal: req.CollapseExternal,
		DisplayMode:      displayMode,
		RequestedAt:      now,
	}, nil
}

func actorOwnerMap(actors []Actor) (map[string][]int64, map[int64]Actor, []frontierAddress) {
	owners := map[string][]int64{}
	actorsByID := map[int64]Actor{}
	var seeds []frontierAddress
	seenSeeds := map[string]struct{}{}
	for _, actor := range actors {
		actorsByID[actor.ID] = actor
		for _, addr := range actor.Addresses {
			norm := normalizeAddress(addr.Address)
			if norm == "" {
				continue
			}
			owners[norm] = appendUniqueInt64(owners[norm], actor.ID)
			if chainKey := frontierKey(addr.ChainHint, addr.Address); chainKey != "" {
				owners[chainKey] = appendUniqueInt64(owners[chainKey], actor.ID)
			}
			seed := frontierAddress{
				Address: norm,
				Chain:   normalizeChain(addr.ChainHint, addr.Address),
			}
			if key := frontierKey(seed.Chain, seed.Address); key != "" {
				if _, ok := seenSeeds[key]; ok {
					continue
				}
				seenSeeds[key] = struct{}{}
				seeds = append(seeds, seed)
			}
		}
	}
	sort.Slice(seeds, func(i, j int) bool {
		return frontierKey(seeds[i].Chain, seeds[i].Address) < frontierKey(seeds[j].Chain, seeds[j].Address)
	})
	return owners, actorsByID, seeds
}

func flowTypeSet(in []string) map[string]bool {
	out := map[string]bool{}
	for _, item := range in {
		v := strings.ToLower(strings.TrimSpace(item))
		if v == "" {
			continue
		}
		out[v] = true
	}
	return out
}

func (a *App) ensureActorTrackerCoverage(ctx context.Context, addresses []string, start, end time.Time) (int64, bool, []string, map[string][]midgardAction, map[string]bool, error) {
	_, prefilterSatisfied, prefilterWarnings, _, actionCache, truncatedCache, prefilterErr := a.ensureMidgardAddressCoverage(ctx, addresses, start, end)
	if prefilterErr != nil {
		return 0, false, nil, nil, nil, prefilterErr
	}
	return 0, prefilterSatisfied, prefilterWarnings, actionCache, truncatedCache, nil
}

func (a *App) ensureMidgardAddressCoverage(ctx context.Context, addresses []string, start, end time.Time) (int64, bool, []string, bool, map[string][]midgardAction, map[string]bool, error) {
	normalized := normalizeAddressList(addresses)
	if len(normalized) == 0 {
		return 0, false, nil, false, nil, nil, nil
	}

	logInfo(ctx, "midgard_prefilter_started", map[string]any{
		"address_count": len(normalized),
		"start_time":    start.Format(time.RFC3339),
		"end_time":      end.Format(time.RFC3339),
	})

	var warnings []string
	coverageSatisfied := true
	usableCalls := 0
	actionCache := map[string][]midgardAction{}
	truncatedCache := map[string]bool{}

	for _, seed := range normalized {
		address := seed.Address
		actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, address, start, end, midgardMaxPagesPerAddress)
		if err != nil {
			coverageSatisfied = false
			warnings = append(warnings, fmt.Sprintf("midgard fetch failed for %s", shortAddress(address)))
			logError(ctx, "midgard_prefilter_address_failed", err, map[string]any{
				"address": address,
			})
			continue
		}
		actionCache[address] = actions
		truncatedCache[address] = truncated
		usableCalls++
		if truncated {
			coverageSatisfied = false
			warnings = append(warnings, fmt.Sprintf("midgard actions truncated for %s after %d pages", shortAddress(address), midgardMaxPagesPerAddress))
		}
	}

	if usableCalls == 0 {
		logInfo(ctx, "midgard_prefilter_unavailable", map[string]any{
			"address_count": len(normalized),
		})
		return 0, false, warnings, false, nil, nil, nil
	}

	logInfo(ctx, "midgard_prefilter_completed", map[string]any{
		"address_count":      len(normalized),
		"coverage_satisfied": coverageSatisfied,
		"warnings":           len(warnings),
	})
	return 0, coverageSatisfied, warnings, true, actionCache, truncatedCache, nil
}

func (a *App) prefetchMidgardBatch(
	ctx context.Context,
	wave []queueItem,
	start, end time.Time,
	hop int,
	cache map[string][]midgardAction,
	truncCache map[string]bool,
	fetchCount *int,
	rateLimited *bool,
	budget int,
	builder *graphBuilder,
) {
	var toFetch []queueItem
	for _, item := range wave {
		if _, ok := cache[item.Address]; ok {
			continue
		}
		if *fetchCount >= budget {
			break
		}
		toFetch = append(toFetch, item)
	}
	if len(toFetch) == 0 {
		return
	}

	handleResult := func(address string, actions []midgardAction, truncated bool, err error) {
		if err != nil {
			builder.warnings = append(builder.warnings, fmt.Sprintf("midgard action flow fetch failed for %s", shortAddress(address)))
			logError(ctx, "midgard_graph_action_fetch_failed", err, map[string]any{
				"address": address,
				"hop":     hop,
			})
			if hop > 0 && isMidgardRateLimitError(err) {
				*rateLimited = true
			}
			cache[address] = nil
			truncCache[address] = false
		} else {
			cache[address] = actions
			truncCache[address] = truncated
		}
	}

	// Serialize hop>0 fetches to avoid Midgard 429 rate limiting.
	if hop > 0 {
		for _, item := range toFetch {
			if *fetchCount >= budget {
				break
			}
			*fetchCount++
			maxPages := midgardGraphPagesForHop(item.Hop)
			actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, item.Address, start, end, maxPages)
			handleResult(item.Address, actions, truncated, err)
			if *rateLimited {
				break
			}
			time.Sleep(midgardActionPageDelay)
		}
		return
	}

	type fetchResult struct {
		address   string
		actions   []midgardAction
		truncated bool
		err       error
	}

	sem := make(chan struct{}, midgardConcurrentFetches)
	results := make(chan fetchResult, len(toFetch))
	var wg sync.WaitGroup
	for _, item := range toFetch {
		if *fetchCount >= budget {
			break
		}
		*fetchCount++
		wg.Add(1)
		go func(addr string, hopLevel int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			maxPages := midgardGraphPagesForHop(hopLevel)
			actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, addr, start, end, maxPages)
			results <- fetchResult{address: addr, actions: actions, truncated: truncated, err: err}
		}(item.Address, item.Hop)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		handleResult(r.address, r.actions, r.truncated, r.err)
	}
}

func (a *App) fetchMidgardActionsForAddress(ctx context.Context, address string, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	address = seed.Address
	if maxPages < 1 {
		maxPages = 1
	}
	if maxPages > midgardMaxPagesPerAddress {
		maxPages = midgardMaxPagesPerAddress
	}

	fromTimestamp := start.Unix()
	if fromTimestamp < 0 {
		fromTimestamp = 0
	}
	endTimestamp := end.Unix()
	if endTimestamp < fromTimestamp {
		endTimestamp = fromTimestamp
	}

	// Check disk cache (exact match, then superset range match).
	if cached, truncated, found, err := lookupMidgardActionCache(ctx, a.db, address, fromTimestamp, endTimestamp, maxPages); err == nil && found {
		cached = canonicalizeMidgardLookupActions(cached)
		logInfo(ctx, "midgard_action_cache_hit", map[string]any{
			"address":   address,
			"actions":   len(cached),
			"truncated": truncated,
			"max_pages": maxPages,
		})
		return cached, truncated, nil
	}

	actions := make([]midgardAction, 0, maxPages*midgardActionsPageLimit)
	for page := 0; page < maxPages; page++ {
		params := url.Values{}
		params.Set("address", address)
		params.Set("fromTimestamp", strconv.FormatInt(fromTimestamp, 10))
		params.Set("timestamp", strconv.FormatInt(endTimestamp, 10))
		params.Set("limit", strconv.Itoa(midgardActionsPageLimit))
		params.Set("offset", strconv.Itoa(page*midgardActionsPageLimit))

		var response midgardActionsResponse
		path := "/actions?" + params.Encode()
		offset := page * midgardActionsPageLimit
		if err := a.mid.GetJSONObserved(ctx, path, &response, func(meta RequestAttemptMeta) {
			fields := map[string]any{
				"address":               address,
				"page":                  page,
				"offset":                offset,
				"limit":                 midgardActionsPageLimit,
				"path":                  meta.Path,
				"endpoint":              meta.Endpoint,
				"url":                   meta.URL,
				"attempt":               meta.Attempt,
				"status":                meta.StatusCode,
				"result":                meta.Result,
				"duration_ms":           meta.Duration.Milliseconds(),
				"will_retry":            meta.WillRetry,
				"retryable_status":      meta.RetryableStatus,
				"retry_after":           meta.RetryAfter,
				"x_ratelimit_limit":     meta.XRateLimitLimit,
				"x_ratelimit_remaining": meta.XRateLimitRemaining,
				"x_ratelimit_reset":     meta.XRateLimitReset,
				"ratelimit_limit":       meta.RateLimitLimit,
				"ratelimit_remaining":   meta.RateLimitRemaining,
				"ratelimit_reset":       meta.RateLimitReset,
				"cf_ray":                meta.CFRay,
				"cf_mitigated":          meta.CFMitigated,
			}
			if meta.Result == "success" {
				logInfo(ctx, "midgard_graph_action_call", fields)
				return
			}
			callErr := fmt.Errorf("midgard graph action result=%s", meta.Result)
			if strings.TrimSpace(meta.Error) != "" {
				callErr = fmt.Errorf("%s", strings.TrimSpace(meta.Error))
			}
			logError(ctx, "midgard_graph_action_call_failed", callErr, fields)
		}); err != nil {
			if isMidgardRateLimitError(err) {
				sleepWithContext(ctx, midgard429Cooldown)
			}
			return actions, false, err
		}

		actions = append(actions, response.Actions...)
		if len(response.Actions) < midgardActionsPageLimit {
			actions = canonicalizeMidgardLookupActions(actions)
			if err := insertMidgardActionCache(ctx, a.db, address, fromTimestamp, endTimestamp, maxPages, false, actions); err != nil {
				logError(ctx, "midgard_action_cache_write_failed", err, map[string]any{"address": address})
			}
			return actions, false, nil
		}
		if page+1 < maxPages && !sleepWithContext(ctx, midgardActionPageDelay) {
			return actions, false, ctx.Err()
		}
	}

	actions = canonicalizeMidgardLookupActions(actions)
	if err := insertMidgardActionCache(ctx, a.db, address, fromTimestamp, endTimestamp, maxPages, true, actions); err != nil {
		logError(ctx, "midgard_action_cache_write_failed", err, map[string]any{"address": address})
	}
	return actions, true, nil
}

// fetchMidgardActionsForAddressPaged is like fetchMidgardActionsForAddress but
// starts at an arbitrary page offset instead of 0. It skips the disk cache
// because paged requests are not cache-aligned. It does not enforce the
// midgardMaxPagesPerAddress cap so callers must bound pageCount themselves.
func (a *App) fetchMidgardActionsForAddressPaged(ctx context.Context, address string, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	address = seed.Address
	if pageCount < 1 {
		pageCount = 1
	}

	fromTimestamp := start.Unix()
	if fromTimestamp < 0 {
		fromTimestamp = 0
	}
	endTimestamp := end.Unix()
	if endTimestamp < fromTimestamp {
		endTimestamp = fromTimestamp
	}

	actions := make([]midgardAction, 0, pageCount*midgardActionsPageLimit)
	for i := 0; i < pageCount; i++ {
		page := startPage + i
		params := url.Values{}
		params.Set("address", address)
		params.Set("fromTimestamp", strconv.FormatInt(fromTimestamp, 10))
		params.Set("timestamp", strconv.FormatInt(endTimestamp, 10))
		params.Set("limit", strconv.Itoa(midgardActionsPageLimit))
		params.Set("offset", strconv.Itoa(page*midgardActionsPageLimit))

		var response midgardActionsResponse
		path := "/actions?" + params.Encode()
		offset := page * midgardActionsPageLimit
		if err := a.mid.GetJSONObserved(ctx, path, &response, func(meta RequestAttemptMeta) {
			fields := map[string]any{
				"address":               address,
				"page":                  page,
				"offset":                offset,
				"limit":                 midgardActionsPageLimit,
				"path":                  meta.Path,
				"endpoint":              meta.Endpoint,
				"url":                   meta.URL,
				"attempt":               meta.Attempt,
				"status":                meta.StatusCode,
				"result":                meta.Result,
				"duration_ms":           meta.Duration.Milliseconds(),
				"will_retry":            meta.WillRetry,
				"retryable_status":      meta.RetryableStatus,
				"retry_after":           meta.RetryAfter,
				"x_ratelimit_limit":     meta.XRateLimitLimit,
				"x_ratelimit_remaining": meta.XRateLimitRemaining,
				"x_ratelimit_reset":     meta.XRateLimitReset,
				"ratelimit_limit":       meta.RateLimitLimit,
				"ratelimit_remaining":   meta.RateLimitRemaining,
				"ratelimit_reset":       meta.RateLimitReset,
				"cf_ray":                meta.CFRay,
				"cf_mitigated":          meta.CFMitigated,
			}
			if meta.Result == "success" {
				logInfo(ctx, "midgard_explorer_paged_call", fields)
				return
			}
			callErr := fmt.Errorf("midgard explorer paged result=%s", meta.Result)
			if strings.TrimSpace(meta.Error) != "" {
				callErr = fmt.Errorf("%s", strings.TrimSpace(meta.Error))
			}
			logError(ctx, "midgard_explorer_paged_call_failed", callErr, fields)
		}); err != nil {
			if isMidgardRateLimitError(err) {
				sleepWithContext(ctx, midgard429Cooldown)
			}
			return actions, false, err
		}

		actions = append(actions, response.Actions...)
		if len(response.Actions) < midgardActionsPageLimit {
			return canonicalizeMidgardLookupActions(actions), false, nil
		}
		if i+1 < pageCount && !sleepWithContext(ctx, midgardActionPageDelay) {
			return actions, false, ctx.Err()
		}
	}

	return canonicalizeMidgardLookupActions(actions), true, nil
}

// probeMidgardTotalPages does a binary search to find the last page with data
// for an address. Returns the total number of pages (0-indexed last page + 1).
func (a *App) probeMidgardTotalPages(ctx context.Context, address string, start, end time.Time) (int, error) {
	// Exponential probe to find upper bound.
	probe := 10 // start at page 10 (offset 500)
	for {
		actions, _, err := a.fetchMidgardActionsForAddressPaged(ctx, address, start, end, probe, 1)
		if err != nil {
			return 0, err
		}
		if len(actions) == 0 {
			break
		}
		probe *= 2
		if probe > 20000 {
			// Safety cap at 1M actions.
			break
		}
	}

	// Binary search between probe/2 and probe.
	lo, hi := probe/2, probe
	for lo < hi {
		mid := (lo + hi) / 2
		actions, _, err := a.fetchMidgardActionsForAddressPaged(ctx, address, start, end, mid, 1)
		if err != nil {
			return 0, err
		}
		if len(actions) > 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

func filterMidgardActionsByTimeRange(actions []midgardAction, startTS, endTS int64) []midgardAction {
	out := make([]midgardAction, 0, len(actions))
	for _, a := range actions {
		ts := parseInt64(a.Date)
		if ts <= 0 {
			out = append(out, a)
			continue
		}
		sec := ts / 1_000_000_000
		if sec >= startTS && sec <= endTS {
			out = append(out, a)
		}
	}
	return out
}

func filterExternalTransfersByTimeRange(transfers []externalTransfer, startTS, endTS int64) []externalTransfer {
	out := make([]externalTransfer, 0, len(transfers))
	for _, transfer := range transfers {
		if transfer.Time.IsZero() {
			out = append(out, transfer)
			continue
		}
		sec := transfer.Time.Unix()
		if sec >= startTS && sec <= endTS {
			out = append(out, transfer)
		}
	}
	return out
}

func (a *App) hydrateBondMemoNodeCache(_ context.Context, actions []midgardAction, cache map[string]string) []string {
	if len(actions) == 0 || cache == nil {
		return nil
	}
	for _, action := range actions {
		if midgardActionClass(action) != "bonds" {
			continue
		}
		// Read node address directly from Midgard metadata.
		nodeAddress := ""
		switch {
		case action.Metadata.Rebond != nil:
			nodeAddress = normalizeAddress(action.Metadata.Rebond.NodeAddress)
			if nodeAddress == "" {
				nodeAddress = parseBondMemoNodeAddress(action.Metadata.Rebond.Memo)
			}
		case action.Metadata.Bond != nil:
			nodeAddress = normalizeAddress(action.Metadata.Bond.NodeAddress)
			if nodeAddress == "" {
				nodeAddress = parseBondMemoNodeAddress(action.Metadata.Bond.Memo)
			}
		}
		for _, txID := range midgardActionTxIDs(action) {
			txID = cleanTxID(txID)
			if txID == "" {
				continue
			}
			if _, ok := cache[txID]; ok {
				continue
			}
			cache[txID] = normalizeAddress(nodeAddress)
		}
	}
	return nil
}

func compactHeightRanges(heights []int64, padding, mergeGap int64) []heightRange {
	if len(heights) == 0 {
		return nil
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })

	makeRange := func(height int64) heightRange {
		start := height - padding
		if start < 1 {
			start = 1
		}
		end := height + padding
		return heightRange{Start: start, End: end}
	}

	current := makeRange(heights[0])
	out := make([]heightRange, 0, len(heights))
	for _, height := range heights[1:] {
		next := makeRange(height)
		if next.Start <= current.End+mergeGap {
			if next.End > current.End {
				current.End = next.End
			}
			continue
		}
		out = append(out, current)
		current = next
	}
	out = append(out, current)
	return out
}

func normalizeAddressList(addresses []string) []frontierAddress {
	seen := map[string]struct{}{}
	out := make([]frontierAddress, 0, len(addresses))
	for _, address := range addresses {
		normalized := normalizeFrontierAddress(address)
		if normalized.Address == "" {
			continue
		}
		key := frontierKey(normalized.Chain, normalized.Address)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		return frontierKey(out[i].Chain, out[i].Address) < frontierKey(out[j].Chain, out[j].Address)
	})
	return out
}

func normalizeFrontierAddress(raw string) frontierAddress {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return frontierAddress{}
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return frontierAddress{}
	}

	chainHint := ""
	if idx := strings.Index(raw, "|"); idx > 0 {
		candidateChain := strings.ToUpper(strings.TrimSpace(raw[:idx]))
		if isLikelyChainCode(candidateChain) {
			chainHint = candidateChain
			raw = strings.TrimSpace(raw[idx+1:])
		}
	}

	candidates := splitAddressCandidates(raw)
	for _, candidate := range candidates {
		norm := normalizeAddress(candidate)
		if isLikelyAddressCandidate(norm) {
			return frontierAddress{
				Address: norm,
				Chain:   normalizeChain(chainHint, candidate),
			}
		}
	}
	return frontierAddress{}
}

func isLikelyChainCode(value string) bool {
	if value == "" || len(value) > 12 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func frontierKey(chain, address string) string {
	norm := normalizeAddress(address)
	if norm == "" {
		return ""
	}
	chain = normalizeChain(chain, address)
	if chain == "" {
		return norm
	}
	return chain + "|" + norm
}

func encodeFrontierAddress(value frontierAddress) string {
	if value.Address == "" {
		return ""
	}
	if chain := normalizeChain(value.Chain, value.Address); chain != "" {
		return chain + "|" + normalizeAddress(value.Address)
	}
	return normalizeAddress(value.Address)
}

func splitAddressCandidates(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case '/', ',', ';', '|', '\\', ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return []string{raw}
	}
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	return out
}

func isLikelyAddressCandidate(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(value), "0x") && !isLikelyEVMAddress(value) {
		return false
	}
	if len(value) < 6 || len(value) > 160 {
		return false
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return false
	}
	if strings.HasSuffix(value, ":") {
		return false
	}
	if strings.ContainsAny(value, " \t\r\n?&=#%") {
		return false
	}
	if strings.Count(value, ":") > 1 {
		return false
	}
	return true
}

func (a *App) loadProtocolDirectory(_ context.Context) (protocolDirectory, error) {
	out := protocolDirectory{
		AddressKinds: map[string]protocolAddress{},
		SupportedChains: map[string]struct{}{
			"BTC": {}, "ETH": {}, "LTC": {}, "BCH": {}, "DOGE": {},
			"AVAX": {}, "BSC": {}, "GAIA": {}, "BASE": {},
			"SOL": {}, "TRON": {}, "XRP": {}, "THOR": {},
		},
	}

	// Populate from hardcoded known address maps.
	for addr, label := range knownAddressLabels {
		out.AddressKinds[normalizeAddress(addr)] = protocolAddress{
			Kind:  "known",
			Label: label,
		}
	}
	for addr, label := range frontierBlacklist {
		out.AddressKinds[normalizeAddress(addr)] = protocolAddress{
			Kind:  "module",
			Label: label,
		}
	}
	for addr := range graphExcludedAddresses {
		if _, exists := out.AddressKinds[normalizeAddress(addr)]; !exists {
			out.AddressKinds[normalizeAddress(addr)] = protocolAddress{
				Kind:  "excluded",
				Label: "Protocol",
			}
		}
	}

	return out, nil
}

func (a *App) buildPriceBook(ctx context.Context) (priceBook, error) {
	pools, err := a.fetchPools(ctx)
	if err != nil {
		return priceBook{}, err
	}

	book := priceBook{
		AssetUSD:      map[string]float64{},
		PoolAssets:    map[string]struct{}{},
		PoolSnapshots: map[string]MidgardPool{},
		HasPoolData:   true,
	}
	var runeUSDs []float64
	for _, pool := range pools {
		if !strings.EqualFold(pool.Status, "available") && pool.Status != "" {
			continue
		}
		asset := normalizeAsset(pool.Asset)
		if asset == "" {
			continue
		}
		book.PoolAssets[asset] = struct{}{}
		book.PoolSnapshots[asset] = pool

		// Midgard provides assetPriceUSD directly.
		assetPriceUSD := parseFloat64(pool.AssetPriceUSD)
		if assetPriceUSD > 0 {
			book.AssetUSD[asset] = assetPriceUSD
		}

		// Derive RUNE USD from stable pools.
		runeDepth := float64(parseInt64(pool.RuneDepth)) / 1e8
		assetDepth := float64(parseInt64(pool.AssetDepth)) / 1e8
		if runeDepth > 0 && assetDepth > 0 && isStableAsset(asset) {
			runeUSDs = append(runeUSDs, assetDepth/runeDepth)
		}
	}
	if len(runeUSDs) == 0 {
		return book, fmt.Errorf("no stable pools available for USD normalization")
	}
	sort.Float64s(runeUSDs)
	book.RuneUSD = runeUSDs[len(runeUSDs)/2]
	book.AssetUSD["THOR.RUNE"] = book.RuneUSD
	return book, nil
}

func parseFloat64(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return f
}

type liveHoldingValue struct {
	Asset     string
	AmountRaw string
	USDSpot   float64
}

func (a *App) refreshActorTrackerLiveHoldings(ctx context.Context, nodes []FlowNode) ([]string, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("at least one node is required")
	}

	protocols, err := a.loadProtocolDirectory(ctx)
	if err != nil {
		return nil, err
	}
	prices, priceErr := a.buildPriceBook(ctx)
	warnings := []string{}
	if priceErr != nil {
		warnings = append(warnings, "spot USD normalization unavailable; falling back to asset-native values")
	}
	warnings = append(warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, protocols)...)
	return uniqueStrings(warnings), nil
}

func (a *App) enrichNodesWithLiveHoldings(ctx context.Context, nodes []FlowNode, prices priceBook, protocols protocolDirectory) []string {
	if len(nodes) == 0 {
		return nil
	}
	// Live-holdings enrichment runs after the graph build has already spent most of
	// the request budget. Detach it from the parent deadline, then enforce timeouts
	// per upstream lookup so slow public trackers do not consume the entire batch.
	baseLookupCtx := context.WithoutCancel(ctx)

	warnings := []string{}
	now := time.Now().UTC().Format(time.RFC3339)

	poolByAsset := map[string]MidgardPool{}
	poolsCtx, poolsCancel := context.WithTimeout(baseLookupCtx, a.liveHoldingsLookupTimeout("midgard", "THOR"))
	if pools, err := a.fetchPools(poolsCtx); err == nil {
		for _, pool := range pools {
			asset := normalizeAsset(pool.Asset)
			if asset == "" {
				continue
			}
			poolByAsset[asset] = pool
		}
	} else {
		warnings = append(warnings, "live holdings for pool nodes unavailable")
	}
	poolsCancel()

	type nodeRef struct {
		index int
	}
	type addressLookupTask struct {
		key       string
		chain     string
		address   string
		provider  string
		bucketKey string
		refs      []nodeRef
	}
	addressLookupTasks := map[string]*addressLookupTask{}
	nodeLookupRefs := make([]nodeRef, 0)
	queueAddressLookupTask := func(idx int) {
		address := normalizeAddress(getString(nodes[idx].Metrics, "address"))
		if address == "" {
			return
		}
		chain := strings.ToUpper(strings.TrimSpace(nodes[idx].Chain))
		if chain == "" {
			chain = normalizeChain("", address)
		}
		if chain == "" {
			return
		}
		key := frontierKey(chain, address)
		if key == "" {
			return
		}
		task, ok := addressLookupTasks[key]
		if !ok {
			provider := a.liveHoldingsProviderForChain(chain)
			task = &addressLookupTask{
				key:       key,
				chain:     chain,
				address:   address,
				provider:  provider,
				bucketKey: liveHoldingsBucketKey(provider, chain),
			}
			addressLookupTasks[key] = task
		}
		task.refs = append(task.refs, nodeRef{index: idx})
	}

	for i := range nodes {
		if nodes[i].Metrics == nil {
			nodes[i].Metrics = map[string]any{}
		}
		switch nodes[i].Kind {
		case "pool":
			poolAsset := normalizeAsset(getString(nodes[i].Metrics, "pool"))
			if poolAsset == "" {
				continue
			}
			pool, ok := poolByAsset[poolAsset]
			if !ok {
				continue
			}
			holdings := []liveHoldingValue{
				{
					Asset:     "THOR.RUNE",
					AmountRaw: strings.TrimSpace(pool.RuneDepth),
					USDSpot:   prices.usdFor("THOR.RUNE", pool.RuneDepth),
				},
				{
					Asset:     poolAsset,
					AmountRaw: strings.TrimSpace(pool.AssetDepth),
					USDSpot:   prices.usdFor(poolAsset, pool.AssetDepth),
				},
			}
			applyLiveHoldingMetrics(&nodes[i], holdings, "pool_snapshot", now)
		case "node":
			nodes[i].Metrics["node_total_bond"] = ""
			nodeLookupRefs = append(nodeLookupRefs, nodeRef{index: i})
		default:
			queueAddressLookupTask(i)
		}
	}

	var (
		thorBondedByAddress  map[string]string
		thorBondedByNode     map[string]string
		thorNodeStatusByNode map[string]string
		hasTHORLookups       bool
	)
	for _, task := range addressLookupTasks {
		if task != nil && strings.EqualFold(strings.TrimSpace(task.chain), "THOR") {
			hasTHORLookups = true
			break
		}
	}
	if hasTHORLookups || len(nodeLookupRefs) > 0 {
		bondLookupCtx, bondLookupCancel := context.WithTimeout(baseLookupCtx, a.liveHoldingsLookupTimeout("thornode", "THOR"))
		bondedByAddress, bondedByNode, nodeStatusByNode, err := a.fetchTHORBondIndexes(bondLookupCtx)
		bondLookupCancel()
		if err != nil {
			if hasTHORLookups {
				warnings = append(warnings, "live bonded rune unavailable for THOR addresses")
			}
			if len(nodeLookupRefs) > 0 {
				warnings = append(warnings, "live bonded rune unavailable for validator nodes")
			}
		} else {
			thorBondedByAddress = bondedByAddress
			thorBondedByNode = bondedByNode
			thorNodeStatusByNode = nodeStatusByNode
		}
	}

	for _, ref := range nodeLookupRefs {
		idx := ref.index
		nodeAddress := normalizeAddress(getString(nodes[idx].Metrics, "address"))
		amountRaw := ""
		nodeStatus := ""
		if thorBondedByNode != nil {
			amountRaw = strings.TrimSpace(thorBondedByNode[nodeAddress])
		}
		if thorNodeStatusByNode != nil {
			nodeStatus = strings.TrimSpace(thorNodeStatusByNode[nodeAddress])
		}
		if nodeAddress != "" {
			nodes[idx].Label = thorNodeDisplayLabel(nodeAddress, nodeStatus)
		}
		nodes[idx].Metrics["node_status"] = nodeStatus
		nodes[idx].Metrics["node_total_bond"] = amountRaw
		if hasGraphableLiquidity(amountRaw) {
			holdings := []liveHoldingValue{{
				Asset:     "THOR.RUNE",
				AmountRaw: amountRaw,
				USDSpot:   prices.usdFor("THOR.RUNE", amountRaw),
			}}
			applyLiveHoldingMetrics(&nodes[idx], holdings, "thornode_node_bond", now)
		} else {
			nodes[idx].Metrics["live_holdings_available"] = false
			nodes[idx].Metrics["live_holdings_status"] = "error"
		}
	}

	if len(addressLookupTasks) == 0 {
		return uniqueStrings(warnings)
	}

	type addressResult struct {
		taskKey  string
		chain    string
		address  string
		provider string
		holdings []liveHoldingValue
		err      error
		elapsed  time.Duration
	}
	results := make(chan addressResult, len(addressLookupTasks))
	buckets := map[string][]addressLookupTask{}
	for _, task := range addressLookupTasks {
		if task == nil {
			continue
		}
		buckets[task.bucketKey] = append(buckets[task.bucketKey], *task)
	}

	runBucket := func(tasks []addressLookupTask) {
		if len(tasks) == 0 {
			return
		}
		jobs := make(chan addressLookupTask, len(tasks))
		workerCount := min(a.liveHoldingsBucketConcurrency(tasks[0].provider, tasks[0].chain), len(tasks))
		if workerCount < 1 {
			workerCount = 1
		}
		var bucketWG sync.WaitGroup
		bucketWG.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer bucketWG.Done()
				for task := range jobs {
					taskTimeout := a.liveHoldingsLookupTimeout(task.provider, task.chain)
					taskCtx, taskCancel := context.WithTimeout(baseLookupCtx, taskTimeout)
					startedAt := time.Now()

					var (
						holdings []liveHoldingValue
						err      error
					)
					if task.chain == "THOR" {
						holdings, err = a.fetchTHORAddressLiveHoldings(taskCtx, task.address, prices, thorBondedByAddress)
					} else {
						holdings, err = a.fetchAddressLiveHoldings(taskCtx, task.chain, task.address, prices)
					}

					results <- addressResult{
						taskKey:  task.key,
						chain:    task.chain,
						address:  task.address,
						provider: task.provider,
						holdings: holdings,
						err:      err,
						elapsed:  time.Since(startedAt),
					}
					taskCancel()
				}
			}()
		}
		for _, task := range tasks {
			jobs <- task
		}
		close(jobs)
		bucketWG.Wait()
	}

	var lookupWG sync.WaitGroup
	for _, tasks := range buckets {
		bucketTasks := append([]addressLookupTask(nil), tasks...)
		lookupWG.Add(1)
		go func() {
			defer lookupWG.Done()
			runBucket(bucketTasks)
		}()
	}
	go func() {
		lookupWG.Wait()
		close(results)
	}()

	var failed []string
	for result := range results {
		task, ok := addressLookupTasks[result.taskKey]
		if !ok {
			continue
		}
		refs := task.refs
		if result.err != nil {
			failed = append(failed, fmt.Sprintf("%s:%s", task.chain, shortAddress(task.address)))
			fields := map[string]any{
				"chain":      task.chain,
				"address":    task.address,
				"provider":   result.provider,
				"elapsed_ms": result.elapsed.Milliseconds(),
			}
			if task.chain == "THOR" {
				fields["provider_candidates"] = "thornode,midgard"
			} else if providers := strings.Join(a.cfg.trackerProvidersForChain(task.chain), ","); providers != "" {
				fields["provider_candidates"] = providers
			}
			logError(baseLookupCtx, "actor_tracker_live_holdings_lookup_failed", result.err, fields)
			for _, ref := range refs {
				if nodes[ref.index].Metrics == nil {
					nodes[ref.index].Metrics = map[string]any{}
				}
				nodes[ref.index].Metrics["live_holdings_available"] = false
				nodes[ref.index].Metrics["live_holdings_status"] = "error"
			}
			continue
		}
		for _, ref := range refs {
			source := "external_live_balance"
			if task.chain == "THOR" {
				source = "thornode_midgard"
			}
			applyLiveHoldingMetrics(&nodes[ref.index], result.holdings, source, now)
		}
	}
	if len(failed) > 0 {
		sort.Strings(failed)
		limit := min(3, len(failed))
		warnings = append(warnings, fmt.Sprintf("live holdings unavailable for %d address nodes (%s)", len(failed), strings.Join(failed[:limit], ", ")))
	}
	return uniqueStrings(warnings)
}

func (a *App) liveHoldingsProviderForChain(chain string) string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	if chain == "THOR" {
		return "thornode"
	}
	if provider := strings.ToLower(strings.TrimSpace(a.cfg.trackerProviderForChain(chain))); provider != "" {
		return provider
	}
	return "unconfigured"
}

func (a *App) liveHoldingsLookupTimeout(provider, chain string) time.Duration {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	switch {
	case provider == "utxo":
		return a.clampLiveHoldingsLookupTimeout(8 * time.Second)
	case provider == "solana" || provider == "xrpl" || provider == "cosmos":
		return a.clampLiveHoldingsLookupTimeout(6 * time.Second)
	case provider == "etherscan" || provider == "blockscout" || provider == "nodereal" || provider == "trongrid":
		return a.clampLiveHoldingsLookupTimeout(12 * time.Second)
	case provider == "thornode" || provider == "midgard" || chain == "THOR":
		return a.clampLiveHoldingsLookupTimeout(12 * time.Second)
	default:
		return a.clampLiveHoldingsLookupTimeout(10 * time.Second)
	}
}

func (a *App) clampLiveHoldingsLookupTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if a != nil && a.cfg.RequestTimeout > 0 && a.cfg.RequestTimeout < timeout {
		return a.cfg.RequestTimeout
	}
	return timeout
}

func (a *App) liveHoldingsBucketConcurrency(provider, chain string) int {
	if concurrency, _ := trackerThrottlePolicy(provider, chain); concurrency > 0 {
		return concurrency
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "unconfigured":
		return 1
	case "utxo", "blockscout", "xrpl":
		return 3
	case "thornode", "midgard":
		return 2
	default:
		return 2
	}
}

func liveHoldingsBucketKey(provider, chain string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	if provider == "" {
		provider = "unconfigured"
	}
	if chain == "" {
		chain = "UNKNOWN"
	}
	return provider + "|" + chain
}

func applyLiveHoldingMetrics(node *FlowNode, holdings []liveHoldingValue, source, timestamp string) {
	if node == nil {
		return
	}
	if node.Metrics == nil {
		node.Metrics = map[string]any{}
	}
	sort.Slice(holdings, func(i, j int) bool {
		return holdings[i].USDSpot > holdings[j].USDSpot
	})
	totalUSD := 0.0
	assets := make([]map[string]any, 0, len(holdings))
	for _, holding := range holdings {
		if !hasGraphableLiquidity(holding.AmountRaw) {
			continue
		}
		totalUSD += holding.USDSpot
		assets = append(assets, map[string]any{
			"asset":      holding.Asset,
			"amount_raw": holding.AmountRaw,
			"usd_spot":   holding.USDSpot,
		})
	}
	node.Metrics["live_holdings_available"] = true
	node.Metrics["live_holdings_status"] = "available"
	node.Metrics["live_holdings_usd_spot"] = totalUSD
	node.Metrics["live_holdings_assets"] = assets
	node.Metrics["live_holdings_source"] = source
	node.Metrics["live_holdings_at"] = timestamp
}

func thorDenomToAsset(denom string) string {
	return normalizeTHORDenomAsset(denom)
}

func (b *graphBuilder) projectMidgardAction(action midgardAction, baseDepth int) ([]projectedSegment, []frontierAddress, []string) {
	segments, next, warnings, _ := b.projectMidgardActionWithExternal(action, baseDepth, nil)
	return segments, next, warnings
}

func (b *graphBuilder) projectMidgardActionWithExternal(action midgardAction, baseDepth int, externalTransfers []externalTransfer) ([]projectedSegment, []frontierAddress, []string, map[string]struct{}) {
	action, consumedExternalTransfers := b.stitchMidgardAction(action, externalTransfers)
	actionMeta := describeMidgardAction(action)
	actionClass := actionMeta.ActionClass
	if !b.allowed(actionClass) {
		return nil, nil, nil, consumedExternalTransfers
	}
	if strings.EqualFold(strings.TrimSpace(action.Status), "failed") {
		return nil, nil, nil, consumedExternalTransfers
	}

	legsIn := action.In
	legsOut := action.Out
	if len(legsIn) == 0 && len(legsOut) == 0 {
		return nil, nil, nil, consumedExternalTransfers
	}

	actionTime := parseMidgardActionTime(action.Date)
	if actionTime.IsZero() {
		actionTime = time.Now().UTC()
	}
	height := parseInt64(action.Height)
	fallbackTxID := cleanTxID(midgardSyntheticTxID(action))
	swapTxID := midgardSwapCorrelationTxID(action, fallbackTxID)
	addressesInAction := make([]string, 0, len(legsIn)+len(legsOut))
	for _, leg := range legsIn {
		addressesInAction = append(addressesInAction, leg.Address)
	}
	for _, leg := range legsOut {
		addressesInAction = append(addressesInAction, leg.Address)
	}
	actionActorIDs := b.actorIDsForAddresses(addressesInAction...)

	var segments []projectedSegment
	var nextAddresses []frontierAddress

	addSegment := func(source, target flowRef, asset, amount string, confidence float64, txID string) {
		if source.ID == "" || target.ID == "" {
			return
		}
		if !b.prices.supportsGraphAsset(asset) {
			return
		}
		amount = strings.TrimSpace(amount)
		if actionClass != "ownership" && !hasGraphableLiquidity(amount) {
			return
		}
		txID = cleanTxID(txID)
		if txID == "" {
			txID = fallbackTxID
		}
		seg := projectedSegment{
			Source:           source,
			Target:           target,
			ActionClass:      actionClass,
			ActionKey:        actionMeta.ActionKey,
			ActionLabel:      actionMeta.ActionLabel,
			ActionDomain:     actionMeta.ActionDomain,
			ContractType:     actionMeta.ContractType,
			ContractProtocol: actionMeta.ContractProtocol,
			Asset:            normalizeAsset(asset),
			AmountRaw:        amount,
			USDSpot:          b.prices.usdFor(asset, amount),
			TxID:             txID,
			Height:           height,
			Time:             actionTime,
			Confidence:       confidence,
			ActorIDs:         mergeInt64s(mergeInt64s(source.ActorIDs, target.ActorIDs), actionActorIDs),
		}
		if actionClass == "swaps" {
			seg.CanonicalKey = canonicalSwapSegmentKey(firstNonEmpty(swapTxID, seg.TxID), source.Address, target.Address, seg.Asset)
		}
		if b.minUSD > 0 && seg.USDSpot > 0 && seg.USDSpot < b.minUSD && !hasActorIDs(seg.ActorIDs) {
			return
		}
		segments = append(segments, seg)
		for _, ref := range []flowRef{source, target} {
			if shouldExpandAddressRef(ref) {
				nextAddresses = append(nextAddresses, frontierAddress{Address: ref.Address, Chain: ref.Chain})
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(action.Type), "contract") {
		contractDesc := lookupContractCallDescriptor(actionMeta.ContractType)
		sourceHint := ""
		if action.Metadata.Contract != nil {
			sourceHint = findContractExecutionAddress(action.Metadata.Contract.Msg)
		}
		receiverLegs, payoutLegs := splitMidgardContractLegs(action)
		useExecutionReceiver := preferExecutionAddressAsContractReceiver(actionMeta.ContractType)
		suppressPayouts := suppressContractPayoutProjection(actionMeta.ContractType)
		representativePayoutAddress := ""
		if isCalcStrategyRepresentative(actionMeta.ContractType) {
			representativePayoutAddress = findRepresentativeContractPayoutAddress(action)
		}
		suppressFallback := false

		for _, receiverLeg := range receiverLegs {
			receiverAddress := receiverLeg.Address
			if useExecutionReceiver && sourceHint != "" {
				receiverAddress = sourceHint
			}
			resolvedPayoutAddress := representativePayoutAddress
			if resolvedPayoutAddress == "" {
				resolvedPayoutAddress = normalizeAddress(b.calcPayoutByContract[normalizeAddress(receiverAddress)])
			}
			if resolvedPayoutAddress == "" {
				resolvedPayoutAddress = knownCalcRepresentativePayoutAddress(receiverAddress)
			}
			receiverRef := b.makeContractRef(receiverAddress, contractDesc, baseDepth+1)
			if receiverRef.ID == "" {
				continue
			}

			var inputSource flowRef
			if sourceHint != "" &&
				normalizeAddress(sourceHint) != receiverRef.Address &&
				!preferInboundContractSource(actionMeta.ContractType) {
				inputSource = b.makeContractRef(sourceHint, contractCallDescriptor{}, baseDepth)
			}

			inputAdded := false
			for _, inLeg := range legsIn {
				sourceRef := inputSource
				if sourceRef.ID == "" {
					sourceRef = b.makeAddressRef(inLeg.Address, chainFromMidgardCoins(inLeg.Coins), baseDepth)
				}
				if sourceRef.ID == "" || sourceRef.Key == receiverRef.Key {
					continue
				}
				txID := firstNonEmpty(strings.ToUpper(strings.TrimSpace(inLeg.TxID)), strings.ToUpper(strings.TrimSpace(receiverLeg.TxID)))
				if len(inLeg.Coins) > 0 {
					for _, coin := range inLeg.Coins {
						addSegment(sourceRef, receiverRef, coin.Asset, coin.Amount, 0.86, txID)
						inputAdded = true
					}
					continue
				}
				if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, inLeg, receiverLeg); ok {
					addSegment(sourceRef, receiverRef, inferredAsset, inferredAmount, 0.82, txID)
					inputAdded = true
				}
			}

			if !inputAdded && inputSource.ID != "" {
				if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, midgardActionLeg{}, receiverLeg); ok {
					addSegment(inputSource, receiverRef, inferredAsset, inferredAmount, 0.78, receiverLeg.TxID)
				}
			}

			for _, payoutLeg := range payoutLegs {
				if resolvedPayoutAddress != "" {
					targetRef := b.makeAddressRef(resolvedPayoutAddress, chainFromMidgardCoins(payoutLeg.Coins), baseDepth+2)
					if targetRef.ID != "" && targetRef.Key != receiverRef.Key {
						txID := strings.ToUpper(strings.TrimSpace(payoutLeg.TxID))
						for _, coin := range payoutLeg.Coins {
							if normalizeAsset(coin.Asset) != "THOR.TCY" {
								continue
							}
							addSegment(receiverRef, targetRef, coin.Asset, coin.Amount, 0.84, txID)
						}
					}
					continue
				}
				if isCalcStrategyFallbackRepresentative(actionMeta.ContractType) {
					suppressFallback = true
					continue
				}
				if suppressPayouts {
					suppressFallback = true
					continue
				}
				targetRef := b.makeAddressRef(payoutLeg.Address, chainFromMidgardCoins(payoutLeg.Coins), baseDepth+2)
				if targetRef.ID == "" || targetRef.Key == receiverRef.Key {
					continue
				}
				txID := strings.ToUpper(strings.TrimSpace(payoutLeg.TxID))
				for _, coin := range payoutLeg.Coins {
					addSegment(receiverRef, targetRef, coin.Asset, coin.Amount, 0.84, txID)
				}
			}
		}

		if len(segments) == 0 && !(suppressFallback && hasCalcStrategyExecuteMsgPayload(action)) {
			fallbackSource := flowRef{}
			if sourceHint != "" {
				fallbackSource = b.makeContractRef(sourceHint, contractCallDescriptor{}, baseDepth)
			}
			if fallbackSource.ID == "" && len(legsIn) > 0 {
				fallbackSource = b.makeAddressRef(legsIn[0].Address, chainFromMidgardCoins(legsIn[0].Coins), baseDepth)
			}
			for _, payoutLeg := range payoutLegs {
				targetRef := b.makeAddressRef(payoutLeg.Address, chainFromMidgardCoins(payoutLeg.Coins), baseDepth+1)
				if fallbackSource.ID == "" || targetRef.ID == "" || fallbackSource.Key == targetRef.Key {
					continue
				}
				txID := firstNonEmpty(strings.ToUpper(strings.TrimSpace(payoutLeg.TxID)), fallbackTxID)
				for _, coin := range payoutLeg.Coins {
					addSegment(fallbackSource, targetRef, coin.Asset, coin.Amount, 0.76, txID)
				}
			}
		}

		if len(segments) > 0 {
			return segments, uniqueFrontierAddresses(nextAddresses), nil, consumedExternalTransfers
		}
		return nil, nil, nil, consumedExternalTransfers
	}

	if actionClass == "bonds" {
		actionType := strings.ToLower(strings.TrimSpace(action.Type))
		resolveNodeAddressForLeg := func(leg midgardActionLeg) string {
			return normalizeAddress(b.bondMemoNodeByTx[cleanTxID(leg.TxID)])
		}
		resolveNodeAddressFromLegs := func(legs []midgardActionLeg) string {
			for _, leg := range legs {
				if nodeAddress := resolveNodeAddressForLeg(leg); nodeAddress != "" {
					return nodeAddress
				}
			}
			return ""
		}
		resolveNodeAddressFromAction := func() string {
			for _, txID := range midgardActionTxIDs(action) {
				if nodeAddress := normalizeAddress(b.bondMemoNodeByTx[cleanTxID(txID)]); nodeAddress != "" {
					return nodeAddress
				}
			}
			return ""
		}
		pickCoin := func(primary, fallback []midgardActionCoin) (string, string) {
			for _, coin := range primary {
				amount := strings.TrimSpace(coin.Amount)
				if !hasGraphableLiquidity(amount) {
					continue
				}
				asset := normalizeAsset(coin.Asset)
				if asset != "" {
					return asset, amount
				}
			}
			for _, coin := range fallback {
				amount := strings.TrimSpace(coin.Amount)
				if !hasGraphableLiquidity(amount) {
					continue
				}
				asset := normalizeAsset(coin.Asset)
				if asset != "" {
					return asset, amount
				}
			}
			return "THOR.RUNE", "0"
		}
		isOutboundBondFlow := strings.Contains(actionType, "unbond") || actionType == "leave" || strings.Contains(actionType, "slash") || strings.Contains(actionType, "reward")
		isRebondFlow := actionType == "rebond"
		bondProviderAddress := ""
		if action.Metadata.Bond != nil {
			bondProviderAddress = normalizeAddress(action.Metadata.Bond.Provider)
		}

		globalNodeAddress := firstNonEmpty(resolveNodeAddressFromAction(), resolveNodeAddressFromLegs(legsIn), resolveNodeAddressFromLegs(legsOut))
		for _, inLeg := range legsIn {
			walletAddress := strings.TrimSpace(inLeg.Address)
			if bondProviderAddress != "" {
				walletAddress = bondProviderAddress
			}
			walletRef := b.makeBondWalletRef(walletAddress, baseDepth)
			if walletRef.ID == "" || isAsgardModuleAddress(walletRef.Address) || isBondModuleAddress(walletRef.Address) {
				continue
			}
			if walletRef.Kind == "node" {
				continue
			}
			nodeAddress := firstNonEmpty(midgardRebondValidatorAddress(action), resolveNodeAddressForLeg(inLeg), globalNodeAddress)
			txID := strings.ToUpper(strings.TrimSpace(inLeg.TxID))
			var fallbackCoins []midgardActionCoin
			for _, outLeg := range legsOut {
				if isBondModuleAddress(outLeg.Address) {
					fallbackCoins = append(fallbackCoins, outLeg.Coins...)
				}
			}
			asset, amount := pickCoin(inLeg.Coins, fallbackCoins)
			if isRebondFlow {
				targetRef := b.makeBondWalletRef(midgardRebondNewBondAddress(action), baseDepth+1)
				if targetRef.ID == "" || targetRef.Key == walletRef.Key {
					continue
				}
				before := len(segments)
				addSegment(walletRef, targetRef, asset, amount, 0.9, txID)
				for i := before; i < len(segments); i++ {
					segments[i].ValidatorAddress = nodeAddress
					segments[i].ValidatorLabel = thorNodeDisplayLabel(nodeAddress, "")
				}
				continue
			}
			nodeRef := b.makeNodeRef(nodeAddress, baseDepth+1)
			if nodeRef.ID == "" {
				continue
			}
			if isOutboundBondFlow {
				addSegment(nodeRef, walletRef, asset, amount, 0.88, txID)
			} else {
				addSegment(walletRef, nodeRef, asset, amount, 0.9, txID)
			}
		}
		return segments, uniqueFrontierAddresses(nextAddresses), nil, consumedExternalTransfers
	}

	if actionClass == "swaps" {
		swapOutLegs, suppressedFeeLegs := selectMidgardSwapOutLegs(legsOut)
		if suppressedFeeLegs > 0 {
			b.feeActionDrop += suppressedFeeLegs
		}
		for _, inLeg := range legsIn {
			if isAsgardModuleAddress(inLeg.Address) {
				continue
			}
			source := b.makeAddressRef(inLeg.Address, chainFromMidgardCoins(inLeg.Coins), baseDepth)
			swapInAsset := ""
			swapInAmount := ""
			if len(inLeg.Coins) > 0 {
				swapInAsset = normalizeAsset(inLeg.Coins[0].Asset)
				swapInAmount = strings.TrimSpace(inLeg.Coins[0].Amount)
			}
			for _, outLeg := range swapOutLegs {
				if isAsgardModuleAddress(outLeg.Address) {
					continue
				}
				target := b.makeAddressRef(outLeg.Address, chainFromMidgardCoins(outLeg.Coins), baseDepth+1)
				if source.ID == "" || target.ID == "" || source.Key == target.Key {
					continue
				}
				coins := outLeg.Coins
				if len(coins) == 0 {
					coins = inLeg.Coins
				}
				txID := firstNonEmpty(strings.ToUpper(strings.TrimSpace(outLeg.TxID)), strings.ToUpper(strings.TrimSpace(inLeg.TxID)))
				if len(coins) == 0 {
					if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, inLeg, outLeg); ok {
						before := len(segments)
						addSegment(source, target, inferredAsset, inferredAmount, 0.7, txID)
						for i := before; i < len(segments); i++ {
							segments[i].SwapInAsset = swapInAsset
							segments[i].SwapInAmountRaw = swapInAmount
							segments[i].SwapOutAsset = normalizeAsset(inferredAsset)
							segments[i].SwapOutAmountRaw = strings.TrimSpace(inferredAmount)
						}
					}
					continue
				}
				for _, coin := range coins {
					before := len(segments)
					addSegment(source, target, coin.Asset, coin.Amount, 0.74, txID)
					for i := before; i < len(segments); i++ {
						segments[i].SwapInAsset = swapInAsset
						segments[i].SwapInAmountRaw = swapInAmount
						segments[i].SwapOutAsset = normalizeAsset(coin.Asset)
						segments[i].SwapOutAmountRaw = strings.TrimSpace(coin.Amount)
					}
				}
			}
		}
		if len(segments) > 0 {
			return segments, uniqueFrontierAddresses(nextAddresses), nil, consumedExternalTransfers
		}
	}

	if actionClass == "liquidity" {
		poolRef := b.makePoolRef(midgardActionPool(action), baseDepth+1)
		if poolRef.ID != "" {
			for _, inLeg := range legsIn {
				source := b.makeAddressRef(inLeg.Address, chainFromMidgardCoins(inLeg.Coins), baseDepth)
				txID := strings.ToUpper(strings.TrimSpace(inLeg.TxID))
				if len(inLeg.Coins) == 0 {
					if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, inLeg, midgardActionLeg{}); ok {
						addSegment(source, poolRef, inferredAsset, inferredAmount, 0.72, txID)
					} else {
						addSegment(source, poolRef, "THOR.RUNE", "0", 0.62, txID)
					}
					continue
				}
				for _, coin := range inLeg.Coins {
					addSegment(source, poolRef, coin.Asset, coin.Amount, 0.74, txID)
				}
			}
			for _, outLeg := range legsOut {
				target := b.makeAddressRef(outLeg.Address, chainFromMidgardCoins(outLeg.Coins), baseDepth+2)
				txID := strings.ToUpper(strings.TrimSpace(outLeg.TxID))
				if len(outLeg.Coins) == 0 {
					if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, midgardActionLeg{}, outLeg); ok {
						addSegment(poolRef, target, inferredAsset, inferredAmount, 0.7, txID)
					} else {
						addSegment(poolRef, target, "THOR.RUNE", "0", 0.6, txID)
					}
					continue
				}
				for _, coin := range outLeg.Coins {
					addSegment(poolRef, target, coin.Asset, coin.Amount, 0.74, txID)
				}
			}
			if len(segments) > 0 {
				return segments, uniqueFrontierAddresses(nextAddresses), nil, consumedExternalTransfers
			}
		}
	}

	for _, inLeg := range legsIn {
		source := b.makeAddressRef(inLeg.Address, chainFromMidgardCoins(inLeg.Coins), baseDepth)
		for _, outLeg := range legsOut {
			target := b.makeAddressRef(outLeg.Address, chainFromMidgardCoins(outLeg.Coins), baseDepth+1)
			coins := outLeg.Coins
			if len(coins) == 0 {
				coins = inLeg.Coins
			}
			txID := firstNonEmpty(strings.ToUpper(strings.TrimSpace(outLeg.TxID)), strings.ToUpper(strings.TrimSpace(inLeg.TxID)))
			if len(coins) == 0 {
				if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, inLeg, outLeg); ok {
					addSegment(source, target, inferredAsset, inferredAmount, 0.68, txID)
				} else {
					addSegment(source, target, "THOR.RUNE", "0", 0.62, txID)
				}
				continue
			}
			for _, coin := range coins {
				addSegment(source, target, coin.Asset, coin.Amount, 0.72, txID)
			}
		}
	}

	return segments, uniqueFrontierAddresses(nextAddresses), nil, consumedExternalTransfers
}

func selectMidgardSwapOutLegs(legsOut []midgardActionLeg) ([]midgardActionLeg, int) {
	if len(legsOut) <= 1 {
		return legsOut, 0
	}
	hasExplicitOutTx := false
	for _, leg := range legsOut {
		if cleanTxID(leg.TxID) != "" {
			hasExplicitOutTx = true
			break
		}
	}
	if !hasExplicitOutTx {
		return legsOut, 0
	}
	filtered := make([]midgardActionLeg, 0, len(legsOut))
	suppressed := 0
	for _, leg := range legsOut {
		if cleanTxID(leg.TxID) == "" && isMidgardFeeLikeSwapOutLeg(leg) {
			suppressed++
			continue
		}
		filtered = append(filtered, leg)
	}
	if len(filtered) == 0 || suppressed == 0 {
		return legsOut, 0
	}
	return filtered, suppressed
}

func isMidgardFeeLikeSwapOutLeg(leg midgardActionLeg) bool {
	if normalizeChain("", leg.Address) != "THOR" {
		return false
	}
	if len(leg.Coins) == 0 {
		return false
	}
	for _, coin := range leg.Coins {
		if normalizeAsset(coin.Asset) != "THOR.RUNE" {
			return false
		}
		if !hasGraphableLiquidity(coin.Amount) {
			return false
		}
	}
	return true
}

func (b *graphBuilder) stitchMidgardAction(action midgardAction, externalTransfers []externalTransfer) (midgardAction, map[string]struct{}) {
	if len(externalTransfers) == 0 {
		return action, map[string]struct{}{}
	}
	consumed := map[string]struct{}{}
	byTxID := map[string][]externalTransfer{}
	consumeActionTxIDTransfers := func() {
		for _, txID := range midgardActionTxIDs(action) {
			for _, match := range byTxID[txID] {
				consumed[externalTransferKey(match)] = struct{}{}
			}
		}
	}
	for _, transfer := range externalTransfers {
		txID := cleanTxID(transfer.TxID)
		if txID == "" {
			continue
		}
		byTxID[txID] = append(byTxID[txID], transfer)
	}

	// Secure actions should be represented by the Midgard secure segment only.
	// Consume any external transfer with the same txID to avoid duplicate
	// tracker.utxo.transfer edges for the same underlying secure tx.
	if strings.EqualFold(strings.TrimSpace(action.Type), "secure") {
		consumeActionTxIDTransfers()
		return action, consumed
	}

	for i := range action.In {
		leg := &action.In[i]
		if !b.isProtocolTransitAddress(leg.Address) {
			continue
		}
		matches := b.matchInboundExternalTransfers(leg, byTxID[cleanTxID(leg.TxID)])
		if len(matches) == 0 {
			continue
		}
		senders := map[string]struct{}{}
		for _, match := range matches {
			senders[normalizeAddress(match.From)] = struct{}{}
			consumed[externalTransferKey(match)] = struct{}{}
		}
		if len(senders) == 1 {
			for sender := range senders {
				leg.Address = sender
			}
		}
	}

	for _, leg := range action.Out {
		matches := b.matchOutboundExternalTransfers(leg, byTxID[cleanTxID(leg.TxID)])
		for _, match := range matches {
			consumed[externalTransferKey(match)] = struct{}{}
		}
	}

	// Add-liquidity actions often carry the user's chain txID directly on the
	// Midgard leg instead of a protocol transit leg, so consume same-tx tracker
	// transfers to avoid duplicate liquidity+transfer rendering.
	actionType := strings.ToLower(strings.TrimSpace(action.Type))
	if strings.Contains(actionType, "addliquidity") || strings.Contains(actionType, "add_liquidity") {
		consumeActionTxIDTransfers()
	}

	return action, consumed
}

func (b *graphBuilder) isProtocolTransitAddress(address string) bool {
	meta, ok := b.protocols.AddressKinds[normalizeAddress(address)]
	if !ok {
		return false
	}
	return meta.Kind == "inbound" || meta.Kind == "router"
}

func (b *graphBuilder) matchInboundExternalTransfers(leg *midgardActionLeg, candidates []externalTransfer) []externalTransfer {
	if leg == nil || len(candidates) == 0 || !b.isProtocolTransitAddress(leg.Address) {
		return nil
	}
	var filtered []externalTransfer
	for _, candidate := range candidates {
		if normalizeAddress(candidate.To) != normalizeAddress(leg.Address) {
			continue
		}
		if b.isProtocolTransitAddress(candidate.From) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filterExternalTransfersByCoins(filtered, leg.Coins)
}

func (b *graphBuilder) matchOutboundExternalTransfers(leg midgardActionLeg, candidates []externalTransfer) []externalTransfer {
	if len(candidates) == 0 {
		return nil
	}
	var filtered []externalTransfer
	for _, candidate := range candidates {
		if !b.isProtocolTransitAddress(candidate.From) {
			continue
		}
		if normalizeAddress(candidate.To) != normalizeAddress(leg.Address) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filterExternalTransfersByCoins(filtered, leg.Coins)
}

func filterExternalTransfersByCoins(candidates []externalTransfer, coins []midgardActionCoin) []externalTransfer {
	if len(candidates) == 0 {
		return nil
	}
	if len(coins) == 0 {
		if len(candidates) == 1 {
			return candidates
		}
		return nil
	}
	assetSet := map[string]struct{}{}
	for _, coin := range coins {
		if asset := normalizeAsset(coin.Asset); asset != "" {
			assetSet[asset] = struct{}{}
		}
	}
	if len(assetSet) == 0 {
		if len(candidates) == 1 {
			return candidates
		}
		return nil
	}
	var exact []externalTransfer
	for _, candidate := range candidates {
		if _, ok := assetSet[normalizeAsset(candidate.Asset)]; ok {
			exact = append(exact, candidate)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	if len(candidates) == 1 {
		return candidates
	}
	return nil
}

func (b *graphBuilder) addProjectedSegment(seg projectedSegment) {
	source := b.ensureNode(seg.Source)
	target := b.ensureNode(seg.Target)
	if source.ID == "" || target.ID == "" {
		return
	}
	if seg.ActionClass != "ownership" {
		canonical := strings.TrimSpace(seg.CanonicalKey)
		if canonical != "" {
			if b.seenCanonicalKey == nil {
				b.seenCanonicalKey = map[string]struct{}{}
			}
			if _, exists := b.seenCanonicalKey[canonical]; exists {
				if seg.ActionClass == "swaps" {
					b.swapDeduped++
				}
				return
			}
			b.seenCanonicalKey[canonical] = struct{}{}
			if seg.ActionClass == "swaps" {
				b.swapEmitted++
			}
		}
	}
	actionKey := firstNonEmpty(seg.ActionKey, seg.ActionClass)
	validatorAddress := normalizeAddress(seg.ValidatorAddress)
	validatorLabel := strings.TrimSpace(seg.ValidatorLabel)
	if validatorAddress != "" && validatorLabel == "" {
		validatorLabel = thorNodeDisplayLabel(validatorAddress, "")
	}
	meta := mergeAssetMetadata(assetMetadata{
		AssetKind:     seg.AssetKind,
		TokenStandard: seg.TokenStandard,
		TokenAddress:  seg.TokenAddress,
		TokenSymbol:   seg.TokenSymbol,
		TokenName:     seg.TokenName,
		TokenDecimals: seg.TokenDecimals,
	}, assetMetadataFromAsset(seg.Asset))
	edgeKey := fmt.Sprintf("%s|%s|%s", source.ID, target.ID, actionKey)
	if validatorAddress != "" && isRebondActionKey(actionKey) {
		edgeKey = fmt.Sprintf("%s|validator:%s", edgeKey, validatorAddress)
	}
	edge, ok := b.edges[edgeKey]
	if !ok {
		edge = &FlowEdge{
			ID:               edgeKey,
			From:             source.ID,
			To:               target.ID,
			ActionClass:      seg.ActionClass,
			ActionKey:        actionKey,
			ActionLabel:      firstNonEmpty(seg.ActionLabel, humanizeActionKey(actionKey)),
			ActionDomain:     seg.ActionDomain,
			ValidatorAddress: validatorAddress,
			ValidatorLabel:   validatorLabel,
			ContractType:     seg.ContractType,
			ContractProtocol: seg.ContractProtocol,
			Confidence:       seg.Confidence,
		}
		b.edges[edgeKey] = edge
	}
	edge.ActorIDs = mergeInt64s(edge.ActorIDs, seg.ActorIDs)
	if edge.ActionLabel == "" {
		edge.ActionLabel = firstNonEmpty(seg.ActionLabel, edge.ActionLabel)
	}
	if edge.ActionDomain == "" {
		edge.ActionDomain = seg.ActionDomain
	}
	if edge.ValidatorAddress == "" {
		edge.ValidatorAddress = validatorAddress
	}
	if edge.ValidatorLabel == "" {
		edge.ValidatorLabel = validatorLabel
	}
	if edge.ContractType == "" {
		edge.ContractType = seg.ContractType
	}
	if edge.ContractProtocol == "" {
		edge.ContractProtocol = seg.ContractProtocol
	}
	if seg.ActionClass == "swaps" {
		inAsset := normalizeAsset(seg.SwapInAsset)
		inAmount := strings.TrimSpace(seg.SwapInAmountRaw)
		outAsset := normalizeAsset(firstNonEmpty(seg.SwapOutAsset, seg.Asset))
		outAmount := strings.TrimSpace(firstNonEmpty(seg.SwapOutAmountRaw, seg.AmountRaw))
		added := false

		if inAsset != "" && hasGraphableLiquidity(inAmount) {
			inMeta := assetMetadataFromAsset(inAsset)
			mergeEdgeTransactionAsset(edge, seg.TxID, seg.Height, seg.Time, inAsset, inAmount, b.prices.usdFor(inAsset, inAmount), inMeta, "in")
			added = true
		}
		if outAsset != "" && hasGraphableLiquidity(outAmount) {
			outMeta := assetMetadataFromAsset(outAsset)
			mergeEdgeTransactionAsset(edge, seg.TxID, seg.Height, seg.Time, outAsset, outAmount, b.prices.usdFor(outAsset, outAmount), outMeta, "out")
			added = true
		}
		if !added {
			mergeEdgeTransactionAsset(edge, seg.TxID, seg.Height, seg.Time, seg.Asset, seg.AmountRaw, seg.USDSpot, meta, "")
		}
	} else {
		mergeEdgeTransactionAsset(edge, seg.TxID, seg.Height, seg.Time, seg.Asset, seg.AmountRaw, seg.USDSpot, meta, "")
	}
	recomputeEdgeAggregate(edge)

	source.Metrics["out_edges"] = intMetric(source.Metrics["out_edges"]) + 1
	target.Metrics["in_edges"] = intMetric(target.Metrics["in_edges"]) + 1

	if seg.ActionClass == "ownership" {
		return
	}
	actionID := strings.Join([]string{seg.TxID, actionKey, source.ID, target.ID}, "|")
	if validatorAddress != "" && isRebondActionKey(actionKey) {
		actionID = actionID + "|validator:" + validatorAddress
	}
	action, ok := b.actions[actionID]
	if !ok {
		action = &SupportingAction{
			TxID:             seg.TxID,
			ActionClass:      seg.ActionClass,
			ActionKey:        actionKey,
			ActionLabel:      firstNonEmpty(seg.ActionLabel, humanizeActionKey(actionKey)),
			ActionDomain:     seg.ActionDomain,
			ValidatorAddress: validatorAddress,
			ValidatorLabel:   validatorLabel,
			ContractType:     seg.ContractType,
			ContractProtocol: seg.ContractProtocol,
			PrimaryAsset:     seg.Asset,
			AssetKind:        meta.AssetKind,
			TokenStandard:    meta.TokenStandard,
			TokenAddress:     meta.TokenAddress,
			TokenSymbol:      meta.TokenSymbol,
			TokenName:        meta.TokenName,
			TokenDecimals:    meta.TokenDecimals,
			AmountRaw:        seg.AmountRaw,
			USDSpot:          seg.USDSpot,
			Height:           seg.Height,
			Time:             seg.Time,
			FromNode:         source.ID,
			ToNode:           target.ID,
			ActorIDs:         seg.ActorIDs,
		}
		b.actions[actionID] = action
	} else {
		action.USDSpot += seg.USDSpot
		action.ActorIDs = mergeInt64s(action.ActorIDs, seg.ActorIDs)
		if action.ActionLabel == "" {
			action.ActionLabel = seg.ActionLabel
		}
		if action.ActionDomain == "" {
			action.ActionDomain = seg.ActionDomain
		}
		if action.ValidatorAddress == "" {
			action.ValidatorAddress = validatorAddress
		}
		if action.ValidatorLabel == "" {
			action.ValidatorLabel = validatorLabel
		}
		if action.ContractType == "" {
			action.ContractType = seg.ContractType
		}
		if action.ContractProtocol == "" {
			action.ContractProtocol = seg.ContractProtocol
		}
		if action.PrimaryAsset == "" {
			action.PrimaryAsset = seg.Asset
		}
		if action.AssetKind == "" {
			action.AssetKind = meta.AssetKind
		}
		if action.TokenStandard == "" {
			action.TokenStandard = meta.TokenStandard
		}
		if action.TokenAddress == "" {
			action.TokenAddress = meta.TokenAddress
		}
		if action.TokenSymbol == "" {
			action.TokenSymbol = meta.TokenSymbol
		}
		if action.TokenName == "" {
			action.TokenName = meta.TokenName
		}
		if action.TokenDecimals == 0 {
			action.TokenDecimals = meta.TokenDecimals
		}
		if action.AmountRaw == "" {
			action.AmountRaw = seg.AmountRaw
		}
	}
}

func (b *graphBuilder) ensureNode(ref flowRef) *FlowNode {
	if ref.ID == "" {
		return &FlowNode{}
	}
	// Deduplicate by Key (normalized address / entity) so the same address at
	// different hop depths merges into a single graph node. This prevents
	// disconnected components when a frontier address is both a target from
	// one hop and a source for the next.
	dedup := ref.Key
	if dedup == "" {
		dedup = ref.ID
	}
	node, ok := b.nodes[dedup]
	if !ok {
		metrics := map[string]any{}
		for k, v := range ref.Metrics {
			metrics[k] = v
		}
		node = &FlowNode{
			ID:        ref.ID,
			Kind:      ref.Kind,
			Label:     ref.Label,
			Chain:     ref.Chain,
			Stage:     ref.Stage,
			Depth:     ref.Depth,
			ActorIDs:  append([]int64{}, ref.ActorIDs...),
			Shared:    ref.Shared,
			Collapsed: ref.Collapsed,
			Metrics:   metrics,
		}
		if node.Metrics == nil {
			node.Metrics = map[string]any{}
		}
		b.nodes[dedup] = node
		return node
	}
	if node.Depth > ref.Depth {
		node.Depth = ref.Depth
	}
	node.ActorIDs = mergeInt64s(node.ActorIDs, ref.ActorIDs)
	node.Shared = node.Shared || ref.Shared
	if node.Label == "" {
		node.Label = ref.Label
	}
	for k, v := range ref.Metrics {
		if _, exists := node.Metrics[k]; !exists {
			node.Metrics[k] = v
		}
	}
	return node
}

func (b *graphBuilder) makeAddressRef(address, chain string, depth int) flowRef {
	address = strings.TrimSpace(address)
	if address == "" {
		return flowRef{}
	}
	norm := normalizeAddress(address)
	if graphExcludedAddresses[norm] {
		return flowRef{}
	}
	resolvedChain := normalizeChain(chain, address)
	if b != nil && len(b.addressRefOverrides) > 0 {
		if ref, ok := b.addressRefOverrides[frontierKey(resolvedChain, address)]; ok {
			return ref
		}
	}
	actorKey := frontierKey(chain, address)
	actorIDs := append([]int64{}, b.ownerMap[actorKey]...)
	if len(actorIDs) == 0 {
		actorIDs = append([]int64{}, b.ownerMap[norm]...)
	}
	if len(actorIDs) > 0 {
		label := shortAddress(address)
		if len(actorIDs) == 1 {
			if actor, ok := b.actorsByID[actorIDs[0]]; ok {
				label = actor.Name + " Addr " + shortAddress(address)
			}
		}
		return flowRef{
			ID:        fmt.Sprintf("actor_address:%s:actor_address:%d", norm, depth),
			Key:       norm,
			Kind:      "actor_address",
			Label:     label,
			Chain:     normalizeChain(chain, address),
			Stage:     "actor_address",
			Depth:     depth,
			ActorIDs:  actorIDs,
			Shared:    len(actorIDs) > 1,
			Collapsed: true,
			Address:   norm,
			Metrics: map[string]any{
				"address": address,
			},
		}
	}

	if meta, ok := b.protocols.AddressKinds[norm]; ok {
		stage := "protocol"
		switch meta.Kind {
		case "node", "bond_address":
			stage = "node_bond"
		case "inbound", "router":
			stage = "protocol"
		}
		return flowRef{
			ID:      fmt.Sprintf("%s:%s:%s:%d", meta.Kind, norm, stage, depth),
			Key:     norm,
			Kind:    meta.Kind,
			Label:   meta.Label,
			Chain:   meta.Chain,
			Stage:   stage,
			Depth:   depth,
			Address: norm,
			Metrics: map[string]any{
				"address":      address,
				"node_address": meta.NodeAddress,
			},
		}
	}

	label := shortAddress(address)
	if knownLabel, ok := knownAddressLabels[norm]; ok {
		label = knownLabel
	} else if blacklistLabel, ok := frontierBlacklist[norm]; ok {
		label = blacklistLabel
	}

	// For EVM addresses (0x-prefix), include chain in the dedup key so the same
	// address on ETH vs BASE etc. renders as separate graph nodes.
	nodeKey := norm
	nodeID := fmt.Sprintf("external_address:%s:external:%d", norm, depth)
	if isLikelyEVMAddress(norm) && resolvedChain != "" {
		nodeKey = resolvedChain + "|" + norm
		nodeID = fmt.Sprintf("external_address:%s|%s:external:%d", resolvedChain, norm, depth)
	}

	return flowRef{
		ID:        nodeID,
		Key:       nodeKey,
		Kind:      "external_address",
		Label:     label,
		Chain:     resolvedChain,
		Stage:     "external",
		Depth:     depth,
		Collapsed: true,
		Address:   norm,
		Metrics: map[string]any{
			"address": address,
		},
	}
}

func (b *graphBuilder) makeContractRef(address string, descriptor contractCallDescriptor, depth int) flowRef {
	address = strings.TrimSpace(address)
	if address == "" {
		return flowRef{}
	}
	chainHint := normalizeChain("", address)
	if chainHint == "" {
		chainHint = "THOR"
	}
	ref := b.makeAddressRef(address, chainHint, depth)
	if ref.ID == "" {
		return ref
	}
	switch ref.Kind {
	case "actor_address", "bond_address", "inbound", "router", "node":
		return ref
	}
	norm := normalizeAddress(address)
	resolvedChain := normalizeChain(chainHint, address)
	label := firstNonEmpty(knownAddressLabels[norm], descriptor.Contract)
	if label == "" {
		label = "Contract " + shortAddress(address)
	} else if knownAddressLabels[norm] == "" {
		label = label + " " + shortAddress(address)
	}
	ref.ID = fmt.Sprintf("contract_address:%s:contract:%d", norm, depth)
	ref.Key = norm
	if isLikelyEVMAddress(norm) && resolvedChain != "" {
		ref.ID = fmt.Sprintf("contract_address:%s|%s:contract:%d", resolvedChain, norm, depth)
		ref.Key = resolvedChain + "|" + norm
	}
	ref.Kind = "contract_address"
	ref.Label = label
	ref.Chain = resolvedChain
	ref.Stage = "contract"
	ref.Collapsed = true
	ref.Address = norm
	if ref.Metrics == nil {
		ref.Metrics = map[string]any{}
	}
	ref.Metrics["address"] = address
	if descriptor.ContractType != "" {
		ref.Metrics["contract_type"] = descriptor.ContractType
	}
	if descriptor.Contract != "" {
		ref.Metrics["contract_protocol"] = descriptor.Contract
	}
	return ref
}

func (b *graphBuilder) makePoolRef(pool string, depth int) flowRef {
	pool = normalizeAsset(strings.TrimSpace(pool))
	if pool == "" {
		return flowRef{}
	}
	return flowRef{
		ID:      fmt.Sprintf("pool:%s:pool:%d", normalizeAsset(pool), depth),
		Key:     normalizeAsset(pool),
		Kind:    "pool",
		Label:   poolDisplayLabel(pool),
		Chain:   chainFromAsset(pool),
		Stage:   "pool",
		Depth:   depth,
		Metrics: map[string]any{"pool": pool},
	}
}

func thorNodeDisplayLabel(address, status string) string {
	address = normalizeAddress(address)
	if address == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return "Validator " + shortAddress(address)
	case "whitelisted":
		return "Whitelisted Node " + shortAddress(address)
	case "standby":
		return "Standby Node " + shortAddress(address)
	case "disabled":
		return "Disabled Node " + shortAddress(address)
	default:
		return "Node " + shortAddress(address)
	}
}

func midgardRebondNewBondAddress(action midgardAction) string {
	if action.Metadata.Rebond != nil {
		if value := normalizeAddress(action.Metadata.Rebond.NewBondAddress); value != "" {
			return value
		}
		memo := strings.TrimSpace(action.Metadata.Rebond.Memo)
		if memo != "" {
			parts := strings.Split(memo, ":")
			if len(parts) >= 3 && strings.EqualFold(strings.TrimSpace(parts[0]), "REBOND") {
				return normalizeAddress(parts[2])
			}
		}
	}
	return ""
}

func midgardRebondValidatorAddress(action midgardAction) string {
	if action.Metadata.Rebond != nil {
		if value := normalizeAddress(action.Metadata.Rebond.NodeAddress); value != "" {
			return value
		}
		memo := strings.TrimSpace(action.Metadata.Rebond.Memo)
		if memo != "" {
			return normalizeAddress(parseBondMemoNodeAddress(memo))
		}
	}
	return ""
}

func (b *graphBuilder) makeNodeRef(nodeAddress string, depth int) flowRef {
	nodeAddress = strings.TrimSpace(nodeAddress)
	if nodeAddress == "" {
		return flowRef{}
	}
	normalized := normalizeAddress(nodeAddress)
	metrics := map[string]any{
		"address": nodeAddress,
	}
	return flowRef{
		ID:      fmt.Sprintf("node:%s:node_bond:%d", normalized, depth),
		Key:     normalized,
		Kind:    "node",
		Label:   thorNodeDisplayLabel(nodeAddress, ""),
		Chain:   "THOR",
		Stage:   "node_bond",
		Depth:   depth,
		Address: normalized,
		Metrics: metrics,
	}
}

func (b *graphBuilder) makeBondRef(address string, depth int) flowRef {
	address = strings.TrimSpace(address)
	if address == "" {
		return flowRef{}
	}
	ref := b.makeAddressRef(address, "THOR", depth)
	if ref.Kind == "external_address" {
		ref.Kind = "bond_address"
		ref.Stage = "node_bond"
		ref.ID = fmt.Sprintf("bond_address:%s:node_bond:%d", normalizeAddress(address), depth)
		ref.Label = "Bond " + shortAddress(address)
	}
	return ref
}

func (b *graphBuilder) makeBondWalletRef(address string, depth int) flowRef {
	address = strings.TrimSpace(address)
	if address == "" {
		return flowRef{}
	}
	ref := b.makeAddressRef(address, "THOR", depth)
	if ref.ID == "" || ref.Kind == "actor_address" {
		return ref
	}
	if ref.Kind != "bond_address" {
		return ref
	}
	norm := normalizeAddress(address)
	ref.ID = fmt.Sprintf("external_address:%s:external:%d", norm, depth)
	ref.Key = norm
	ref.Kind = "external_address"
	ref.Stage = "external"
	ref.Label = "Bond Wallet " + shortAddress(address)
	ref.Chain = "THOR"
	ref.Collapsed = true
	ref.Address = norm
	if ref.Metrics == nil {
		ref.Metrics = map[string]any{}
	}
	ref.Metrics["address"] = address
	return ref
}

func (b *graphBuilder) allowed(actionClass string) bool {
	if len(b.allowedFlowTypes) == 0 {
		return true
	}
	return b.allowedFlowTypes[actionClass]
}

func (b *graphBuilder) actorIDsForAddresses(values ...string) []int64 {
	var out []int64
	for _, value := range values {
		norm := normalizeAddress(value)
		if norm == "" {
			continue
		}
		for _, actorID := range b.ownerMap[norm] {
			out = appendUniqueInt64(out, actorID)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (b *graphBuilder) nodeList() []FlowNode {
	out := make([]FlowNode, 0, len(b.nodes))
	for _, node := range b.nodes {
		node.ActorIDs = uniqueInt64s(node.ActorIDs)
		out = append(out, *node)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth == out[j].Depth {
			if out[i].Stage == out[j].Stage {
				return out[i].Label < out[j].Label
			}
			return out[i].Stage < out[j].Stage
		}
		return out[i].Depth < out[j].Depth
	})
	return out
}

func (b *graphBuilder) applyNodeLabelsToValidatorMetadata(nodes []FlowNode) {
	if len(nodes) == 0 {
		return
	}
	labelByAddress := map[string]string{}
	for _, node := range nodes {
		if node.Kind != "node" {
			continue
		}
		address := normalizeAddress(getString(node.Metrics, "address"))
		if address == "" {
			continue
		}
		label := strings.TrimSpace(node.Label)
		if label == "" {
			label = thorNodeDisplayLabel(address, getString(node.Metrics, "node_status"))
		}
		labelByAddress[address] = label
	}
	for _, edge := range b.edges {
		if label := strings.TrimSpace(labelByAddress[normalizeAddress(edge.ValidatorAddress)]); label != "" {
			edge.ValidatorLabel = label
		}
	}
	for _, action := range b.actions {
		if label := strings.TrimSpace(labelByAddress[normalizeAddress(action.ValidatorAddress)]); label != "" {
			action.ValidatorLabel = label
		}
	}
}

func (b *graphBuilder) edgeList() []FlowEdge {
	out := make([]FlowEdge, 0, len(b.edges))
	for _, edge := range b.edges {
		edge.ActorIDs = uniqueInt64s(edge.ActorIDs)
		recomputeEdgeAggregate(edge)
		sort.Slice(edge.Assets, func(i, j int) bool { return edge.Assets[i].USDSpot > edge.Assets[j].USDSpot })
		for i := range edge.Transactions {
			sort.Slice(edge.Transactions[i].Assets, func(a, b int) bool {
				return edge.Transactions[i].Assets[a].USDSpot > edge.Transactions[i].Assets[b].USDSpot
			})
		}
		sort.Slice(edge.Transactions, func(i, j int) bool {
			if edge.Transactions[i].Time.Equal(edge.Transactions[j].Time) {
				return edge.Transactions[i].TxID < edge.Transactions[j].TxID
			}
			if edge.Transactions[i].Time.IsZero() {
				return false
			}
			if edge.Transactions[j].Time.IsZero() {
				return true
			}
			return edge.Transactions[i].Time.Before(edge.Transactions[j].Time)
		})
		sort.Strings(edge.TxIDs)
		sort.Slice(edge.Heights, func(i, j int) bool { return edge.Heights[i] < edge.Heights[j] })
		out = append(out, *edge)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USDSpot == out[j].USDSpot {
			return out[i].ID < out[j].ID
		}
		return out[i].USDSpot > out[j].USDSpot
	})
	return out
}

func (b *graphBuilder) actionList() []SupportingAction {
	out := make([]SupportingAction, 0, len(b.actions))
	for _, action := range b.actions {
		action.ActorIDs = uniqueInt64s(action.ActorIDs)
		out = append(out, *action)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time.Equal(out[j].Time) {
			return out[i].TxID < out[j].TxID
		}
		return out[i].Time.Before(out[j].Time)
	})
	return out
}

func (p priceBook) usdFor(asset, amountRaw string) float64 {
	asset = normalizeAsset(asset)
	if asset == "" || amountRaw == "" {
		return 0
	}
	price, ok := p.AssetUSD[asset]
	if !ok || price <= 0 {
		return 0
	}
	amount := float64(parseInt64(amountRaw)) / 1e8
	return amount * price
}

func (p priceBook) hasPoolAsset(asset string) bool {
	asset = normalizeAsset(asset)
	if asset == "" || len(p.PoolAssets) == 0 {
		return false
	}
	_, ok := p.PoolAssets[asset]
	return ok
}

func (p priceBook) supportsGraphAsset(asset string) bool {
	meta := assetMetadataFromAsset(asset)
	if meta.AssetKind != "fungible_token" {
		return true
	}
	if !p.HasPoolData || len(p.PoolAssets) == 0 {
		return true
	}
	return p.hasPoolAsset(asset)
}

func eventActionClass(eventType string) string {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "add_liquidity", "withdraw", "withdraw_liquidity", "rune_pool_deposit", "rune_pool_withdraw", "refund", "secured_asset_deposit", "secured_asset_withdraw", "trade_account_deposit", "trade_account_withdraw":
		return "liquidity"
	case "swap", "streaming_swap", "outbound":
		return "swaps"
	case "bond", "rebond", "unbond", "leave", "slash", "rewards":
		return "bonds"
	default:
		return "transfers"
	}
}

func isLiquidityDepositEventType(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "add_liquidity", "rune_pool_deposit", "secured_asset_deposit", "trade_account_deposit":
		return true
	default:
		return false
	}
}

func isLiquidityWithdrawalEventType(eventType string) bool {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "withdraw", "withdraw_liquidity", "rune_pool_withdraw", "refund", "secured_asset_withdraw", "trade_account_withdraw":
		return true
	default:
		return false
	}
}

func midgardActionClass(action midgardAction) string {
	return describeMidgardAction(action).ActionClass
}

func midgardActionPool(action midgardAction) string {
	for _, candidate := range action.Pools {
		if pool := normalizeAsset(candidate); pool != "" {
			return pool
		}
	}
	var firstAsset string
	scan := func(legs []midgardActionLeg) string {
		for _, leg := range legs {
			for _, coin := range leg.Coins {
				asset := normalizeAsset(coin.Asset)
				if asset == "" {
					continue
				}
				if firstAsset == "" {
					firstAsset = asset
				}
				if asset != "THOR.RUNE" {
					return asset
				}
			}
		}
		return ""
	}
	if inferred := scan(action.In); inferred != "" {
		return inferred
	}
	if inferred := scan(action.Out); inferred != "" {
		return inferred
	}
	return firstAsset
}

func inferContractActionClass(action midgardAction) string {
	contract := action.Metadata.Contract
	if contract == nil {
		return "liquidity"
	}
	if _, _, ok := findContractSwapAmount(contract.Msg); ok {
		return "swaps"
	}

	if class := inferContractActionClassFromType(contract.ContractType); class != "" {
		return class
	}

	if _, _, ok := parseContractFunds(contract.Funds); ok {
		return "liquidity"
	}
	return "liquidity"
}

func splitMidgardContractLegs(action midgardAction) ([]midgardActionLeg, []midgardActionLeg) {
	inTxIDs := map[string]struct{}{}
	for _, leg := range action.In {
		txID := strings.ToUpper(strings.TrimSpace(leg.TxID))
		if txID != "" {
			inTxIDs[txID] = struct{}{}
		}
	}

	var receivers []midgardActionLeg
	var payouts []midgardActionLeg
	for _, outLeg := range action.Out {
		txID := strings.ToUpper(strings.TrimSpace(outLeg.TxID))
		if txID != "" && len(outLeg.Coins) == 0 {
			if len(inTxIDs) == 0 {
				receivers = append(receivers, outLeg)
				continue
			}
			if _, ok := inTxIDs[txID]; ok {
				receivers = append(receivers, outLeg)
				continue
			}
		}
		if len(outLeg.Coins) > 0 || txID == "" {
			payouts = append(payouts, outLeg)
		}
	}

	if len(receivers) == 0 {
		for _, outLeg := range action.Out {
			if strings.TrimSpace(outLeg.TxID) != "" && len(outLeg.Coins) == 0 {
				receivers = append(receivers, outLeg)
			}
		}
	}

	return receivers, payouts
}

func findContractExecutionAddress(msg map[string]any) string {
	if len(msg) == 0 {
		return ""
	}
	return findStringValueByKey(msg, "contract_address")
}

func findRepresentativeContractPayoutAddress(action midgardAction) string {
	if action.Metadata.Contract == nil || len(action.Metadata.Contract.Msg) == 0 {
		return ""
	}
	destinations := findContractDistributeBankAddresses(action.Metadata.Contract.Msg)
	if len(destinations) == 0 {
		return ""
	}
	inbound := map[string]struct{}{}
	for _, leg := range action.In {
		if addr := normalizeAddress(leg.Address); addr != "" {
			inbound[addr] = struct{}{}
		}
	}
	for _, addr := range destinations {
		norm := normalizeAddress(addr)
		if norm == "" {
			continue
		}
		if _, ok := inbound[norm]; ok {
			return norm
		}
	}
	return normalizeAddress(destinations[0])
}

func hasCalcStrategyExecuteMsgPayload(action midgardAction) bool {
	if action.Metadata.Contract == nil || len(action.Metadata.Contract.Msg) == 0 {
		return false
	}
	rawExecute, ok := action.Metadata.Contract.Msg["execute"]
	if !ok {
		return false
	}
	switch typed := rawExecute.(type) {
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}

func (b *graphBuilder) recordCalcRepresentativePayouts(actions []midgardAction) {
	if b == nil || len(actions) == 0 {
		return
	}
	if b.calcPayoutByContract == nil {
		b.calcPayoutByContract = map[string]string{}
	}
	for _, action := range actions {
		if action.Metadata.Contract == nil || !isCalcStrategyRepresentative(action.Metadata.Contract.ContractType) {
			continue
		}
		payoutAddress := findRepresentativeContractPayoutAddress(action)
		sourceHint := findContractExecutionAddress(action.Metadata.Contract.Msg)
		receiverLegs, _ := splitMidgardContractLegs(action)
		useExecutionReceiver := preferExecutionAddressAsContractReceiver(action.Metadata.Contract.ContractType)
		for _, receiverLeg := range receiverLegs {
			receiverAddress := receiverLeg.Address
			if useExecutionReceiver && sourceHint != "" {
				receiverAddress = sourceHint
			}
			receiverAddress = normalizeAddress(receiverAddress)
			if receiverAddress == "" {
				continue
			}
			if payoutAddress == "" {
				payoutAddress = knownCalcRepresentativePayoutAddress(receiverAddress)
			}
			if payoutAddress == "" {
				continue
			}
			b.calcPayoutByContract[receiverAddress] = payoutAddress
		}
	}
}

func knownCalcRepresentativePayoutAddress(contractAddress string) string {
	return normalizeAddress(knownCalcRepresentativePayouts[normalizeAddress(contractAddress)])
}

func findContractDistributeBankAddresses(value any) []string {
	var out []string
	switch typed := value.(type) {
	case map[string]any:
		if rawDistribute, ok := typed["distribute"]; ok {
			if distribute, ok := rawDistribute.(map[string]any); ok {
				if rawDestinations, ok := distribute["destinations"]; ok {
					if destinations, ok := rawDestinations.([]any); ok {
						for _, rawDestination := range destinations {
							destination, ok := rawDestination.(map[string]any)
							if !ok {
								continue
							}
							recipient, _ := destination["recipient"].(map[string]any)
							bank, _ := recipient["bank"].(map[string]any)
							if addr := normalizeAddress(stringifyAny(bank["address"])); addr != "" {
								out = appendUniqueString(out, addr)
							}
						}
					}
				}
			}
		}
		for _, child := range typed {
			for _, addr := range findContractDistributeBankAddresses(child) {
				out = appendUniqueString(out, addr)
			}
		}
	case []any:
		for _, child := range typed {
			for _, addr := range findContractDistributeBankAddresses(child) {
				out = appendUniqueString(out, addr)
			}
		}
	}
	return out
}

// isCalcStrategyRepresentative returns true for contract types that should
// serve as the single representative edge for a CALC strategy execution.
func isCalcStrategyRepresentative(contractType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contractType))
	return ct == "wasm-calc-manager/strategy.update" || ct == "wasm-calc-strategy/process"
}

func isCalcStrategyExecute(contractType string) bool {
	return strings.EqualFold(strings.TrimSpace(contractType), "wasm-calc-strategy/execute")
}

func isCalcStrategyProcessReply(contractType string) bool {
	return strings.EqualFold(strings.TrimSpace(contractType), "wasm-calc-strategy/process.reply")
}

func isCalcStrategyFallbackRepresentative(contractType string) bool {
	return isCalcStrategyExecute(contractType) || isCalcStrategyProcessReply(contractType)
}

// hasCalcStrategyMsgPayload returns true when the contract msg contains
// an update.nodes array, indicating this action is part of a CALC strategy
// execution pipeline.
func hasCalcStrategyMsgPayload(action midgardAction) bool {
	if action.Metadata.Contract == nil || len(action.Metadata.Contract.Msg) == 0 {
		return false
	}
	for _, key := range []string{"update", "instantiate"} {
		payloadRaw, ok := action.Metadata.Contract.Msg[key]
		if !ok {
			continue
		}
		payloadMap, ok := payloadRaw.(map[string]any)
		if !ok {
			continue
		}
		nodes, ok := payloadMap["nodes"]
		if !ok {
			continue
		}
		nodesSlice, ok := nodes.([]any)
		if ok && len(nodesSlice) > 0 {
			return true
		}
	}
	return false
}

// isSuppressedContractSubExecution returns true for contract call types whose
// individual execution legs should be hidden because a representative edge
// (wasm-calc-manager/strategy.update or wasm-calc-strategy/process) already
// represents the flow.
func isSuppressedContractSubExecution(action midgardAction) bool {
	if action.Metadata.Contract == nil {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(action.Metadata.Contract.ContractType))
	// Keep process as the preferred representative. execute / process.reply may
	// stand in only when the fetched slice is missing a same-tx process row.
	if strings.HasPrefix(ct, "wasm-calc-strategy/") {
		switch ct {
		case "wasm-calc-strategy/process":
			// representative
		case "wasm-calc-strategy/execute", "wasm-calc-strategy/process.reply":
			if hasCalcStrategyMsgPayload(action) {
				return true
			}
		default:
			return true
		}
	}
	if ct == "wasm-calc-manager/strategy.create" {
		return true
	}
	// Suppress any contract action carrying a CALC strategy msg payload
	// (update.nodes) that is not the representative type. This catches
	// ghost-vault, thorchain-swap, fin sub-executions that are internal
	// to the CALC strategy pipeline.
	if !isCalcStrategyRepresentative(ct) && hasCalcStrategyMsgPayload(action) {
		return true
	}
	return false
}

func preferInboundContractSource(contractType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(contractType))
	return strings.HasPrefix(normalized, "wasm-calc-manager/") ||
		normalized == "wasm-calc-strategy/process"
}

func preferExecutionAddressAsContractReceiver(contractType string) bool {
	return isCalcStrategyRepresentative(contractType)
}

func suppressContractPayoutProjection(contractType string) bool {
	return isCalcStrategyRepresentative(contractType)
}

func preferContractFundsAmount(contractType string) bool {
	return isCalcStrategyRepresentative(contractType)
}

func isCalcStrategyProcess(contractType string) bool {
	return strings.EqualFold(strings.TrimSpace(contractType), "wasm-calc-strategy/process")
}

func findStringValueByKey(value any, targetKey string) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(strings.TrimSpace(key), targetKey) {
				return stringifyAny(child)
			}
			if found := findStringValueByKey(child, targetKey); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findStringValueByKey(child, targetKey); found != "" {
				return found
			}
		}
	}
	return ""
}

func midgardGraphPagesForHop(hop int) int {
	if hop <= 0 {
		return midgardGraphPagesPerSeed
	}
	if hop == 1 {
		return midgardGraphPagesPerFirstHop
	}
	return midgardGraphPagesPerHop
}

func isMidgardRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if text == "" {
		return false
	}
	return strings.Contains(text, "status=429") ||
		strings.Contains(text, "too many requests") ||
		strings.Contains(text, "slow down cowboy")
}

func midgardActionTxIDs(action midgardAction) []string {
	out := make([]string, 0, len(action.In)+len(action.Out))
	for _, leg := range action.In {
		txID := cleanTxID(leg.TxID)
		if txID == "" {
			continue
		}
		out = appendUniqueString(out, txID)
	}
	for _, leg := range action.Out {
		txID := cleanTxID(leg.TxID)
		if txID == "" {
			continue
		}
		out = appendUniqueString(out, txID)
	}
	sort.Strings(out)
	return out
}

// collectCalcStrategyTxIDs returns txIDs associated with CALC rows that can
// stand in for the user-facing Treasury flow. Swap actions sharing these txIDs
// are internal to the CALC pipeline and should be suppressed.
func collectCalcStrategyTxIDs(actions []midgardAction) map[string]struct{} {
	if len(actions) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, action := range actions {
		if action.Metadata.Contract == nil {
			continue
		}
		ct := action.Metadata.Contract.ContractType
		if !isCalcStrategyRepresentative(ct) && !isCalcStrategyExecute(ct) {
			continue
		}
		for _, txID := range midgardActionTxIDs(action) {
			if txID == "" {
				continue
			}
			out[txID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectCalcStrategyProcessTxIDs(actions []midgardAction) map[string]struct{} {
	if len(actions) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, action := range actions {
		if action.Metadata.Contract == nil {
			continue
		}
		if !isCalcStrategyProcess(action.Metadata.Contract.ContractType) {
			continue
		}
		for _, txID := range midgardActionTxIDs(action) {
			if txID == "" {
				continue
			}
			out[txID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectMidgardSwapTxIDs(actions []midgardAction) map[string]struct{} {
	if len(actions) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, action := range actions {
		if strings.EqualFold(strings.TrimSpace(action.Status), "failed") {
			continue
		}
		if describeMidgardAction(action).ActionClass != "swaps" {
			continue
		}
		for _, txID := range midgardActionTxIDs(action) {
			if txID == "" {
				continue
			}
			out[txID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldSkipMidgardActionForGraph(action midgardAction, refundTxIDs map[string]struct{}, liquidityFeeTxIDs map[string]struct{}, calcStrategyTxIDs map[string]struct{}, calcStrategyProcessTxIDs map[string]struct{}) (bool, string) {
	if isMidgardRefundActionType(action.Type) {
		return true, "refund_action"
	}
	if isLiquidityFeeMidgardAction(action) {
		return true, "liquidity_fee_action"
	}
	if isSuppressedContractSubExecution(action) {
		return true, "contract_sub_execution"
	}
	if action.Metadata.Contract != nil &&
		strings.EqualFold(strings.TrimSpace(action.Metadata.Contract.ContractType), "wasm-calc-manager/strategy.update") {
		for _, txID := range midgardActionTxIDs(action) {
			if _, ok := calcStrategyProcessTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
		}
		if txID := midgardSwapCorrelationTxID(action, cleanTxID(midgardSyntheticTxID(action))); txID != "" {
			if _, ok := calcStrategyProcessTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
		}
	}
	if action.Metadata.Contract != nil && isCalcStrategyExecute(action.Metadata.Contract.ContractType) {
		for _, txID := range midgardActionTxIDs(action) {
			if _, ok := calcStrategyProcessTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
		}
		if txID := midgardSwapCorrelationTxID(action, cleanTxID(midgardSyntheticTxID(action))); txID != "" {
			if _, ok := calcStrategyProcessTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
		}
	}
	if action.Metadata.Contract != nil && isCalcStrategyProcessReply(action.Metadata.Contract.ContractType) {
		for _, txID := range midgardActionTxIDs(action) {
			if _, ok := calcStrategyProcessTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
			if _, ok := calcStrategyTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
		}
		if txID := midgardSwapCorrelationTxID(action, cleanTxID(midgardSyntheticTxID(action))); txID != "" {
			if _, ok := calcStrategyProcessTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
			if _, ok := calcStrategyTxIDs[txID]; ok {
				return true, "contract_sub_execution"
			}
		}
	}
	if len(refundTxIDs) == 0 && len(liquidityFeeTxIDs) == 0 && len(calcStrategyTxIDs) == 0 && len(calcStrategyProcessTxIDs) == 0 {
		return false, ""
	}
	isSwapAction := describeMidgardAction(action).ActionClass == "swaps"
	for _, txID := range midgardActionTxIDs(action) {
		if _, ok := refundTxIDs[txID]; ok {
			return true, "refund_associated"
		}
		if isSwapAction {
			if _, ok := liquidityFeeTxIDs[txID]; ok {
				return true, "liquidity_fee_associated"
			}
		}
		if isSwapAction {
			if _, ok := calcStrategyTxIDs[txID]; ok {
				return true, "calc_strategy_sub_swap"
			}
		}
	}
	if txID := midgardSwapCorrelationTxID(action, cleanTxID(midgardSyntheticTxID(action))); txID != "" {
		if _, ok := refundTxIDs[txID]; ok {
			return true, "refund_associated"
		}
		if isSwapAction {
			if _, ok := liquidityFeeTxIDs[txID]; ok {
				return true, "liquidity_fee_associated"
			}
		}
		if isSwapAction {
			if _, ok := calcStrategyTxIDs[txID]; ok {
				return true, "calc_strategy_sub_swap"
			}
		}
	}
	return false, ""
}

func shouldSkipExternalTransferForGraph(transfer externalTransfer, refundTxIDs map[string]struct{}, liquidityFeeTxIDs map[string]struct{}) (bool, string) {
	if len(refundTxIDs) == 0 && len(liquidityFeeTxIDs) == 0 {
		return false, ""
	}
	txID := cleanTxID(transfer.TxID)
	if txID == "" {
		return false, ""
	}
	if _, ok := refundTxIDs[txID]; ok {
		return true, "refund_associated"
	}
	if _, ok := liquidityFeeTxIDs[txID]; ok {
		return true, "liquidity_fee_associated"
	}
	return false, ""
}

func collectMidgardRefundTxIDs(actions []midgardAction) map[string]struct{} {
	if len(actions) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, action := range actions {
		if !isMidgardRefundActionType(action.Type) {
			continue
		}
		for _, txID := range midgardRefundCorrelationTxIDs(action) {
			if txID == "" {
				continue
			}
			out[txID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectMidgardLiquidityFeeTxIDs(actions []midgardAction) map[string]struct{} {
	if len(actions) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, action := range actions {
		if !isLiquidityFeeMidgardAction(action) {
			continue
		}
		for _, txID := range midgardLiquidityFeeCorrelationTxIDs(action) {
			if txID == "" {
				continue
			}
			out[txID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func midgardRefundCorrelationTxIDs(action midgardAction) []string {
	txIDs := midgardActionTxIDs(action)
	if len(txIDs) > 0 {
		return txIDs
	}
	fallback := midgardSwapCorrelationTxID(action, cleanTxID(midgardSyntheticTxID(action)))
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}

func midgardLiquidityFeeCorrelationTxIDs(action midgardAction) []string {
	txIDs := midgardActionTxIDs(action)
	if len(txIDs) > 0 {
		return txIDs
	}
	fallback := midgardSwapCorrelationTxID(action, cleanTxID(midgardSyntheticTxID(action)))
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}

func isMidgardRefundActionType(actionType string) bool {
	return strings.EqualFold(strings.TrimSpace(actionType), "refund")
}

func isLiquidityFeeMidgardAction(action midgardAction) bool {
	meta := describeMidgardAction(action)
	actionType := strings.ToLower(strings.TrimSpace(action.Type))
	actionKey := strings.ToLower(strings.TrimSpace(meta.ActionKey))
	actionLabel := strings.ToLower(strings.TrimSpace(meta.ActionLabel))
	if strings.Contains(actionType, "fee") {
		return true
	}
	if strings.Contains(actionKey, ".fee") || strings.HasSuffix(actionKey, "fee") || strings.Contains(actionKey, "affiliate_fee") {
		return true
	}
	return strings.Contains(actionLabel, "fee")
}

func mergeStringSet(dst, src map[string]struct{}) {
	if dst == nil || len(src) == 0 {
		return
	}
	for v := range src {
		dst[v] = struct{}{}
	}
}

func midgardActionKey(action midgardAction) string {
	txIDs := midgardActionTxIDs(action)
	if len(txIDs) == 0 && strings.TrimSpace(action.Date) == "" && strings.TrimSpace(action.Height) == "" {
		return ""
	}
	actionType := strings.ToLower(strings.TrimSpace(action.Type))
	// Contract actions with the same height/date/txIDs but different contract
	// types (e.g. wasm-calc-strategy/update vs wasm-calc-manager/strategy.update)
	// must produce distinct keys so suppressing one doesn't hide the other.
	contractType := ""
	if actionType == "contract" && action.Metadata.Contract != nil {
		contractType = strings.ToLower(strings.TrimSpace(action.Metadata.Contract.ContractType))
	}
	return strings.Join([]string{
		actionType,
		contractType,
		strings.TrimSpace(action.Height),
		strings.TrimSpace(action.Date),
		strings.Join(txIDs, ","),
	}, "|")
}

func midgardSyntheticTxID(action midgardAction) string {
	txIDs := midgardActionTxIDs(action)
	if len(txIDs) > 0 {
		return txIDs[0]
	}
	key := midgardActionKey(action)
	if key == "" {
		return "MIDGARD:UNKNOWN"
	}
	return "MIDGARD:" + strings.ToUpper(strings.ReplaceAll(key, "|", ":"))
}

func parseMidgardActionTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	n := parseInt64(raw)
	if n <= 0 {
		return time.Time{}
	}
	// Midgard action date is nanoseconds since epoch.
	if n > 1_000_000_000_000 {
		return time.Unix(0, n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

func chainFromMidgardCoins(coins []midgardActionCoin) string {
	for _, coin := range coins {
		if chain := chainFromAsset(coin.Asset); chain != "" {
			return chain
		}
	}
	return ""
}

func inferContractLegAmount(action midgardAction, inLeg, outLeg midgardActionLeg) (string, string, bool) {
	if !strings.EqualFold(strings.TrimSpace(action.Type), "contract") {
		return "", "", false
	}
	contract := action.Metadata.Contract
	if contract == nil {
		return "", "", false
	}
	if strings.TrimSpace(outLeg.TxID) != "" &&
		strings.TrimSpace(inLeg.TxID) != "" &&
		!strings.EqualFold(strings.TrimSpace(outLeg.TxID), strings.TrimSpace(inLeg.TxID)) {
		return "", "", false
	}
	if preferContractFundsAmount(contract.ContractType) {
		if asset, amount, ok := parseContractFunds(contract.Funds); ok {
			return asset, amount, true
		}
	}
	if asset, amount, ok := findContractSwapAmount(contract.Msg); ok {
		return asset, amount, true
	}
	if asset, amount, ok := parseContractFunds(contract.Funds); ok {
		return asset, amount, true
	}
	return "", "", false
}

func findContractSwapAmount(msg map[string]any) (string, string, bool) {
	if len(msg) == 0 {
		return "", "", false
	}
	return findSwapAmountInValue(msg)
}

func findSwapAmountInValue(value any) (string, string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if rawSwap, ok := typed["swap_amount"]; ok {
			if swapMap, ok := rawSwap.(map[string]any); ok {
				amount := stringifyAny(swapMap["amount"])
				denom := normalizeContractDenom(stringifyAny(swapMap["denom"]))
				if denom != "" && amount != "" {
					return denom, amount, true
				}
			}
		}
		for _, child := range typed {
			if asset, amount, ok := findSwapAmountInValue(child); ok {
				return asset, amount, true
			}
		}
	case []any:
		for _, child := range typed {
			if asset, amount, ok := findSwapAmountInValue(child); ok {
				return asset, amount, true
			}
		}
	}
	return "", "", false
}

func parseContractFunds(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := 0
		for idx < len(part) && part[idx] >= '0' && part[idx] <= '9' {
			idx++
		}
		if idx == 0 || idx >= len(part) {
			continue
		}
		amount := strings.TrimSpace(part[:idx])
		denom := normalizeContractDenom(part[idx:])
		if amount == "" || denom == "" {
			continue
		}
		return denom, amount, true
	}
	return "", "", false
}

func normalizeContractDenom(denom string) string {
	denom = strings.TrimSpace(denom)
	if denom == "" {
		return ""
	}
	return normalizeTHORDenomAsset(denom)
}

func normalizeTHORDenomAsset(denom string) string {
	denom = strings.TrimSpace(denom)
	if denom == "" {
		return ""
	}
	lower := strings.ToLower(denom)
	switch lower {
	case "rune":
		return "THOR.RUNE"
	case "tcy":
		return "THOR.TCY"
	}
	if strings.HasPrefix(lower, "x/") {
		symbol := strings.TrimSpace(denom[2:])
		if symbol == "" {
			return ""
		}
		if asset := normalizeAsset(symbol); strings.Contains(asset, ".") {
			return asset
		}
		return "THOR." + strings.ToUpper(symbol)
	}
	asset := normalizeAsset(denom)
	if strings.Contains(asset, ".") {
		return asset
	}
	return "THOR." + strings.ToUpper(denom)
}

func stringifyAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func parseCoinLikeValue(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	parts := strings.Fields(raw)
	if len(parts) >= 2 {
		return normalizeAsset(parts[1]), parts[0]
	}
	if strings.Contains(raw, ".") {
		return normalizeAsset(raw), ""
	}
	return "", raw
}

func parseMemoDestination(memo string) string {
	memo = strings.TrimSpace(memo)
	if memo == "" {
		return ""
	}
	parts := strings.Split(memo, ":")
	if len(parts) < 3 {
		return ""
	}
	candidate := strings.TrimSpace(parts[2])
	lower := strings.ToLower(candidate)
	if isLikelyEVMAddress(candidate) ||
		strings.HasPrefix(lower, "thor") ||
		strings.HasPrefix(lower, "bc1") ||
		strings.HasPrefix(lower, "ltc") ||
		strings.HasPrefix(lower, "bitcoincash:") ||
		strings.HasPrefix(candidate, "T") ||
		strings.HasPrefix(candidate, "D") {
		return candidate
	}
	return ""
}

func parseBondMemoNodeAddress(memo string) string {
	memo = strings.TrimSpace(memo)
	if memo == "" {
		return ""
	}
	parts := strings.Split(memo, ":")
	if len(parts) < 2 {
		return ""
	}
	action := strings.ToUpper(strings.TrimSpace(parts[0]))
	switch {
	case strings.HasPrefix(action, "BOND"),
		strings.HasPrefix(action, "UNBOND"),
		strings.HasPrefix(action, "REBOND"),
		strings.HasPrefix(action, "LEAVE"):
	default:
		return ""
	}
	candidate := normalizeAddress(strings.TrimSpace(parts[1]))
	if candidate == "" || !strings.HasPrefix(candidate, "thor") {
		return ""
	}
	return candidate
}

func extractTxMemo(details map[string]any) string {
	if len(details) == 0 {
		return ""
	}
	if memo := strings.TrimSpace(getString(details, "memo")); memo != "" {
		return memo
	}
	if txValue, ok := details["tx"].(map[string]any); ok {
		if memo := strings.TrimSpace(getString(txValue, "memo")); memo != "" {
			return memo
		}
		if memo := strings.TrimSpace(findStringValueByKey(txValue, "memo")); memo != "" {
			return memo
		}
	}
	if statusValue, ok := details["status_only"].(map[string]any); ok {
		if memo := strings.TrimSpace(getString(statusValue, "memo")); memo != "" {
			return memo
		}
		if txValue, ok := statusValue["tx"].(map[string]any); ok {
			if memo := strings.TrimSpace(getString(txValue, "memo")); memo != "" {
				return memo
			}
			if memo := strings.TrimSpace(findStringValueByKey(txValue, "memo")); memo != "" {
				return memo
			}
		}
		if memo := strings.TrimSpace(findStringValueByKey(statusValue, "memo")); memo != "" {
			return memo
		}
	}
	return strings.TrimSpace(findStringValueByKey(details, "memo"))
}

func parseOutboundMemoTxID(memo string) string {
	memo = strings.TrimSpace(memo)
	if memo == "" {
		return ""
	}
	upper := strings.ToUpper(memo)
	if !strings.HasPrefix(upper, "OUT:") {
		return ""
	}
	txID := strings.TrimSpace(memo[4:])
	return cleanTxID(txID)
}

func cleanTxID(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" || isZeroTxID(raw) {
		return ""
	}
	return raw
}

func isZeroTxID(txID string) bool {
	txID = strings.TrimSpace(txID)
	if txID == "" {
		return false
	}
	for _, r := range txID {
		if r != '0' {
			return false
		}
	}
	return true
}

func firstNonAsgardAddress(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if isAsgardModuleAddress(value) {
			continue
		}
		return value
	}
	return ""
}

func midgardSwapCorrelationTxID(action midgardAction, fallback string) string {
	for _, leg := range action.In {
		if txID := cleanTxID(leg.TxID); txID != "" {
			return txID
		}
	}
	for _, leg := range action.Out {
		if txID := cleanTxID(leg.TxID); txID != "" {
			return txID
		}
	}
	return cleanTxID(fallback)
}

func canonicalSwapSegmentKey(txID, source, target, asset string) string {
	txID = cleanTxID(txID)
	source = normalizeAddress(source)
	target = normalizeAddress(target)
	asset = normalizeAsset(asset)
	if txID == "" || source == "" || target == "" || source == target {
		return ""
	}
	if asset == "" {
		asset = "THOR.RUNE"
	}
	return strings.Join([]string{"swap", txID, source, target, asset}, "|")
}

func normalizeAsset(asset string) string {
	asset = strings.ToUpper(strings.TrimSpace(asset))
	if asset == "" || strings.Contains(asset, ".") {
		return asset
	}
	parts := strings.SplitN(asset, "-", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + "." + parts[1]
	}
	return asset
}

func normalizeChain(chain, address string) string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	address = strings.TrimSpace(address)
	lower := strings.ToLower(address)
	inferred := ""
	switch {
	case strings.HasPrefix(lower, "thor"):
		inferred = "THOR"
	case isLikelyEVMAddress(address):
		inferred = "ETH"
	case strings.HasPrefix(lower, "bc1"):
		inferred = "BTC"
	case strings.HasPrefix(lower, "ltc1"):
		inferred = "LTC"
	case strings.HasPrefix(lower, "bitcoincash:"):
		inferred = "BCH"
	case strings.HasPrefix(lower, "cosmos1"):
		inferred = "GAIA"
	case strings.HasPrefix(address, "T"):
		inferred = "TRON"
	case strings.HasPrefix(address, "r"):
		inferred = "XRP"
	}
	if chain == "" {
		return inferred
	}
	// Preserve explicit hints for EVM addresses since the same 0x address can
	// legitimately exist on multiple chains (ETH/BASE/BSC/AVAX, etc).
	if isLikelyEVMAddress(address) {
		return chain
	}
	// For non-EVM address formats, prefer the prefix-derived chain over any
	// conflicting hint to avoid impossible tracker/provider pairings.
	if inferred != "" && inferred != chain {
		return inferred
	}
	return chain
}

func chainFromAsset(asset string) string {
	asset = normalizeAsset(asset)
	if idx := strings.Index(asset, "."); idx > 0 {
		return asset[:idx]
	}
	return ""
}

func poolDisplayLabel(pool string) string {
	pool = normalizeAsset(pool)
	if pool == "" {
		return "Pool"
	}
	return "Pool " + pool
}

func isStableAsset(asset string) bool {
	asset = normalizeAsset(asset)
	stableMarkers := []string{"USDT", "USDC", "DAI", "USDE", "FDUSD", "USDX", "USDQ", "BUSD"}
	for _, marker := range stableMarkers {
		if strings.Contains(asset, marker) {
			return true
		}
	}
	return false
}

func assetMetadataFromFlowAssetValue(asset FlowAssetValue) assetMetadata {
	return assetMetadata{
		AssetKind:     asset.AssetKind,
		TokenStandard: asset.TokenStandard,
		TokenAddress:  asset.TokenAddress,
		TokenSymbol:   asset.TokenSymbol,
		TokenName:     asset.TokenName,
		TokenDecimals: asset.TokenDecimals,
	}
}

func mergeAssetValues(values *[]FlowAssetValue, asset, amountRaw string, usd float64, meta assetMetadata, direction string) {
	asset = normalizeAsset(asset)
	if asset == "" {
		asset = "THOR.RUNE"
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	switch direction {
	case "in", "out":
	default:
		direction = ""
	}
	meta = mergeAssetMetadata(meta, assetMetadataFromAsset(asset))
	for i := range *values {
		if (*values)[i].Asset == asset && (*values)[i].Direction == direction {
			(*values)[i].AmountRaw = addRawAmounts((*values)[i].AmountRaw, amountRaw)
			(*values)[i].USDSpot += usd
			if (*values)[i].AssetKind == "" {
				(*values)[i].AssetKind = meta.AssetKind
			}
			if (*values)[i].TokenStandard == "" {
				(*values)[i].TokenStandard = meta.TokenStandard
			}
			if (*values)[i].TokenAddress == "" {
				(*values)[i].TokenAddress = meta.TokenAddress
			}
			if (*values)[i].TokenSymbol == "" {
				(*values)[i].TokenSymbol = meta.TokenSymbol
			}
			if (*values)[i].TokenName == "" {
				(*values)[i].TokenName = meta.TokenName
			}
			if (*values)[i].TokenDecimals == 0 {
				(*values)[i].TokenDecimals = meta.TokenDecimals
			}
			return
		}
	}
	*values = append(*values, FlowAssetValue{
		Asset:         asset,
		AmountRaw:     firstNonEmpty(amountRaw, "0"),
		USDSpot:       usd,
		Direction:     direction,
		AssetKind:     meta.AssetKind,
		TokenStandard: meta.TokenStandard,
		TokenAddress:  meta.TokenAddress,
		TokenSymbol:   meta.TokenSymbol,
		TokenName:     meta.TokenName,
		TokenDecimals: meta.TokenDecimals,
	})
}

func mergeEdgeAsset(edge *FlowEdge, asset, amountRaw string, usd float64, meta assetMetadata, direction string) {
	mergeAssetValues(&edge.Assets, asset, amountRaw, usd, meta, direction)
}

func mergeEdgeTransactionAsset(edge *FlowEdge, txID string, height int64, when time.Time, asset, amountRaw string, usd float64, meta assetMetadata, direction string) {
	txID = strings.TrimSpace(txID)
	for i := range edge.Transactions {
		tx := &edge.Transactions[i]
		if tx.TxID != txID {
			continue
		}
		if tx.Height == 0 || (height > 0 && height < tx.Height) {
			tx.Height = height
		}
		if tx.Time.IsZero() || (!when.IsZero() && when.Before(tx.Time)) {
			tx.Time = when
		}
		tx.USDSpot += usd
		mergeAssetValues(&tx.Assets, asset, amountRaw, usd, meta, direction)
		return
	}
	tx := FlowEdgeTransaction{
		TxID:    txID,
		Height:  height,
		Time:    when,
		USDSpot: usd,
		Assets:  []FlowAssetValue{},
	}
	mergeAssetValues(&tx.Assets, asset, amountRaw, usd, meta, direction)
	edge.Transactions = append(edge.Transactions, tx)
}

func recomputeEdgeAggregate(edge *FlowEdge) {
	if edge == nil {
		return
	}
	edge.Assets = nil
	edge.USDSpot = 0
	edge.TxIDs = nil
	edge.Heights = nil
	for _, tx := range edge.Transactions {
		edge.USDSpot += tx.USDSpot
		edge.TxIDs = appendUniqueString(edge.TxIDs, tx.TxID)
		edge.Heights = appendUniqueInt64(edge.Heights, tx.Height)
		for _, asset := range tx.Assets {
			mergeEdgeAsset(edge, asset.Asset, asset.AmountRaw, asset.USDSpot, assetMetadataFromFlowAssetValue(asset), asset.Direction)
		}
	}
}

func addRawAmounts(a, b string) string {
	total := parseInt64(a) + parseInt64(b)
	return strconv.FormatInt(total, 10)
}

func appendUniqueString(in []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return in
	}
	for _, item := range in {
		if item == v {
			return in
		}
	}
	return append(in, v)
}

func appendUniqueInt64(in []int64, v int64) []int64 {
	for _, item := range in {
		if item == v {
			return in
		}
	}
	return append(in, v)
}

func mergeInt64s(a, b []int64) []int64 {
	out := append([]int64{}, a...)
	for _, item := range b {
		out = appendUniqueInt64(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func uniqueInt64s(in []int64) []int64 {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(i, j int) bool { return in[i] < in[j] })
	out := in[:1]
	for _, item := range in[1:] {
		if item != out[len(out)-1] {
			out = append(out, item)
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func uniqueFrontierAddresses(in []frontierAddress) []frontierAddress {
	if len(in) < 2 {
		return in
	}
	seen := map[string]struct{}{}
	out := make([]frontierAddress, 0, len(in))
	for _, item := range in {
		if item.Address == "" {
			continue
		}
		key := frontierKey(item.Chain, item.Address)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item.Address = normalizeAddress(item.Address)
		item.Chain = normalizeChain(item.Chain, item.Address)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return frontierKey(out[i].Chain, out[i].Address) < frontierKey(out[j].Chain, out[j].Address)
	})
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func hasGraphableLiquidity(amount string) bool {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return false
	}
	if parseInt64(amount) > 0 {
		return true
	}
	return strings.Trim(amount, "0") != ""
}

func shouldExpandAddressRef(ref flowRef) bool {
	if ref.Address == "" {
		return false
	}
	if _, blocked := frontierBlacklist[normalizeAddress(ref.Address)]; blocked {
		return false
	}
	switch ref.Kind {
	case "contract_address", "external_address", "actor_address", "bond_address":
		return true
	default:
		return false
	}
}

func isAsgardModuleAddress(address string) bool {
	return normalizeAddress(address) == asgardModuleAddress
}

func isBondModuleAddress(address string) bool {
	return normalizeAddress(address) == bondModuleAddress
}

func shortAddress(address string) string {
	address = strings.TrimSpace(address)
	if len(address) <= 14 {
		return address
	}
	return address[:8] + "…" + address[len(address)-6:]
}

func isRebondActionKey(actionKey string) bool {
	key := strings.ToLower(strings.TrimSpace(actionKey))
	return strings.HasSuffix(key, ".rebond") || strings.Contains(key, "rebond")
}

func hasActorIDs(ids []int64) bool {
	return len(ids) > 0
}

func intMetric(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}
