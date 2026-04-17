package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	explorerDefaultBatchSize = 10 // 10 pages × 50 = 500 actions
	explorerMaxBatchSize     = 20
	explorerModePreview      = "preview"
	explorerModeGraph        = "graph"
)

var explorerCompatibleEVMChains = []string{"ETH", "ARB", "BSC", "BASE", "AVAX"}

type explorerPreparedRequest struct {
	rawAddress string
	address    string
	mode       string
	direction  string
	batchSize  int
	offset     int
	query      AddressExplorerQuery
	start      time.Time
	end        time.Time
	protocols  protocolDirectory
	seeds      []frontierAddress
}

type explorerSeedActivity struct {
	Seed                 frontierAddress
	MidgardActionCount   int
	ExternalTransfers    []externalTransfer
	ExternalWarning      string
	ExternalError        error
	ExternalTransferSeen bool
	Active               bool
}

func (a *App) buildAddressExplorer(ctx context.Context, req AddressExplorerRequest) (AddressExplorerResponse, error) {
	started := time.Now()

	protocols, err := a.loadProtocolDirectory(ctx)
	if err != nil {
		return AddressExplorerResponse{}, err
	}

	prepared, err := normalizeAddressExplorerRequest(req, protocols)
	if err != nil {
		return AddressExplorerResponse{}, err
	}

	logInfo(ctx, "address_explorer_started", map[string]any{
		"address":    prepared.address,
		"mode":       prepared.mode,
		"direction":  prepared.direction,
		"offset":     prepared.offset,
		"batch_size": prepared.batchSize,
	})

	actions, hasMore, nextOffset, totalEstimate, err := a.fetchAddressExplorerActions(
		ctx,
		prepared.address,
		prepared.direction,
		prepared.offset,
		prepared.batchSize,
		prepared.start,
		prepared.end,
	)
	if err != nil {
		return AddressExplorerResponse{}, fmt.Errorf("failed to fetch actions: %w", err)
	}

	transferPages := prepared.batchSize
	if prepared.mode == explorerModePreview {
		transferPages = 1
	}
	seedActivity, activityWarnings := a.inspectAddressExplorerSeeds(ctx, prepared, actions, transferPages)
	activeChains := explorerActiveChains(seedActivity)
	runLabel := explorerRunLabel(prepared.address, activeChains, firstNonEmpty(prepared.direction, "newest"))

	response := AddressExplorerResponse{
		Mode:              prepared.mode,
		RawAddress:        prepared.rawAddress,
		Address:           prepared.address,
		Query:             prepared.query,
		Stats:             map[string]any{},
		Warnings:          uniqueStrings(activityWarnings),
		Nodes:             []FlowNode{},
		Edges:             []FlowEdge{},
		SupportingActions: []SupportingAction{},
		LoadedActions:     0,
		HasMore:           hasMore,
		NextOffset:        nextOffset,
		TotalEstimate:     totalEstimate,
		DirectionRequired: prepared.mode == explorerModePreview && hasMore,
		ActiveChains:      activeChains,
		SeedSummaries:     explorerSeedSummaries(seedActivity),
		RunLabel:          runLabel,
	}

	if prepared.mode == explorerModePreview {
		response.Stats = map[string]any{
			"active_chain_count":   len(activeChains),
			"candidate_seed_count": len(seedActivity),
			"elapsed_ms":           time.Since(started).Milliseconds(),
			"has_more":             hasMore,
		}
		logInfo(ctx, "address_explorer_preview_completed", map[string]any{
			"address":         prepared.address,
			"active_chains":   activeChains,
			"has_more":        hasMore,
			"elapsed_ms":      time.Since(started).Milliseconds(),
			"candidate_seeds": len(seedActivity),
		})
		return response, nil
	}

	prices, priceErr := a.buildPriceBook(ctx)
	builder := &graphBuilder{
		ownerMap:             map[string][]int64{},
		actorsByID:           map[int64]Actor{},
		addressRefOverrides:  map[string]flowRef{},
		protocols:            protocols,
		prices:               prices,
		bondMemoNodeByTx:     map[string]string{},
		calcPayoutByContract: map[string]string{},
		thorTxTransfersByTx:  map[string][]thorTxTransfer{},
		midgardActionsByTx:   map[string][]midgardAction{},
		recordedActionKeys:   map[string]struct{}{},
		allowedFlowTypes:     flowTypeSet(prepared.query.FlowTypes),
		minUSD:               prepared.query.MinUSD,
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
		warnings:             append([]string{}, response.Warnings...),
		seenCanonicalKey:     map[string]struct{}{},
		sourceProtocols:      map[string]struct{}{},
	}
	if priceErr != nil {
		builder.warnings = append(builder.warnings, "spot USD normalization unavailable; falling back to asset-native values")
	}

	for _, chain := range activeChains {
		normalizedAddress := normalizeAddress(prepared.address)
		targetRef := flowRef{
			ID:        fmt.Sprintf("explorer_target:%s|%s:protocol:0", chain, normalizedAddress),
			Key:       frontierKey(chain, prepared.address),
			Kind:      "explorer_target",
			Label:     shortAddress(prepared.address),
			Chain:     chain,
			Stage:     "protocol",
			Depth:     0,
			Collapsed: false,
			Address:   normalizedAddress,
			Metrics: map[string]any{
				"address": prepared.address,
			},
		}
		builder.addressRefOverrides[targetRef.Key] = targetRef
		builder.ensureNode(targetRef)
	}

	combinedExternalTransfers := make([]externalTransfer, 0)
	for _, item := range seedActivity {
		if !item.Active {
			continue
		}
		combinedExternalTransfers = append(combinedExternalTransfers, item.ExternalTransfers...)
	}

	midgardSwapTxIDs := collectMidgardSwapTxIDs(actions)
	refundTxIDs := collectMidgardRefundTxIDs(actions)
	liquidityFeeTxIDs := collectMidgardLiquidityFeeTxIDs(actions)
	calcStrategyTxIDs := collectCalcStrategyTxIDs(actions)
	calcStrategyProcessTxIDs := collectCalcStrategyProcessTxIDs(actions)
	_ = midgardSwapTxIDs

	builder.warnings = append(builder.warnings, a.hydrateBondMemoNodeCache(ctx, actions, builder.bondMemoNodeByTx)...)
	builder.recordMidgardActions(actions)
	a.prefetchThorTxTransfers(ctx, actions, builder)
	builder.recordCalcRepresentativePayouts(actions)

	consumedExternalTransfers := map[string]struct{}{}
	seenMidgardActions := map[string]struct{}{}
	seenExternalTransfers := map[string]struct{}{}

	for _, action := range actions {
		key := midgardActionKey(action)
		if key == "" {
			continue
		}
		if _, exists := seenMidgardActions[key]; exists {
			_, consumed := builder.stitchMidgardAction(action, combinedExternalTransfers)
			mergeStringSet(consumedExternalTransfers, consumed)
			continue
		}
		if skip, _ := shouldSkipMidgardActionForGraph(action, refundTxIDs, liquidityFeeTxIDs, calcStrategyTxIDs, calcStrategyProcessTxIDs); skip {
			seenMidgardActions[key] = struct{}{}
			continue
		}
		if builder.shouldSkipActionBecauseRujiraTrace(action) {
			seenMidgardActions[key] = struct{}{}
			continue
		}

		segments, _, warnings, consumed := builder.projectMidgardActionWithExternal(action, 1, combinedExternalTransfers)
		builder.warnings = append(builder.warnings, warnings...)
		mergeStringSet(consumedExternalTransfers, consumed)
		if len(segments) == 0 {
			continue
		}
		seenMidgardActions[key] = struct{}{}
		for _, segment := range segments {
			builder.addProjectedSegment(segment)
		}
	}

	for _, transfer := range combinedExternalTransfers {
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
		if skip, _ := shouldSkipExternalTransferForGraph(transfer, refundTxIDs, liquidityFeeTxIDs); skip {
			seenExternalTransfers[key] = struct{}{}
			continue
		}
		segments, _ := builder.projectExternalTransfer(transfer, 1)
		if len(segments) == 0 {
			continue
		}
		seenExternalTransfers[key] = struct{}{}
		for _, segment := range segments {
			builder.addProjectedSegment(segment)
		}
	}

	nodes := builder.nodeList()
	builder.warnings = append(builder.warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, builder.protocols, true)...)
	builder.applyNodeLabelsToValidatorMetadata(nodes)
	edges := builder.edgeList()
	supportingActions := builder.actionList()

	response.Nodes = nodes
	response.Edges = edges
	response.SupportingActions = supportingActions
	response.LoadedActions = len(actions)
	response.Warnings = uniqueStrings(builder.warnings)
	response.Stats = map[string]any{
		"node_count":              len(nodes),
		"edge_count":              len(edges),
		"supporting_action_count": len(supportingActions),
		"active_chain_count":      len(activeChains),
		"source_protocol_count":   len(builder.sourceProtocolList()),
		"source_protocols":        builder.sourceProtocolList(),
		"elapsed_ms":              time.Since(started).Milliseconds(),
	}

	logInfo(ctx, "address_explorer_completed", map[string]any{
		"address":       prepared.address,
		"active_chains": activeChains,
		"nodes":         len(nodes),
		"edges":         len(edges),
		"actions":       len(supportingActions),
		"elapsed_ms":    time.Since(started).Milliseconds(),
	})

	return response, nil
}

