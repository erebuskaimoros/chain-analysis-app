package app

import (
	"context"
	"fmt"
	"time"
)

const (
	explorerDefaultBatchSize = 10 // 10 pages × 50 = 500 actions
	explorerMaxBatchSize     = 20
)

func (a *App) buildAddressExplorer(ctx context.Context, req AddressExplorerRequest) (AddressExplorerResponse, error) {
	started := time.Now()

	address := req.Address
	if address == "" {
		return AddressExplorerResponse{}, fmt.Errorf("address is required")
	}
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return AddressExplorerResponse{}, fmt.Errorf("invalid address: %s", address)
	}
	address = seed.Address

	direction := req.Direction
	if direction == "" {
		direction = "newest"
	}
	if direction != "newest" && direction != "oldest" {
		return AddressExplorerResponse{}, fmt.Errorf("direction must be 'newest' or 'oldest'")
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

	query := AddressExplorerQuery{
		Address:   address,
		FlowTypes: req.FlowTypes,
		MinUSD:    req.MinUSD,
		Direction: direction,
		Offset:    offset,
		BatchSize: batchSize,
	}

	logInfo(ctx, "address_explorer_started", map[string]any{
		"address":    address,
		"direction":  direction,
		"offset":     offset,
		"batch_size": batchSize,
	})

	// Wide time range — no user-controlled time filter.
	start := time.Unix(0, 0)
	end := time.Now().UTC()

	totalEstimate := -1

	// For "oldest" direction on first request, probe to find the last page.
	startPage := offset
	if direction == "oldest" && offset == 0 {
		totalPages, err := a.probeMidgardTotalPages(ctx, address, start, end)
		if err != nil {
			return AddressExplorerResponse{}, fmt.Errorf("failed to probe total pages: %w", err)
		}
		totalEstimate = totalPages * midgardActionsPageLimit
		// Start from the oldest batch.
		startPage = totalPages - batchSize
		if startPage < 0 {
			startPage = 0
		}
		logInfo(ctx, "address_explorer_oldest_probe", map[string]any{
			"total_pages":    totalPages,
			"total_estimate": totalEstimate,
			"start_page":     startPage,
		})
	}

	// Fetch Midgard actions for this batch.
	actions, truncated, err := a.fetchMidgardActionsForAddressPaged(ctx, address, start, end, startPage, batchSize)
	if err != nil {
		return AddressExplorerResponse{}, fmt.Errorf("failed to fetch actions: %w", err)
	}

	logInfo(ctx, "address_explorer_actions_fetched", map[string]any{
		"action_count": len(actions),
		"truncated":    truncated,
		"start_page":   startPage,
		"batch_size":   batchSize,
	})

	// Determine pagination state.
	hasMore := false
	nextOffset := 0
	if direction == "newest" {
		// Loading newest-first, "load more" loads older.
		hasMore = truncated
		nextOffset = startPage + batchSize
	} else {
		// Loading oldest-first, "load more" loads newer (lower page numbers).
		hasMore = startPage > 0
		nextOffset = startPage - batchSize
		if nextOffset < 0 {
			nextOffset = 0
		}
	}

	// Load metadata for graph building.
	protocols, err := a.loadProtocolDirectory(ctx)
	if err != nil {
		return AddressExplorerResponse{}, err
	}
	prices, priceErr := a.buildPriceBook(ctx)

	builder := &graphBuilder{
		ownerMap:             map[string][]int64{},
		actorsByID:           map[int64]Actor{},
		protocols:            protocols,
		prices:               prices,
		bondMemoNodeByTx:     map[string]string{},
		calcPayoutByContract: map[string]string{},
		allowedFlowTypes:     flowTypeSet(query.FlowTypes),
		minUSD:               query.MinUSD,
		nodes:                map[string]*FlowNode{},
		edges:                map[string]*FlowEdge{},
		actions:              map[string]*SupportingAction{},
		warnings:             []string{},
		seenCanonicalKey:     map[string]struct{}{},
	}
	if priceErr != nil {
		builder.warnings = append(builder.warnings, "spot USD normalization unavailable; falling back to asset-native values")
	}

	// Create the target node for the queried address.
	targetRef := builder.makeAddressRef(address, seed.Chain, 0)
	targetRef.Kind = "explorer_target"
	targetRef.Label = shortAddress(address)
	builder.ensureNode(targetRef)

	// Fetch external transfers for stitching.
	externalTransfers, _, externalWarning, extErr := a.fetchExternalTransfersForAddress(ctx, seed.Chain, address, start, end, batchSize)
	if externalWarning != "" {
		builder.warnings = append(builder.warnings, externalWarning)
	}
	if extErr != nil {
		builder.warnings = append(builder.warnings, fmt.Sprintf("external tracker fetch failed for %s", shortAddress(address)))
		externalTransfers = nil
	}

	// Collect special tx sets for filtering.
	midgardSwapTxIDs := collectMidgardSwapTxIDs(actions)
	refundTxIDs := collectMidgardRefundTxIDs(actions)
	liquidityFeeTxIDs := collectMidgardLiquidityFeeTxIDs(actions)
	calcStrategyTxIDs := collectCalcStrategyTxIDs(actions)
	calcStrategyProcessTxIDs := collectCalcStrategyProcessTxIDs(actions)
	_ = midgardSwapTxIDs

	builder.warnings = append(builder.warnings, a.hydrateBondMemoNodeCache(ctx, actions, builder.bondMemoNodeByTx)...)
	builder.recordCalcRepresentativePayouts(actions)

	consumedExternalTransfers := map[string]struct{}{}
	seenMidgardActions := map[string]struct{}{}
	seenExternalTransfers := map[string]struct{}{}

	// Process each Midgard action — depth 1, no frontier expansion.
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
		if skip, _ := shouldSkipMidgardActionForGraph(action, refundTxIDs, liquidityFeeTxIDs, calcStrategyTxIDs, calcStrategyProcessTxIDs); skip {
			seenMidgardActions[key] = struct{}{}
			continue
		}

		segments, _, warnings, consumed := builder.projectMidgardActionWithExternal(action, 1, externalTransfers)
		builder.warnings = append(builder.warnings, warnings...)
		mergeStringSet(consumedExternalTransfers, consumed)
		if len(segments) == 0 {
			continue
		}
		seenMidgardActions[key] = struct{}{}
		for _, segment := range segments {
			builder.addProjectedSegment(segment)
		}
		// No frontier expansion — depth 1 only.
	}

	// Process unconsumed external transfers.
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
	builder.warnings = append(builder.warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, builder.protocols)...)
	edges := builder.edgeList()
	supportingActions := builder.actionList()

	stats := map[string]any{
		"node_count":              len(nodes),
		"edge_count":              len(edges),
		"supporting_action_count": len(supportingActions),
		"elapsed_ms":              time.Since(started).Milliseconds(),
	}

	logInfo(ctx, "address_explorer_completed", map[string]any{
		"address":    address,
		"nodes":      len(nodes),
		"edges":      len(edges),
		"actions":    len(supportingActions),
		"elapsed_ms": time.Since(started).Milliseconds(),
	})

	return AddressExplorerResponse{
		Address:           address,
		Query:             query,
		Stats:             stats,
		Warnings:          uniqueStrings(builder.warnings),
		Nodes:             nodes,
		Edges:             edges,
		SupportingActions: supportingActions,
		LoadedActions:     len(actions),
		HasMore:           hasMore,
		NextOffset:        nextOffset,
		TotalEstimate:     totalEstimate,
	}, nil
}
