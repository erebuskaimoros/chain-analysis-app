package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
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
// expansion unlike frontierBlacklist entries. They come from the built-in
// TagPack (labels/builtin.yaml).
var knownAddressLabels = builtinLabelMap()

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
	NativeUSD     map[string]float64
	AssetUSD      map[string]float64
	PoolAssets    map[string]struct{}
	PoolSnapshots map[string]MidgardPool
	PoolProtocols map[string]string
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
	thorTxTransfersByTx  map[string][]thorTxTransfer
	midgardActionsByTx   map[string][]midgardAction
	recordedActionKeys   map[string]struct{}
	allowedFlowTypes     map[string]bool
	minUSD               float64
	includeUnpriced      bool
	history              *priceHistory
	nodes                map[string]*FlowNode
	edges                map[string]*FlowEdge
	actions              map[string]*SupportingAction
	warnings             []string
	seenCanonicalKey     map[string]struct{}
	sourceProtocols      map[string]struct{}
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
	Address string
	Chain   string
	Hop     int
	Depth   int
}

type frontierCandidate struct {
	address  string
	chain    string
	totalUSD float64
	depth    int
}

type frontierAddress struct {
	Address string
	Chain   string
	Depth   int
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
	SourceProtocol   string
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
	// USDAtTime values the segment at its transaction time; Priced is false
	// when no price is known, and PriceSource names where the price came from.
	USDAtTime    float64
	Priced       bool
	PriceSource  string
	TxID         string
	Height       int64
	Time         time.Time
	Confidence   float64
	ActorIDs     []int64
	CanonicalKey string
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
	Date           string                `json:"date"`
	Height         string                `json:"height"`
	Type           string                `json:"type"`
	Status         string                `json:"status"`
	In             []midgardActionLeg    `json:"in"`
	Out            []midgardActionLeg    `json:"out"`
	Pools          []string              `json:"pools"`
	Metadata       midgardActionMetadata `json:"metadata"`
	SourceProtocol string                `json:"-"`
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
		thorTxTransfersByTx:  map[string][]thorTxTransfer{},
		midgardActionsByTx:   map[string][]midgardAction{},
		recordedActionKeys:   map[string]struct{}{},
		allowedFlowTypes:     flowTypeSet(query.FlowTypes),
		minUSD:               query.MinUSD,
		includeUnpriced:      query.IncludeUnpriced,
		history:              newPriceHistory(),
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
		warnings:             append([]string{}, coverageWarnings...),
		seenCanonicalKey:     map[string]struct{}{},
		sourceProtocols:      map[string]struct{}{},
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

	maxNodeDepth := maxGraphNodeDepth(query.MaxHops)
	queue := make([]queueItem, 0, len(seedAddresses))
	queued := map[string]int{}
	for _, seed := range seedAddresses {
		norm := normalizeFrontierAddress(encodeFrontierAddress(seed))
		if norm.Address == "" {
			builder.warnings = append(builder.warnings, fmt.Sprintf("skipped invalid seed address %s", shortAddress(seed.Address)))
			continue
		}
		queue = append(queue, queueItem{Address: norm.Address, Chain: norm.Chain, Hop: 0, Depth: 1})
		queued[frontierKey(norm.Chain, norm.Address)] = 0
	}

	actionLedger := newMidgardActionLedger()
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
		publishJobPartial(ctx, func() (any, map[string]int) {
			nodes, edges := builder.nodeList(), builder.edgeList()
			return ActorTrackerResponse{Query: query, Actors: actors, Warnings: uniqueStrings(builder.warnings), Nodes: nodes, Edges: edges},
				map[string]int{"nodes": len(nodes), "edges": len(edges)}
		})
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

		expandableWave := make([]queueItem, 0, len(wave))
		for _, item := range wave {
			if item.Depth < maxNodeDepth {
				expandableWave = append(expandableWave, item)
			}
		}
		if len(expandableWave) == 0 {
			continue
		}

		// Prefetch Midgard actions for the entire wave concurrently.
		if !midgardRateLimited && midgardFetchCount < midgardGraphMaxFetches {
			a.prefetchMidgardBatch(ctx, expandableWave, query.StartTime, query.EndTime, currentHop,
				midgardActionCache, midgardActionTruncated, &midgardFetchCount, &midgardRateLimited,
				midgardGraphMaxFetches, builder)
		}
		for _, item := range expandableWave {
			mergeStringSet(midgardSwapTxIDs, collectMidgardSwapTxIDs(midgardActionCache[item.Address]))
		}

		// Collect next-hop candidates with cumulative USD values so we can
		// prioritise high-value connections and cap the frontier size.
		nextCandidatesByHop := map[int]map[string]*frontierCandidate{}

		collectCandidate := func(addr frontierAddress, usd float64) {
			norm := normalizeFrontierAddress(encodeFrontierAddress(addr))
			if norm.Address == "" {
				return
			}
			candidateDepth := max(1, addr.Depth)
			candidateHop := max(0, candidateDepth-1)
			key := frontierKey(norm.Chain, norm.Address)
			if prevHop, ok := queued[key]; ok && prevHop <= candidateHop {
				return
			}
			bucket := nextCandidatesByHop[candidateHop]
			if bucket == nil {
				bucket = map[string]*frontierCandidate{}
				nextCandidatesByHop[candidateHop] = bucket
			}
			if c, exists := bucket[key]; exists {
				c.totalUSD += usd
				if candidateDepth < c.depth {
					c.depth = candidateDepth
				}
			} else {
				bucket[key] = &frontierCandidate{
					address:  norm.Address,
					chain:    norm.Chain,
					totalUSD: usd,
					depth:    candidateDepth,
				}
			}
		}

		for _, item := range expandableWave {
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
			builder.recordMidgardActions(actions)
			a.prefetchThorTxTransfers(ctx, actions, builder)
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
			a.preloadPriceHistory(ctx, builder.history, prices, priceNeedsForFlows(actions, externalTransfers))
			consumedExternalTransfers := map[string]struct{}{}
			// Midgard action-level movements provide address-to-address liquidity paths.
			for _, action := range actions {
				key := midgardActionKey(action)
				if key == "" {
					continue
				}
				if actionLedger.isSuppressed(key) {
					_, consumed := builder.stitchMidgardAction(action, externalTransfers)
					mergeStringSet(consumedExternalTransfers, consumed)
					continue
				}
				var segments []projectedSegment
				var warnings []string
				stitched, projected := actionLedger.projectedAction(key)
				if projected {
					// Another traced address already emitted its part of this
					// action. Re-project the same stitched form for this
					// frontier; the ledger drops segments already in the graph.
					_, consumed := builder.stitchMidgardAction(action, externalTransfers)
					mergeStringSet(consumedExternalTransfers, consumed)
					if shouldSkipMidgardActionForFeeOnlyFrontier(stitched, item.Address) {
						continue
					}
					segments, warnings = builder.reprojectMidgardAction(stitched, item.Depth)
				} else {
					if skip, reason := shouldSkipMidgardActionForGraph(action, refundTxIDs, liquidityFeeTxIDs, calcStrategyTxIDs, calcStrategyProcessTxIDs); skip {
						switch reason {
						case "liquidity_fee_action", "liquidity_fee_associated":
							builder.feeActionDrop++
						case "contract_sub_execution":
							builder.contractSubDrop++
						default:
							builder.refundActionDrop++
						}
						actionLedger.suppress(key)
						continue
					}
					if builder.shouldSkipActionBecauseRujiraTrace(action) {
						builder.contractSubDrop++
						actionLedger.suppress(key)
						continue
					}
					if shouldSkipMidgardActionForFeeOnlyFrontier(action, item.Address) {
						builder.swapSuppressed++
						continue
					}
					var consumed map[string]struct{}
					stitched, consumed = builder.stitchMidgardAction(action, externalTransfers)
					mergeStringSet(consumedExternalTransfers, consumed)
					segments, _, warnings = builder.projectMidgardAction(stitched, item.Depth)
				}
				segments, _ = filterProjectedSegmentsToMaxDepth(segments, maxNodeDepth)
				segments, nextAddresses := filterProjectedSegmentsToFrontierStep(segments, frontierAddress{
					Address: item.Address,
					Chain:   item.Chain,
				}, item.Depth+1, maxNodeDepth)
				builder.warnings = append(builder.warnings, warnings...)
				segments = actionLedger.claim(key, stitched, segments)
				if len(segments) == 0 {
					continue
				}
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
					collectCandidate(next, segUSD)
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
				segments, _ := builder.projectExternalTransfer(transfer, item.Depth)
				segments, _ = filterProjectedSegmentsToMaxDepth(segments, maxNodeDepth)
				segments, nextAddresses := filterProjectedSegmentsToFrontierStep(segments, frontierAddress{
					Address: item.Address,
					Chain:   item.Chain,
				}, item.Depth+1, maxNodeDepth)
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
					collectCandidate(next, segUSD)
				}
			}
		}

		// Sort candidates by total USD flow (descending) and cap frontier.
		if len(nextCandidatesByHop) > 0 {
			hops := make([]int, 0, len(nextCandidatesByHop))
			for hop := range nextCandidatesByHop {
				hops = append(hops, hop)
			}
			sort.Ints(hops)
			for _, hop := range hops {
				bucket := nextCandidatesByHop[hop]
				sorted := make([]*frontierCandidate, 0, len(bucket))
				for _, c := range bucket {
					sorted = append(sorted, c)
				}
				sort.Slice(sorted, func(i, j int) bool {
					return sorted[i].totalUSD > sorted[j].totalUSD
				})
				if len(sorted) > maxFrontierPerHop {
					builder.warnings = append(builder.warnings, fmt.Sprintf(
						"hop %d frontier capped from %d to %d addresses (by USD flow priority)",
						hop, len(sorted), maxFrontierPerHop))
					sorted = sorted[:maxFrontierPerHop]
				}
				for _, c := range sorted {
					queued[frontierKey(c.chain, c.address)] = hop
					queue = append(queue, queueItem{
						Address: c.address,
						Chain:   c.chain,
						Hop:     hop,
						Depth:   c.depth,
					})
				}
			}
		}
	}

	nodes := builder.nodeList()
	a.applyAddressLabels(ctx, nodes)
	builder.warnings = append(builder.warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, builder.protocols, false)...)
	builder.applyNodeLabelsToValidatorMetadata(nodes)
	edges := builder.edgeList()
	actions := builder.actionList()
	builder.applyAtTimeValues(edges, actions)

	stats := map[string]any{
		"actor_count":                        len(actors),
		"node_count":                         len(nodes),
		"edge_count":                         len(edges),
		"supporting_action_count":            len(actions),
		"source_protocol_count":              len(builder.sourceProtocolList()),
		"source_protocols":                   builder.sourceProtocolList(),
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
		IncludeUnpriced:  req.IncludeUnpriced,
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
		includeUnpriced:      query.IncludeUnpriced,
		history:              newPriceHistory(),
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
		warnings:             append(append([]string{}, coverageWarnings...), warnings...),
		seenCanonicalKey:     map[string]struct{}{},
		sourceProtocols:      map[string]struct{}{},
	}
	if priceErr != nil {
		builder.warnings = append(builder.warnings, "spot USD normalization unavailable; falling back to asset-native values")
	}

	actionLedger := newMidgardActionLedger()
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
		builder.recordMidgardActions(actions)
		a.prefetchThorTxTransfers(ctx, actions, builder)
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
		a.preloadPriceHistory(ctx, builder.history, prices, priceNeedsForFlows(actions, externalTransfers))
		consumedExternalTransfers := map[string]struct{}{}
		for _, action := range actions {
			key := midgardActionKey(action)
			if key == "" {
				continue
			}
			if actionLedger.isSuppressed(key) {
				_, consumed := builder.stitchMidgardAction(action, externalTransfers)
				mergeStringSet(consumedExternalTransfers, consumed)
				continue
			}
			var segments []projectedSegment
			var segmentWarnings []string
			stitched, projected := actionLedger.projectedAction(key)
			if projected {
				// Another expanded address already emitted its part of this
				// action; re-project the same stitched form for this address.
				_, consumed := builder.stitchMidgardAction(action, externalTransfers)
				mergeStringSet(consumedExternalTransfers, consumed)
				if shouldSkipMidgardActionForFeeOnlyFrontier(stitched, address) {
					continue
				}
				segments, segmentWarnings = builder.reprojectMidgardAction(stitched, 1)
			} else {
				if skip, reason := shouldSkipMidgardActionForGraph(action, refundTxIDs, liquidityFeeTxIDs, calcStrategyTxIDs, calcStrategyProcessTxIDs); skip {
					switch reason {
					case "liquidity_fee_action", "liquidity_fee_associated":
						builder.feeActionDrop++
					case "contract_sub_execution", "calc_strategy_sub_swap":
						builder.contractSubDrop++
					default:
						builder.refundActionDrop++
					}
					actionLedger.suppress(key)
					continue
				}
				if builder.shouldSkipActionBecauseRujiraTrace(action) {
					builder.contractSubDrop++
					actionLedger.suppress(key)
					continue
				}
				if shouldSkipMidgardActionForFeeOnlyFrontier(action, address) {
					builder.swapSuppressed++
					continue
				}
				var consumed map[string]struct{}
				stitched, consumed = builder.stitchMidgardAction(action, externalTransfers)
				mergeStringSet(consumedExternalTransfers, consumed)
				segments, _, segmentWarnings = builder.projectMidgardAction(stitched, 1)
			}
			segments, _ = filterProjectedSegmentsToMaxDepth(segments, maxGraphNodeDepth(query.MaxHops))
			segments, _ = filterProjectedSegmentsToFrontierStep(segments, frontierAddress{
				Address: address,
				Chain:   seed.Chain,
			}, 2, maxGraphNodeDepth(query.MaxHops))
			builder.warnings = append(builder.warnings, segmentWarnings...)
			segments = actionLedger.claim(key, stitched, segments)
			if len(segments) == 0 {
				continue
			}
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
			segments, _ = filterProjectedSegmentsToMaxDepth(segments, maxGraphNodeDepth(query.MaxHops))
			segments, _ = filterProjectedSegmentsToFrontierStep(segments, frontierAddress{
				Address: address,
				Chain:   seed.Chain,
			}, 2, maxGraphNodeDepth(query.MaxHops))
			for _, segment := range segments {
				builder.addProjectedSegment(segment)
			}
		}
	}

	nodes := builder.nodeList()
	a.applyAddressLabels(ctx, nodes)
	builder.warnings = append(builder.warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, builder.protocols, false)...)
	builder.applyNodeLabelsToValidatorMetadata(nodes)
	edges := builder.edgeList()
	actions := builder.actionList()
	builder.applyAtTimeValues(edges, actions)
	stats := map[string]any{
		"actor_count":                        len(actors),
		"node_count":                         len(nodes),
		"edge_count":                         len(edges),
		"supporting_action_count":            len(actions),
		"source_protocol_count":              len(builder.sourceProtocolList()),
		"source_protocols":                   builder.sourceProtocolList(),
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
		parsed, err := parseActorTrackerTime(strings.TrimSpace(req.EndTime))
		if err != nil {
			return ActorTrackerQuery{}, fmt.Errorf("invalid end_time: %w", err)
		}
		end = parsed.UTC()
	}

	start := end.Add(-7 * 24 * time.Hour)
	if strings.TrimSpace(req.StartTime) != "" {
		parsed, err := parseActorTrackerTime(strings.TrimSpace(req.StartTime))
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
		IncludeUnpriced:  req.IncludeUnpriced,
		CollapseExternal: req.CollapseExternal,
		DisplayMode:      displayMode,
		RequestedAt:      now,
	}, nil
}

func parseActorTrackerTime(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}

	for _, layout := range []string{
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999",
	} {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("expected RFC3339 or local datetime, got %q", raw)
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
