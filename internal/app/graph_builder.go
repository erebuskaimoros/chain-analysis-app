package app

import (
	"fmt"
	"sort"
	"strings"
)

func (b *graphBuilder) addProjectedSegment(seg projectedSegment) {
	source := b.ensureNode(seg.Source)
	target := b.ensureNode(seg.Target)
	if source.ID == "" || target.ID == "" {
		return
	}
	mergeNodeSourceProtocol(source, seg.SourceProtocol)
	mergeNodeSourceProtocol(target, seg.SourceProtocol)
	if b.sourceProtocols == nil {
		b.sourceProtocols = map[string]struct{}{}
	}
	if protocol := normalizeSourceProtocol(seg.SourceProtocol); protocol != "" {
		b.sourceProtocols[protocol] = struct{}{}
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
		validatorLabel = protocolBondDisplayLabel(seg.SourceProtocol, validatorAddress, "")
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
			ConfidenceReason: seg.ConfidenceReason,
			SourceProtocols:  nil,
		}
		b.edges[edgeKey] = edge
	}
	if seg.Confidence < edge.Confidence {
		edge.Confidence, edge.ConfidenceReason = seg.Confidence, seg.ConfidenceReason
	}
	edge.ActorIDs = mergeInt64s(edge.ActorIDs, seg.ActorIDs)
	if protocol := normalizeSourceProtocol(seg.SourceProtocol); protocol != "" {
		edge.SourceProtocols = appendUniqueString(edge.SourceProtocols, protocol)
	}
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
			mergeEdgeTransactionAsset(edge, seg.TxID, seg.SourceProtocol, seg.Height, seg.Time, inAsset, inAmount, b.prices.usdFor(inAsset, inAmount), inMeta, "in")
			added = true
		}
		if outAsset != "" && hasGraphableLiquidity(outAmount) {
			outMeta := assetMetadataFromAsset(outAsset)
			mergeEdgeTransactionAsset(edge, seg.TxID, seg.SourceProtocol, seg.Height, seg.Time, outAsset, outAmount, b.prices.usdFor(outAsset, outAmount), outMeta, "out")
			added = true
		}
		if !added {
			mergeEdgeTransactionAsset(edge, seg.TxID, seg.SourceProtocol, seg.Height, seg.Time, seg.Asset, seg.AmountRaw, seg.USDSpot, meta, "")
		}
	} else {
		mergeEdgeTransactionAsset(edge, seg.TxID, seg.SourceProtocol, seg.Height, seg.Time, seg.Asset, seg.AmountRaw, seg.USDSpot, meta, "")
	}
	if seg.InboundTxID != "" {
		for i := range edge.Transactions {
			if edge.Transactions[i].TxID == seg.TxID && edge.Transactions[i].InboundTxID == "" {
				edge.Transactions[i].InboundTxID = seg.InboundTxID
			}
		}
	}
	recomputeEdgeAggregate(edge)

	source.Metrics["out_edges"] = intMetric(source.Metrics["out_edges"]) + 1
	target.Metrics["in_edges"] = intMetric(target.Metrics["in_edges"]) + 1

	if seg.ActionClass == "ownership" {
		return
	}
	actionID := strings.Join([]string{seg.TxID, normalizeSourceProtocol(seg.SourceProtocol), actionKey, source.ID, target.ID}, "|")
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
			SourceProtocol:   normalizeSourceProtocol(seg.SourceProtocol),
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
		if action.SourceProtocol == "" {
			action.SourceProtocol = normalizeSourceProtocol(seg.SourceProtocol)
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

func (b *graphBuilder) makePoolRef(pool, protocol string, depth int) flowRef {
	pool = normalizeAsset(strings.TrimSpace(pool))
	if pool == "" {
		return flowRef{}
	}
	protocol = normalizeSourceProtocol(protocol)
	return flowRef{
		ID:    fmt.Sprintf("pool:%s:%s:pool:%d", normalizeAsset(pool), strings.ToLower(protocol), depth),
		Key:   protocolPoolSnapshotKey(protocol, pool),
		Kind:  "pool",
		Label: poolDisplayLabel(pool) + " (" + protocol + ")",
		Chain: chainFromAsset(pool),
		Stage: "pool",
		Depth: depth,
		Metrics: map[string]any{
			"pool":            pool,
			"source_protocol": protocol,
		},
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

func (b *graphBuilder) makeNodeRef(nodeAddress, protocol string, depth int) flowRef {
	nodeAddress = strings.TrimSpace(nodeAddress)
	if nodeAddress == "" {
		return flowRef{}
	}
	normalized := normalizeAddress(nodeAddress)
	protocol = normalizeSourceProtocol(protocol)
	metrics := map[string]any{
		"address":         nodeAddress,
		"source_protocol": protocol,
	}
	return flowRef{
		ID:      fmt.Sprintf("node:%s:%s:node_bond:%d", normalized, strings.ToLower(protocol), depth),
		Key:     normalized,
		Kind:    "node",
		Label:   protocolBondDisplayLabel(protocol, nodeAddress, ""),
		Chain:   normalizeChain(protocol, nodeAddress),
		Stage:   "node_bond",
		Depth:   depth,
		Address: normalized,
		Metrics: metrics,
	}
}

func (b *graphBuilder) makeBondRef(address, protocol string, depth int) flowRef {
	address = strings.TrimSpace(address)
	if address == "" {
		return flowRef{}
	}
	protocol = normalizeSourceProtocol(protocol)
	ref := b.makeAddressRef(address, protocol, depth)
	if ref.Kind == "external_address" {
		ref.Kind = "bond_address"
		ref.Stage = "node_bond"
		ref.ID = fmt.Sprintf("bond_address:%s:%s:node_bond:%d", normalizeAddress(address), strings.ToLower(protocol), depth)
		ref.Label = "Bond " + shortAddress(address)
	}
	return ref
}

func (b *graphBuilder) makeBondWalletRef(address, protocol string, depth int) flowRef {
	address = strings.TrimSpace(address)
	if address == "" {
		return flowRef{}
	}
	protocol = normalizeSourceProtocol(protocol)
	ref := b.makeAddressRef(address, protocol, depth)
	if ref.ID == "" || ref.Kind == "actor_address" {
		return ref
	}
	if ref.Kind != "bond_address" {
		return ref
	}
	norm := normalizeAddress(address)
	ref.ID = fmt.Sprintf("external_address:%s:%s:external:%d", norm, strings.ToLower(protocol), depth)
	ref.Key = norm
	ref.Kind = "external_address"
	ref.Stage = "external"
	ref.Label = "Bond Wallet " + shortAddress(address)
	ref.Chain = normalizeChain(protocol, address)
	ref.Collapsed = true
	ref.Address = norm
	if ref.Metrics == nil {
		ref.Metrics = map[string]any{}
	}
	ref.Metrics["address"] = address
	ref.Metrics["source_protocol"] = protocol
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
			label = protocolBondDisplayLabel(getString(node.Metrics, "source_protocol"), address, getString(node.Metrics, "node_status"))
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

func (b *graphBuilder) sourceProtocolList() []string {
	if b == nil || len(b.sourceProtocols) == 0 {
		return nil
	}
	out := make([]string, 0, len(b.sourceProtocols))
	for protocol := range b.sourceProtocols {
		if protocol = normalizeSourceProtocol(protocol); protocol != "" {
			out = append(out, protocol)
		}
	}
	sort.Strings(out)
	return out
}

// priceSegment values a segment at its transaction time.
func (b *graphBuilder) priceSegment(seg *projectedSegment) {
	seg.USDAtTime, seg.PriceSource, seg.Priced = usdAtTime(b.history, b.prices, seg.Asset, seg.AmountRaw, seg.Time)
}

// belowMinUSD reports whether the min-USD filter drops a segment: priced
// flows worth less than the minimum at their transaction time, and unpriced
// flows unless the request includes them.
func (b *graphBuilder) belowMinUSD(seg projectedSegment) bool {
	if b.minUSD <= 0 {
		return false
	}
	if !seg.Priced {
		return !b.includeUnpriced
	}
	return seg.USDAtTime < b.minUSD
}

// applyAtTimeValues fills usd_at_time on edges, their transactions and
// assets, and supporting actions. A transaction is valued by its inbound
// assets when it has any (a swap's input), otherwise by all its assets, so a
// swap is not counted on both sides.
func (b *graphBuilder) applyAtTimeValues(edges []FlowEdge, actions []SupportingAction) {
	for i := range edges {
		edge := &edges[i]
		edge.USDAtTime = 0
		byAsset := map[string]float64{}
		for j := range edge.Transactions {
			tx := &edge.Transactions[j]
			var inbound, all float64
			hasInbound := false
			for k := range tx.Assets {
				asset := &tx.Assets[k]
				usd, source, _ := usdAtTime(b.history, b.prices, asset.Asset, asset.AmountRaw, tx.Time)
				asset.USDAtTime, asset.PriceSource = usd, source
				all += usd
				if asset.Direction == "in" {
					inbound += usd
					hasInbound = true
				}
				byAsset[asset.Asset+"|"+asset.Direction] += usd
			}
			tx.USDAtTime = all
			if hasInbound {
				tx.USDAtTime = inbound
			}
			edge.USDAtTime += tx.USDAtTime
		}
		for k := range edge.Assets {
			edge.Assets[k].USDAtTime = byAsset[edge.Assets[k].Asset+"|"+edge.Assets[k].Direction]
		}
	}
	for i := range actions {
		usd, source, _ := usdAtTime(b.history, b.prices, actions[i].PrimaryAsset, actions[i].AmountRaw, actions[i].Time)
		actions[i].USDAtTime, actions[i].PriceSource = usd, source
	}
}
