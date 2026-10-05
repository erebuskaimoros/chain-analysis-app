package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type liveHoldingValue struct {
	Asset     string
	AmountRaw string
	USDSpot   float64
}

type liveHoldingNodeRef struct {
	index    int
	protocol string
}

type liveHoldingAddressLookupTask struct {
	key       string
	chain     string
	address   string
	provider  string
	bucketKey string
	refs      []liveHoldingNodeRef
}

type liveHoldingTaskRank struct {
	phase     int
	supported int
	depth     int
	kindRank  int
	chain     string
	address   string
}

func planAddressLookupPhases(nodes []FlowNode, tasks map[string]*liveHoldingAddressLookupTask) [][]liveHoldingAddressLookupTask {
	ordered := orderedAddressLookupTasks(nodes, tasks)
	if len(ordered) == 0 {
		return nil
	}
	priority := make([]liveHoldingAddressLookupTask, 0, len(ordered))
	background := make([]liveHoldingAddressLookupTask, 0, len(ordered))
	for _, task := range ordered {
		if liveHoldingTaskRankForNodes(nodes, task).phase == 0 {
			priority = append(priority, task)
			continue
		}
		background = append(background, task)
	}
	phases := make([][]liveHoldingAddressLookupTask, 0, 2)
	if len(priority) > 0 {
		phases = append(phases, priority)
	}
	if len(background) > 0 {
		phases = append(phases, background)
	}
	return phases
}

func orderedAddressLookupTasks(nodes []FlowNode, tasks map[string]*liveHoldingAddressLookupTask) []liveHoldingAddressLookupTask {
	ordered := make([]liveHoldingAddressLookupTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			ordered = append(ordered, *task)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left := liveHoldingTaskRankForNodes(nodes, ordered[i])
		right := liveHoldingTaskRankForNodes(nodes, ordered[j])
		if left.phase != right.phase {
			return left.phase < right.phase
		}
		if left.supported != right.supported {
			return left.supported < right.supported
		}
		if left.depth != right.depth {
			return left.depth < right.depth
		}
		if left.kindRank != right.kindRank {
			return left.kindRank < right.kindRank
		}
		if left.chain != right.chain {
			return left.chain < right.chain
		}
		return left.address < right.address
	})
	return ordered
}

func liveHoldingTaskRankForNodes(nodes []FlowNode, task liveHoldingAddressLookupTask) liveHoldingTaskRank {
	rank := liveHoldingTaskRank{
		phase:     1,
		supported: 0,
		depth:     1 << 30,
		kindRank:  99,
		chain:     strings.ToUpper(strings.TrimSpace(task.chain)),
		address:   normalizeAddress(task.address),
	}
	if strings.EqualFold(strings.TrimSpace(task.provider), "unconfigured") {
		rank.supported = 1
	}
	for _, ref := range task.refs {
		if ref.index < 0 || ref.index >= len(nodes) {
			continue
		}
		node := nodes[ref.index]
		if priorityAddressLiveHoldingsNode(node) {
			rank.phase = 0
		}
		if depth := max(0, node.Depth); depth < rank.depth {
			rank.depth = depth
		}
		if kindRank := liveHoldingKindRank(node.Kind); kindRank < rank.kindRank {
			rank.kindRank = kindRank
		}
	}
	if rank.depth == 1<<30 {
		rank.depth = 1 << 29
	}
	return rank
}

func priorityAddressLiveHoldingsNode(node FlowNode) bool {
	if len(node.ActorIDs) > 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(node.Kind)) {
	case "actor_address", "explorer_target":
		return true
	default:
		return false
	}
}

func liveHoldingKindRank(kind string) int {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "actor_address":
		return 0
	case "explorer_target":
		return 1
	case "bond_address":
		return 2
	case "contract_address":
		return 3
	case "router", "inbound":
		return 4
	default:
		return 5
	}
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
	warnings = append(warnings, a.enrichNodesWithLiveHoldings(ctx, nodes, prices, protocols, true)...)
	return uniqueStrings(warnings), nil
}

func markUnresolvedAddressLookupTasksPending(nodes []FlowNode, tasks map[string]*liveHoldingAddressLookupTask) {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		for _, ref := range task.refs {
			if ref.index < 0 || ref.index >= len(nodes) {
				continue
			}
			if nodes[ref.index].Metrics == nil {
				nodes[ref.index].Metrics = map[string]any{}
			}
			status := strings.ToLower(strings.TrimSpace(getString(nodes[ref.index].Metrics, "live_holdings_status")))
			if status != "" {
				continue
			}
			nodes[ref.index].Metrics["live_holdings_available"] = false
			nodes[ref.index].Metrics["live_holdings_status"] = "pending"
			nodes[ref.index].Metrics["live_holdings_error_kind"] = "budget"
		}
	}
}