func normalizeAddressExplorerRequest(req AddressExplorerRequest, protocols protocolDirectory) (explorerPreparedRequest, error) {
	rawAddress := strings.TrimSpace(req.Address)
	if rawAddress == "" {
		return explorerPreparedRequest{}, fmt.Errorf("address is required")
	}

	seed, explicitChain := parseExplorerInputSeed(rawAddress)
	if seed.Address == "" {
		return explorerPreparedRequest{}, fmt.Errorf("invalid address: %s", rawAddress)
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	switch mode {
	case "", explorerModeGraph:
		mode = explorerModeGraph
	case explorerModePreview:
	default:
		return explorerPreparedRequest{}, fmt.Errorf("mode must be 'preview' or 'graph'")
	}

	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	switch direction {
	case "":
		if mode == explorerModeGraph {
			direction = "newest"
		}
	case "newest", "oldest":
	default:
		return explorerPreparedRequest{}, fmt.Errorf("direction must be 'newest' or 'oldest'")
	}

	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = explorerDefaultBatchSize
	}
	if batchSize > explorerMaxBatchSize {
		batchSize = explorerMaxBatchSize
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	seeds, err := resolveAddressExplorerSeeds(protocols, seed, explicitChain)
	if err != nil {
		return explorerPreparedRequest{}, err
	}

	query := AddressExplorerQuery{
		Address:   seed.Address,
		FlowTypes: req.FlowTypes,
		MinUSD:    req.MinUSD,
		Mode:      mode,
		Direction: direction,
		Offset:    offset,
		BatchSize: batchSize,
	}

	return explorerPreparedRequest{
		rawAddress: rawAddress,
		address:    seed.Address,
		mode:       mode,
		direction:  direction,
		batchSize:  batchSize,
		offset:     offset,
		query:      query,
		start:      time.Unix(0, 0),
		end:        time.Now().UTC(),
		protocols:  protocols,
		seeds:      seeds,
	}, nil
}

func parseExplorerInputSeed(raw string) (frontierAddress, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return frontierAddress{}, false
	}
	explicitChain := false
	if idx := strings.Index(raw, "|"); idx > 0 {
		candidateChain := strings.ToUpper(strings.TrimSpace(raw[:idx]))
		if isLikelyChainCode(candidateChain) {
			explicitChain = true
		}
	}
	return normalizeFrontierAddress(raw), explicitChain
}