func (a *App) enrichNodesWithLiveHoldings(
	ctx context.Context,
	nodes []FlowNode,
	prices priceBook,
	protocols protocolDirectory,
	includeAddressLookups bool,
) []string {
	if len(nodes) == 0 {
		return nil
	}
	// Live-holdings enrichment runs after the graph build has already spent most of
	// the request budget. Detach it from the parent deadline, then enforce both a
	// batch budget and per-upstream timeouts so slow public trackers do not block
	// the graph response for minutes.
	baseLookupCtx := context.WithoutCancel(ctx)
	batchTimeout := liveHoldingsBudgetFromContext(ctx, a.liveHoldingsBatchTimeout())
	lookupCtx, lookupCancel := context.WithTimeout(baseLookupCtx, batchTimeout)
	defer lookupCancel()

	warnings := []string{}
	now := time.Now().UTC().Format(time.RFC3339)
	addressLookupTasks := map[string]*liveHoldingAddressLookupTask{}
	nodeLookupRefs := make([]liveHoldingNodeRef, 0)
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
			task = &liveHoldingAddressLookupTask{
				key:       key,
				chain:     chain,
				address:   address,
				provider:  provider,
				bucketKey: liveHoldingsBucketKey(provider, chain),
			}
			addressLookupTasks[key] = task
		}
		task.refs = append(task.refs, liveHoldingNodeRef{index: idx})
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
			protocol := normalizeSourceProtocol(getString(nodes[i].Metrics, "source_protocol"))
			if protocol == sourceProtocolTHOR && strings.EqualFold(strings.TrimSpace(nodes[i].Chain), "MAYA") {
				protocol = sourceProtocolMAYA
			}
			pool, ok := prices.PoolSnapshots[protocolPoolSnapshotKey(protocol, poolAsset)]
			if !ok {
				continue
			}
			holdings := []liveHoldingValue{
				{
					Asset:     nativeAssetForProtocol(protocol),
					AmountRaw: strings.TrimSpace(pool.RuneDepth),
					USDSpot:   prices.usdFor(nativeAssetForProtocol(protocol), pool.RuneDepth),
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
			protocol := normalizeSourceProtocol(getString(nodes[i].Metrics, "source_protocol"))
			if protocol == sourceProtocolTHOR && strings.EqualFold(strings.TrimSpace(nodes[i].Chain), "MAYA") {
				protocol = sourceProtocolMAYA
			}
			nodeLookupRefs = append(nodeLookupRefs, liveHoldingNodeRef{index: i, protocol: protocol})
		default:
			if !includeAddressLookups && !priorityAddressLiveHoldingsNode(nodes[i]) {
				continue
			}
			queueAddressLookupTask(i)
		}
	}

	type bondSnapshot struct {
		bondedByAddress  map[string]string
		bondedByNode     map[string]string
		nodeStatusByNode map[string]string
	}
	bondSnapshots := map[string]bondSnapshot{}
	requiredBondProtocols := map[string]struct{}{}
	for _, task := range addressLookupTasks {
		if task == nil {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(task.chain)) {
		case "THOR", "MAYA":
			requiredBondProtocols[strings.ToUpper(strings.TrimSpace(task.chain))] = struct{}{}
		}
	}
	for _, ref := range nodeLookupRefs {
		requiredBondProtocols[normalizeSourceProtocol(ref.protocol)] = struct{}{}
	}
	for protocol := range requiredBondProtocols {
		provider := "thornode"
		if protocol == sourceProtocolMAYA {
			provider = "mayanode"
		}
		bondLookupCtx, bondLookupCancel := context.WithTimeout(lookupCtx, a.liveHoldingsLookupTimeout(provider, protocol))
		bondedByAddress, bondedByNode, nodeStatusByNode, err := a.fetchProtocolBondIndexes(bondLookupCtx, protocol)
		bondLookupCancel()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("live bonded %s unavailable for %s addresses", strings.ToLower(strings.TrimPrefix(nativeAssetForProtocol(protocol), strings.ToUpper(protocol)+".")), protocol))
			warnings = append(warnings, fmt.Sprintf("live bonded %s unavailable for %s validator nodes", strings.ToLower(strings.TrimPrefix(nativeAssetForProtocol(protocol), strings.ToUpper(protocol)+".")), protocol))
		} else {
			bondSnapshots[protocol] = bondSnapshot{
				bondedByAddress:  bondedByAddress,
				bondedByNode:     bondedByNode,
				nodeStatusByNode: nodeStatusByNode,
			}
		}
	}

	for _, ref := range nodeLookupRefs {
		idx := ref.index
		nodeAddress := normalizeAddress(getString(nodes[idx].Metrics, "address"))
		amountRaw := ""
		nodeStatus := ""
		snapshot := bondSnapshots[normalizeSourceProtocol(ref.protocol)]
		if snapshot.bondedByNode != nil {
			amountRaw = strings.TrimSpace(snapshot.bondedByNode[nodeAddress])
		}
		if snapshot.nodeStatusByNode != nil {
			nodeStatus = strings.TrimSpace(snapshot.nodeStatusByNode[nodeAddress])
		}
		if nodeAddress != "" {
			nodes[idx].Label = protocolBondDisplayLabel(ref.protocol, nodeAddress, nodeStatus)
		}
		nodes[idx].Metrics["node_status"] = nodeStatus
		nodes[idx].Metrics["node_total_bond"] = amountRaw
		if hasGraphableLiquidity(amountRaw) {
			holdings := []liveHoldingValue{{
				Asset:     nativeAssetForProtocol(ref.protocol),
				AmountRaw: amountRaw,
				USDSpot:   prices.usdFor(nativeAssetForProtocol(ref.protocol), amountRaw),
			}}
			source := "thornode_node_bond"
			if normalizeSourceProtocol(ref.protocol) == sourceProtocolMAYA {
				source = "mayanode_node_bond"
			}
			applyLiveHoldingMetrics(&nodes[idx], holdings, source, now)
		} else {
			nodes[idx].Metrics["live_holdings_available"] = false
			nodes[idx].Metrics["live_holdings_status"] = "error"
			nodes[idx].Metrics["live_holdings_error_kind"] = "no_bond"
		}
	}

	if len(addressLookupTasks) == 0 {
		if errors.Is(lookupCtx.Err(), context.DeadlineExceeded) {
			logInfo(baseLookupCtx, "actor_tracker_live_holdings_budget_exhausted", map[string]any{
				"timeout_ms": batchTimeout.Milliseconds(),
				"nodes":      len(nodes),
				"lookups":    0,
			})
			warnings = append(warnings, "live holdings lookup budget exhausted; some live values were skipped")
		}
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
	var failed []string
	runTaskPhase := func(tasks []liveHoldingAddressLookupTask) {
		if len(tasks) == 0 || lookupCtx.Err() != nil {
			return
		}
		results := make(chan addressResult, len(tasks))
		buckets := map[string][]liveHoldingAddressLookupTask{}
		for _, task := range tasks {
			buckets[task.bucketKey] = append(buckets[task.bucketKey], task)
		}
		runBucket := func(tasks []liveHoldingAddressLookupTask) {
			if len(tasks) == 0 {
				return
			}
			jobs := make(chan liveHoldingAddressLookupTask, len(tasks))
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
						if err := lookupCtx.Err(); err != nil {
							results <- addressResult{
								taskKey:  task.key,
								chain:    task.chain,
								address:  task.address,
								provider: task.provider,
								err:      err,
							}
							continue
						}
						taskTimeout := a.liveHoldingsLookupTimeout(task.provider, task.chain)
						taskCtx, taskCancel := context.WithTimeout(lookupCtx, taskTimeout)
						startedAt := time.Now()

						var (
							holdings []liveHoldingValue
							err      error
						)
						switch task.chain {
						case "THOR":
							holdings, err = a.fetchProtocolAddressLiveHoldings(taskCtx, sourceProtocolTHOR, task.address, prices, bondSnapshots[sourceProtocolTHOR].bondedByAddress)
						case "MAYA":
							holdings, err = a.fetchProtocolAddressLiveHoldings(taskCtx, sourceProtocolMAYA, task.address, prices, bondSnapshots[sourceProtocolMAYA].bondedByAddress)
						default:
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
			bucketTasks := append([]liveHoldingAddressLookupTask(nil), tasks...)
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

		for result := range results {
			task, ok := addressLookupTasks[result.taskKey]
			if !ok {
				continue
			}
			refs := task.refs
			if result.err != nil {
				budgetExceeded := errors.Is(lookupCtx.Err(), context.DeadlineExceeded) &&
					(errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled))
				if budgetExceeded {
					for _, ref := range refs {
						if nodes[ref.index].Metrics == nil {
							nodes[ref.index].Metrics = map[string]any{}
						}
						nodes[ref.index].Metrics["live_holdings_available"] = false
						nodes[ref.index].Metrics["live_holdings_status"] = "pending"
						nodes[ref.index].Metrics["live_holdings_error_kind"] = "budget"
					}
					continue
				}
				errorKind, _ := classifyProviderError(result.err)
				failed = append(failed, fmt.Sprintf("%s:%s", task.chain, shortAddress(task.address)))
				fields := map[string]any{
					"chain":      task.chain,
					"address":    task.address,
					"provider":   result.provider,
					"elapsed_ms": result.elapsed.Milliseconds(),
				}
				if task.chain == "THOR" {
					fields["provider_candidates"] = "thornode,midgard"
				} else if task.chain == "MAYA" {
					fields["provider_candidates"] = "mayanode,midgard"
				} else if providers := strings.Join(a.cfg.trackerProvidersForChain(task.chain), ","); providers != "" {
					fields["provider_candidates"] = providers
				}
				fields["error_kind"] = string(errorKind)
				logError(baseLookupCtx, "actor_tracker_live_holdings_lookup_failed", result.err, fields)
				for _, ref := range refs {
					if nodes[ref.index].Metrics == nil {
						nodes[ref.index].Metrics = map[string]any{}
					}
					nodes[ref.index].Metrics["live_holdings_available"] = false
					nodes[ref.index].Metrics["live_holdings_status"] = "error"
					nodes[ref.index].Metrics["live_holdings_error_kind"] = string(errorKind)
				}
				continue
			}
			for _, ref := range refs {
				source := "external_live_balance"
				if task.chain == "THOR" {
					source = "thornode_midgard"
				} else if task.chain == "MAYA" {
					source = "mayanode_midgard"
				}
				applyLiveHoldingMetrics(&nodes[ref.index], result.holdings, source, now)
			}
		}
	}
	for _, phaseTasks := range planAddressLookupPhases(nodes, addressLookupTasks) {
		runTaskPhase(phaseTasks)
		if errors.Is(lookupCtx.Err(), context.DeadlineExceeded) {
			break
		}
	}
	if len(failed) > 0 {
		sort.Strings(failed)
		limit := min(3, len(failed))
		warnings = append(warnings, fmt.Sprintf("live holdings unavailable for %d address nodes (%s)", len(failed), strings.Join(failed[:limit], ", ")))
	}
	if errors.Is(lookupCtx.Err(), context.DeadlineExceeded) {
		markUnresolvedAddressLookupTasksPending(nodes, addressLookupTasks)
		logInfo(baseLookupCtx, "actor_tracker_live_holdings_budget_exhausted", map[string]any{
			"timeout_ms": batchTimeout.Milliseconds(),
			"nodes":      len(nodes),
			"lookups":    len(addressLookupTasks),
		})
		warnings = append(warnings, "live holdings lookup budget exhausted; some live values were skipped")
	}
	return uniqueStrings(warnings)
}

func (a *App) liveHoldingsProviderForChain(chain string) string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	if chain == "THOR" {
		return "thornode"
	}
	if chain == "MAYA" {
		return "mayanode"
	}
	if provider := strings.ToLower(strings.TrimSpace(a.cfg.trackerProviderForChain(chain))); provider != "" {
		return provider
	}
	return "unconfigured"
}

func (a *App) liveHoldingsLookupTimeout(provider, chain string) time.Duration {
	if a != nil && a.cfg.LiveHoldingsTimeout > 0 {
		return a.cfg.LiveHoldingsTimeout
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	chain = strings.ToUpper(strings.TrimSpace(chain))
	switch {
	case provider == "utxo":
		return a.clampLiveHoldingsLookupTimeout(8 * time.Second)
	case provider == "solana" || provider == "xrpl" || provider == "cosmos" || provider == "radix":
		return a.clampLiveHoldingsLookupTimeout(6 * time.Second)
	case provider == "etherscan" || provider == "blockscout" || provider == "nodereal" || provider == "trongrid":
		return a.clampLiveHoldingsLookupTimeout(12 * time.Second)
	case provider == "thornode" || provider == "mayanode" || provider == "midgard" || chain == "THOR" || chain == "MAYA":
		return a.clampLiveHoldingsLookupTimeout(12 * time.Second)
	default:
		return a.clampLiveHoldingsLookupTimeout(10 * time.Second)
	}
}

type liveHoldingsBudgetCtxKey struct{}

// withLiveHoldingsBudget lets a caller that is not bound to an HTTP request
// (a background job) give the lookup batch a longer budget.
func withLiveHoldingsBudget(ctx context.Context, budget time.Duration) context.Context {
	return context.WithValue(ctx, liveHoldingsBudgetCtxKey{}, budget)
}

func liveHoldingsBudgetFromContext(ctx context.Context, fallback time.Duration) time.Duration {
	if ctx != nil {
		if budget, ok := ctx.Value(liveHoldingsBudgetCtxKey{}).(time.Duration); ok && budget > fallback {
			return budget
		}
	}
	return fallback
}

func (a *App) liveHoldingsBatchTimeout() time.Duration {
	if a != nil && a.cfg.LiveHoldingsTimeout > 0 {
		return a.cfg.LiveHoldingsTimeout
	}
	return a.clampLiveHoldingsLookupTimeout(10 * time.Second)
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
	case "thornode", "mayanode", "midgard":
		return 4
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
	delete(node.Metrics, "live_holdings_error_kind")
	delete(node.Metrics, "live_holdings_last_error_kind")
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