func resolveAddressExplorerSeeds(protocols protocolDirectory, seed frontierAddress, explicitChain bool) ([]frontierAddress, error) {
	address := normalizeAddress(seed.Address)
	if address == "" {
		return nil, fmt.Errorf("invalid address")
	}

	normalizedChain := normalizeChain(seed.Chain, address)
	if explicitChain || !isLikelyEVMAddress(address) {
		if normalizedChain == "" {
			return nil, fmt.Errorf("unable to determine chain for %s", seed.Address)
		}
		if _, ok := protocols.SupportedChains[normalizedChain]; !ok {
			return nil, fmt.Errorf("unsupported chain: %s", normalizedChain)
		}
		return []frontierAddress{{Address: address, Chain: normalizedChain}}, nil
	}

	chains := compatibleExplorerChains(protocols, address)
	if len(chains) == 0 {
		if normalizedChain == "" {
			return nil, fmt.Errorf("unable to determine compatible chains for %s", seed.Address)
		}
		chains = []string{normalizedChain}
	}

	out := make([]frontierAddress, 0, len(chains))
	for _, chain := range chains {
		out = append(out, frontierAddress{Address: address, Chain: chain})
	}
	return uniqueFrontierAddresses(out), nil
}

func compatibleExplorerChains(protocols protocolDirectory, address string) []string {
	address = normalizeAddress(address)
	if !isLikelyEVMAddress(address) {
		chain := normalizeChain("", address)
		if chain == "" {
			return nil
		}
		if _, ok := protocols.SupportedChains[chain]; !ok {
			return nil
		}
		return []string{chain}
	}

	out := make([]string, 0, len(explorerCompatibleEVMChains))
	for _, chain := range explorerCompatibleEVMChains {
		if _, ok := protocols.SupportedChains[chain]; ok {
			out = append(out, chain)
		}
	}
	return out
}

func (a *App) fetchAddressExplorerActions(ctx context.Context, address, direction string, offset, batchSize int, start, end time.Time) ([]midgardAction, bool, int, int, error) {
	seed := normalizeFrontierAddress(address)
	protocols := a.actionSourceProtocolsForSeed(seed)
	if len(protocols) == 0 {
		return nil, false, 0, -1, nil
	}
	if len(protocols) == 1 {
		return a.fetchAddressExplorerActionsForProtocol(ctx, protocols[0], seed, direction, offset, batchSize, start, end)
	}

	totalEstimate := -1
	if direction == "" {
		direction = "newest"
	}
	var (
		groups   [][]midgardAction
		hasMore  bool
		firstErr error
	)
	for _, protocol := range protocols {
		startPage := offset
		protocolHasMore := false
		if direction == "oldest" {
			totalPages, err := a.probeMidgardTotalPagesForProtocol(ctx, protocol, seed.Address, start, end)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if totalEstimate < 0 {
				totalEstimate = 0
			}
			totalEstimate += totalPages * midgardActionsPageLimit
			startPage = totalPages - (offset + batchSize)
			if startPage < 0 {
				startPage = 0
			}
			protocolHasMore = totalPages > offset+batchSize
		}

		actions, truncated, err := a.fetchActionHistoryForAddressPagedFromProtocol(ctx, protocol, seed, start, end, startPage, batchSize)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		groups = append(groups, actions)
		if direction == "newest" {
			protocolHasMore = truncated
		}
		hasMore = hasMore || protocolHasMore
	}
	merged := mergeSourcedMidgardActions(groups...)
	if len(merged) == 0 && firstErr != nil {
		return nil, false, 0, totalEstimate, firstErr
	}
	nextOffset := offset + batchSize
	return merged, hasMore, nextOffset, totalEstimate, nil
}

func (a *App) fetchAddressExplorerActionsForProtocol(ctx context.Context, protocol string, seed frontierAddress, direction string, offset, batchSize int, start, end time.Time) ([]midgardAction, bool, int, int, error) {
	totalEstimate := -1
	startPage := offset
	if direction == "" {
		direction = "newest"
	}
	if direction == "oldest" && offset == 0 {
		totalPages, err := a.probeMidgardTotalPagesForProtocol(ctx, protocol, seed.Address, start, end)
		if err != nil {
			return nil, false, 0, 0, err
		}
		totalEstimate = totalPages * midgardActionsPageLimit
		startPage = totalPages - batchSize
		if startPage < 0 {
			startPage = 0
		}
		logInfo(ctx, "address_explorer_oldest_probe", map[string]any{
			"total_pages":    totalPages,
			"total_estimate": totalEstimate,
			"start_page":     startPage,
			"protocol":       protocol,
		})
	}

	actions, truncated, err := a.fetchActionHistoryForAddressPagedFromProtocol(ctx, protocol, seed, start, end, startPage, batchSize)
	if err != nil {
		return actions, false, 0, totalEstimate, err
	}

	hasMore := false
	nextOffset := 0
	if direction == "newest" {
		hasMore = truncated
		nextOffset = startPage + batchSize
	} else {
		hasMore = startPage > 0
		nextOffset = startPage - batchSize
		if nextOffset < 0 {
			nextOffset = 0
		}
	}

	logInfo(ctx, "address_explorer_actions_fetched", map[string]any{
		"address":      seed.Address,
		"action_count": len(actions),
		"direction":    direction,
		"has_more":     hasMore,
		"start_page":   startPage,
		"batch_size":   batchSize,
		"protocol":     protocol,
	})

	return actions, hasMore, nextOffset, totalEstimate, nil
}

func (a *App) inspectAddressExplorerSeeds(ctx context.Context, prepared explorerPreparedRequest, actions []midgardAction, transferPages int) ([]explorerSeedActivity, []string) {
	counts := explorerMidgardActionCountsByChain(actions, prepared.address, prepared.seeds)
	warnings := []string{}
	out := make([]explorerSeedActivity, 0, len(prepared.seeds))

	for _, seed := range prepared.seeds {
		chain := strings.ToUpper(strings.TrimSpace(seed.Chain))
		item := explorerSeedActivity{
			Seed:               seed,
			MidgardActionCount: counts[chain],
		}

		transfers, _, warning, err := a.fetchExternalTransfersForAddress(ctx, chain, prepared.address, prepared.start, prepared.end, transferPages)
		if err == nil {
			item.ExternalTransfers = transfers
			item.ExternalTransferSeen = len(transfers) > 0
		}
		item.ExternalWarning = warning
		item.ExternalError = err
		item.Active = item.MidgardActionCount > 0 || item.ExternalTransferSeen
		out = append(out, item)

		if item.Active || len(prepared.seeds) == 1 {
			if warning != "" {
				warnings = append(warnings, warning)
			}
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("external tracker fetch failed for %s on %s", shortAddress(prepared.address), chain))
			}
		}
	}

	if len(out) == 1 && !out[0].Active && len(actions) > 0 {
		out[0].Active = true
		out[0].MidgardActionCount = max(out[0].MidgardActionCount, len(actions))
	}

	return out, uniqueStrings(warnings)
}

func explorerMidgardActionCountsByChain(actions []midgardAction, address string, seeds []frontierAddress) map[string]int {
	address = normalizeAddress(address)
	if address == "" {
		return map[string]int{}
	}

	candidateChains := map[string]struct{}{}
	for _, seed := range seeds {
		chain := strings.ToUpper(strings.TrimSpace(seed.Chain))
		if chain != "" {
			candidateChains[chain] = struct{}{}
		}
	}

	seen := map[string]map[string]struct{}{}
	counts := map[string]int{}
	for _, action := range actions {
		actionChains := map[string]struct{}{}
		for _, leg := range action.In {
			if normalizeAddress(leg.Address) != address {
				continue
			}
			chain := normalizeChain(chainFromMidgardCoins(leg.Coins), leg.Address)
			if chain == "" && len(seeds) == 1 {
				chain = strings.ToUpper(strings.TrimSpace(seeds[0].Chain))
			}
			if chain == "" {
				continue
			}
			if len(candidateChains) > 0 {
				if _, ok := candidateChains[chain]; !ok {
					continue
				}
			}
			actionChains[chain] = struct{}{}
		}
		for _, leg := range action.Out {
			if normalizeAddress(leg.Address) != address {
				continue
			}
			chain := normalizeChain(chainFromMidgardCoins(leg.Coins), leg.Address)
			if chain == "" && len(seeds) == 1 {
				chain = strings.ToUpper(strings.TrimSpace(seeds[0].Chain))
			}
			if chain == "" {
				continue
			}
			if len(candidateChains) > 0 {
				if _, ok := candidateChains[chain]; !ok {
					continue
				}
			}
			actionChains[chain] = struct{}{}
		}

		if len(actionChains) == 0 && len(seeds) == 1 {
			fallbackChain := strings.ToUpper(strings.TrimSpace(seeds[0].Chain))
			if fallbackChain != "" {
				actionChains[fallbackChain] = struct{}{}
			}
		}

		actionKey := midgardActionKey(action)
		if actionKey == "" {
			actionKey = midgardSyntheticTxID(action)
		}
		if actionKey == "" {
			actionKey = strings.Join([]string{
				strings.TrimSpace(action.Type),
				strings.TrimSpace(action.Date),
				strings.TrimSpace(action.Height),
			}, "|")
		}

		for chain := range actionChains {
			if seen[chain] == nil {
				seen[chain] = map[string]struct{}{}
			}
			if _, ok := seen[chain][actionKey]; ok {
				continue
			}
			seen[chain][actionKey] = struct{}{}
			counts[chain]++
		}
	}
	return counts
}

func explorerActiveChains(items []explorerSeedActivity) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if !item.Active {
			continue
		}
		chain := strings.ToUpper(strings.TrimSpace(item.Seed.Chain))
		if chain == "" {
			continue
		}
		if _, ok := seen[chain]; ok {
			continue
		}
		seen[chain] = struct{}{}
		out = append(out, chain)
	}
	return out
}

func explorerSeedSummaries(items []explorerSeedActivity) []AddressExplorerSeedSummary {
	out := make([]AddressExplorerSeedSummary, 0, len(items))
	for _, item := range items {
		out = append(out, AddressExplorerSeedSummary{
			Chain:                 strings.ToUpper(strings.TrimSpace(item.Seed.Chain)),
			Address:               item.Seed.Address,
			Active:                item.Active,
			MidgardActionCount:    item.MidgardActionCount,
			ExternalTransferCount: len(item.ExternalTransfers),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Chain < out[j].Chain
	})
	return out
}

func explorerRunLabel(address string, activeChains []string, direction string) string {
	chainSummary := "No active chains"
	if len(activeChains) > 0 {
		chainSummary = strings.Join(activeChains, "/")
	}
	direction = strings.TrimSpace(direction)
	if direction == "" {
		direction = "newest"
	}
	return fmt.Sprintf("%s | %s | %s", shortAddress(address), chainSummary, direction)
}
