package app

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

type thorCosmosTxResponse struct {
	TxResponse thorCosmosTxResult `json:"tx_response"`
}

type thorCosmosTxResult struct {
	TxHash    string              `json:"txhash"`
	Height    string              `json:"height"`
	Timestamp string              `json:"timestamp"`
	Code      int64               `json:"code"`
	Events    []thorCosmosTxEvent `json:"events"`
}

type thorCosmosTxEvent struct {
	Type       string                  `json:"type"`
	Attributes []thorCosmosTxEventAttr `json:"attributes"`
}

type thorCosmosTxEventAttr struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type thorTxTransfer struct {
	Index     int
	TxID      string
	Height    int64
	Time      time.Time
	From      string
	To        string
	Asset     string
	AmountRaw string
}

type thorTransferAmount struct {
	Asset     string
	AmountRaw string
}

type thorTxTraceRole string

const (
	thorTxTraceRoleUnknown   thorTxTraceRole = ""
	thorTxTraceRoleControl   thorTxTraceRole = "control"
	thorTxTraceRoleMarket    thorTxTraceRole = "market"
	thorTxTraceRoleCustody   thorTxTraceRole = "custody"
	thorTxTraceRoleFinancing thorTxTraceRole = "financing"
)

type thorTxVisibleContract struct {
	Descriptor contractCallDescriptor
	Role       thorTxTraceRole
}

type thorTxTraceContext struct {
	VisibleContracts   map[string]thorTxVisibleContract
	FinancingContracts map[string]struct{}
}

func (a *App) prefetchThorTxTransfers(ctx context.Context, actions []midgardAction, builder *graphBuilder) {
	if a == nil || builder == nil || len(actions) == 0 {
		return
	}
	if builder.thorTxTransfersByTx == nil {
		builder.thorTxTransfersByTx = map[string][]thorTxTransfer{}
	}
	txIDs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, action := range actions {
		if !shouldPrefetchThorTxTraceForAction(action) {
			continue
		}
		txID := traceTxIDForAction(action)
		if txID == "" {
			continue
		}
		if _, ok := builder.thorTxTransfersByTx[txID]; ok {
			continue
		}
		if _, ok := seen[txID]; ok {
			continue
		}
		seen[txID] = struct{}{}
		txIDs = append(txIDs, txID)
	}
	if len(txIDs) == 0 {
		return
	}

	const maxConcurrency = 4
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, txID := range txIDs {
		wg.Add(1)
		go func(txID string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			transfers, err := a.fetchThorTxTransfers(ctx, txID)
			if err != nil {
				logError(ctx, "rujira_trace_fetch_failed", err, map[string]any{
					"txid": txID,
				})
			}

			mu.Lock()
			builder.thorTxTransfersByTx[txID] = transfers
			mu.Unlock()
		}(txID)
	}

	wg.Wait()
}

func shouldPrefetchThorTxTraceForAction(action midgardAction) bool {
	if !strings.EqualFold(strings.TrimSpace(action.Type), "contract") {
		return false
	}
	if normalizeSourceProtocol(sourceProtocolFromAction(action)) != sourceProtocolTHOR {
		return false
	}
	if action.Metadata.Contract == nil {
		return false
	}
	return isTraceProjectableRujiraContractType(action.Metadata.Contract.ContractType)
}

func (a *App) fetchThorTxTransfers(ctx context.Context, txID string) ([]thorTxTransfer, error) {
	client := a.thornodeClient()
	if client == nil {
		return nil, errExternalTrackerUnavailable
	}
	txID = cleanTxID(txID)
	if txID == "" {
		return nil, nil
	}

	var response thorCosmosTxResponse
	if err := client.GetJSON(ctx, "/cosmos/tx/v1beta1/txs/"+txID, &response); err != nil {
		return nil, err
	}
	if response.TxResponse.Code != 0 {
		return nil, nil
	}
	return parseThorTxTransfers(response), nil
}

func parseThorTxTransfers(response thorCosmosTxResponse) []thorTxTransfer {
	txID := cleanTxID(response.TxResponse.TxHash)
	if txID == "" {
		return nil
	}
	height := parseInt64(response.TxResponse.Height)
	when, _ := time.Parse(time.RFC3339, strings.TrimSpace(response.TxResponse.Timestamp))

	out := make([]thorTxTransfer, 0)
	for idx, event := range response.TxResponse.Events {
		if !strings.EqualFold(strings.TrimSpace(event.Type), "transfer") {
			continue
		}
		sender := ""
		recipient := ""
		amounts := make([]thorTransferAmount, 0)
		for _, attr := range event.Attributes {
			switch strings.ToLower(strings.TrimSpace(attr.Key)) {
			case "sender":
				sender = normalizeAddress(attr.Value)
			case "recipient":
				recipient = normalizeAddress(attr.Value)
			case "amount":
				amounts = append(amounts, parseThorTransferEventAmounts(attr.Value)...)
			}
		}
		if sender == "" || recipient == "" || len(amounts) == 0 {
			continue
		}
		for _, amount := range amounts {
			if amount.Asset == "" || !hasGraphableLiquidity(amount.AmountRaw) {
				continue
			}
			out = append(out, thorTxTransfer{
				Index:     idx,
				TxID:      txID,
				Height:    height,
				Time:      when,
				From:      sender,
				To:        recipient,
				Asset:     amount.Asset,
				AmountRaw: amount.AmountRaw,
			})
		}
	}
	return out
}

func parseThorTransferEventAmounts(raw string) []thorTransferAmount {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	out := make([]thorTransferAmount, 0, len(parts))
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
		asset := normalizeProtocolDenomAsset(sourceProtocolTHOR, part[idx:])
		if amount == "" || asset == "" {
			continue
		}
		out = append(out, thorTransferAmount{Asset: asset, AmountRaw: amount})
	}
	return out
}

func (b *graphBuilder) recordMidgardActions(actions []midgardAction) {
	if b == nil || len(actions) == 0 {
		return
	}
	if b.midgardActionsByTx == nil {
		b.midgardActionsByTx = map[string][]midgardAction{}
	}
	if b.recordedActionKeys == nil {
		b.recordedActionKeys = map[string]struct{}{}
	}
	for _, action := range actions {
		key := midgardActionKey(action)
		if key != "" {
			if _, ok := b.recordedActionKeys[key]; ok {
				continue
			}
			b.recordedActionKeys[key] = struct{}{}
		}
		for _, txID := range midgardActionTxIDs(action) {
			if txID == "" {
				continue
			}
			b.midgardActionsByTx[txID] = append(b.midgardActionsByTx[txID], action)
		}
	}
}

func (b *graphBuilder) shouldSkipActionBecauseRujiraTrace(action midgardAction) bool {
	if b == nil || action.Metadata.Contract == nil {
		return false
	}
	txID := traceTxIDForAction(action)
	if txID == "" {
		return false
	}
	transfers, ok := b.thorTxTransfersByTx[txID]
	if !ok || len(transfers) == 0 || !b.hasTraceProjectableRujiraAction(txID) {
		return false
	}
	contractType := strings.ToLower(strings.TrimSpace(action.Metadata.Contract.ContractType))
	return isCalcControlPlaneContractType(contractType) ||
		isInternalRujiraFinancingContractType(contractType) ||
		isNonCapitalRujiraContractType(contractType)
}

func (b *graphBuilder) hasTraceProjectableRujiraAction(txID string) bool {
	for _, action := range b.midgardActionsByTx[cleanTxID(txID)] {
		if action.Metadata.Contract == nil {
			continue
		}
		if isTraceProjectableRujiraContractType(action.Metadata.Contract.ContractType) {
			return true
		}
	}
	return false
}

func (b *graphBuilder) projectRujiraContractActionFromTrace(action midgardAction, contractDesc contractCallDescriptor, baseDepth int) ([]projectedSegment, []frontierAddress, []string, bool) {
	if b == nil || action.Metadata.Contract == nil {
		return nil, nil, nil, false
	}
	contractType := strings.ToLower(strings.TrimSpace(action.Metadata.Contract.ContractType))
	if !isTraceProjectableRujiraContractType(contractType) {
		return nil, nil, nil, false
	}

	txID := traceTxIDForAction(action)
	transfers, ok := b.thorTxTransfersByTx[txID]
	if !ok || len(transfers) == 0 {
		return nil, nil, nil, false
	}

	traceCtx := b.buildThorTxTraceContext(txID)
	actionMeta := describeMidgardAction(action)
	actionClass := actionMeta.ActionClass
	actionProtocol := sourceProtocolFromAction(action)
	actionTime := parseMidgardActionTime(action.Date)
	if actionTime.IsZero() {
		actionTime = time.Now().UTC()
	}
	height := parseInt64(action.Height)
	addressesInAction := make([]string, 0, len(action.In)+len(action.Out))
	for _, leg := range action.In {
		addressesInAction = append(addressesInAction, leg.Address)
	}
	for _, leg := range action.Out {
		addressesInAction = append(addressesInAction, leg.Address)
	}
	actionActorIDs := b.actorIDsForAddresses(addressesInAction...)

	var segments []projectedSegment
	var nextAddresses []frontierAddress

	addTraceSegment := func(source, target flowRef, asset, amount string, confidence float64) {
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
		seg := projectedSegment{
			Source:           source,
			Target:           target,
			SourceProtocol:   actionProtocol,
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
			seg.CanonicalKey = canonicalSwapSegmentKey(txID, source.Address, target.Address, seg.Asset, actionProtocol)
		}
		if b.minUSD > 0 && seg.USDSpot > 0 && seg.USDSpot < b.minUSD && !hasActorIDs(seg.ActorIDs) {
			return
		}
		segments = append(segments, seg)
		for _, ref := range []flowRef{source, target} {
			if shouldExpandAddressRef(ref) {
				nextAddresses = append(nextAddresses, frontierAddress{Address: ref.Address, Chain: ref.Chain, Depth: ref.Depth})
			}
		}
	}

	receiverLegs, _ := splitMidgardContractLegs(action)
	if len(receiverLegs) == 0 {
		return nil, nil, nil, true
	}

	for _, receiverLeg := range receiverLegs {
		contractAddress := normalizeAddress(receiverLeg.Address)
		if contractAddress == "" {
			continue
		}
		contractRef := b.makeContractRef(contractAddress, contractDesc, baseDepth+1)
		if contractRef.ID == "" {
			continue
		}

		for _, transfer := range b.selectOwnedThorTraceTransfersForContractAction(action, contractAddress, transfers, traceCtx, true) {
			sourceRef := b.makeThorTraceRef(transfer.From, baseDepth, traceCtx)
			if sourceRef.ID == "" || sourceRef.Key == contractRef.Key {
				continue
			}
			addTraceSegment(sourceRef, contractRef, transfer.Asset, transfer.AmountRaw, 0.97)
		}

		for _, transfer := range b.selectOwnedThorTraceTransfersForContractAction(action, contractAddress, transfers, traceCtx, false) {
			targetRef := b.makeThorTraceRef(transfer.To, baseDepth+2, traceCtx)
			if targetRef.ID == "" || targetRef.Key == contractRef.Key {
				continue
			}
			addTraceSegment(contractRef, targetRef, transfer.Asset, transfer.AmountRaw, 0.97)
		}
	}

	return segments, uniqueFrontierAddresses(nextAddresses), nil, true
}

func (b *graphBuilder) buildThorTxTraceContext(txID string) thorTxTraceContext {
	ctx := thorTxTraceContext{
		VisibleContracts:   map[string]thorTxVisibleContract{},
		FinancingContracts: map[string]struct{}{},
	}
	for _, action := range b.midgardActionsByTx[cleanTxID(txID)] {
		if action.Metadata.Contract == nil {
			continue
		}
		contractType := strings.ToLower(strings.TrimSpace(action.Metadata.Contract.ContractType))
		role := thorTxTraceRoleForContractType(contractType)
		if role == thorTxTraceRoleUnknown {
			continue
		}
		descriptor := lookupContractCallDescriptor(contractType)
		receiverLegs, _ := splitMidgardContractLegs(action)
		for _, receiverLeg := range receiverLegs {
			address := normalizeAddress(receiverLeg.Address)
			if address == "" {
				continue
			}
			if role == thorTxTraceRoleFinancing {
				ctx.FinancingContracts[address] = struct{}{}
				continue
			}
			existing, ok := ctx.VisibleContracts[address]
			if !ok || thorTxTraceRolePriority(role) > thorTxTraceRolePriority(existing.Role) {
				ctx.VisibleContracts[address] = thorTxVisibleContract{
					Descriptor: descriptor,
					Role:       role,
				}
			}
		}
	}
	return ctx
}

func (b *graphBuilder) selectOwnedThorTraceTransfersForContractAction(action midgardAction, contractAddress string, transfers []thorTxTransfer, traceCtx thorTxTraceContext, inbound bool) []thorTxTransfer {
	if action.Metadata.Contract == nil || len(transfers) == 0 {
		return nil
	}
	contractAddress = normalizeAddress(contractAddress)
	if contractAddress == "" {
		return nil
	}
	currentRole := thorTxTraceRoleForContractType(action.Metadata.Contract.ContractType)
	if currentRole != thorTxTraceRoleMarket && currentRole != thorTxTraceRoleCustody {
		return nil
	}

	visibleByAsset := map[string][]thorTxTransfer{}
	leafByAsset := map[string][]thorTxTransfer{}

	for _, transfer := range transfers {
		counterparty := ""
		if inbound {
			if normalizeAddress(transfer.To) != contractAddress {
				continue
			}
			counterparty = normalizeAddress(transfer.From)
		} else {
			if normalizeAddress(transfer.From) != contractAddress {
				continue
			}
			counterparty = normalizeAddress(transfer.To)
		}
		if counterparty == "" || counterparty == contractAddress {
			continue
		}
		if _, financing := traceCtx.FinancingContracts[counterparty]; financing {
			continue
		}
		if graphExcludedAddresses[counterparty] {
			continue
		}

		counterRole, counterVisible := traceCtx.roleForAddress(counterparty)
		if currentRole == thorTxTraceRoleMarket && inbound && counterVisible && counterRole == thorTxTraceRoleMarket {
			continue
		}

		if counterVisible {
			visibleByAsset[transfer.Asset] = append(visibleByAsset[transfer.Asset], transfer)
			continue
		}
		leafByAsset[transfer.Asset] = append(leafByAsset[transfer.Asset], transfer)
	}

	out := make([]thorTxTransfer, 0)
	for _, items := range visibleByAsset {
		out = append(out, items...)
	}
	for asset, items := range leafByAsset {
		if len(visibleByAsset[asset]) > 0 {
			continue
		}
		if selected, ok := pickLargestThorTxTransfer(items); ok {
			out = append(out, selected)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Index != out[j].Index {
			return out[i].Index < out[j].Index
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		if out[i].Asset != out[j].Asset {
			return out[i].Asset < out[j].Asset
		}
		return out[i].AmountRaw < out[j].AmountRaw
	})
	return out
}

func (b *graphBuilder) makeThorTraceRef(address string, depth int, traceCtx thorTxTraceContext) flowRef {
	address = normalizeAddress(address)
	if address == "" {
		return flowRef{}
	}
	if visible, ok := traceCtx.VisibleContracts[address]; ok {
		return b.makeContractRef(address, visible.Descriptor, depth)
	}
	return b.makeAddressRef(address, "THOR", depth)
}

func (t thorTxTraceContext) roleForAddress(address string) (thorTxTraceRole, bool) {
	visible, ok := t.VisibleContracts[normalizeAddress(address)]
	if !ok {
		return thorTxTraceRoleUnknown, false
	}
	return visible.Role, true
}

func isTraceProjectableRujiraContractType(contractType string) bool {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	if !strings.HasPrefix(contractType, "wasm-rujira-") {
		return false
	}
	if isInternalRujiraFinancingContractType(contractType) {
		return false
	}
	if isLiquidityFeeContractType(contractType) {
		return false
	}
	desc := lookupContractCallDescriptor(contractType)
	if desc.FundsMove || desc.UserFacing {
		return true
	}
	family, _ := splitContractType(contractType)
	return isTraceProjectableRujiraFamily(family)
}

func isInternalRujiraFinancingContractType(contractType string) bool {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	switch contractType {
	case "wasm-rujira-ghost-vault/borrow",
		"wasm-rujira-ghost-vault/repay",
		"wasm-rujira-ghost-mint/borrow",
		"wasm-rujira-ghost-mint/repay":
		return true
	default:
		return false
	}
}

func isCalcControlPlaneContractType(contractType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(contractType)), "wasm-calc-")
}

func isLiquidityFeeContractType(contractType string) bool {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	return strings.Contains(contractType, ".fee") || strings.Contains(contractType, "market-maker.fee")
}

func isNonCapitalRujiraContractType(contractType string) bool {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	if !strings.HasPrefix(contractType, "wasm-rujira-") {
		return false
	}
	if isTraceProjectableRujiraContractType(contractType) || isLiquidityFeeContractType(contractType) || isInternalRujiraFinancingContractType(contractType) {
		return false
	}
	desc := lookupContractCallDescriptor(contractType)
	switch desc.Access {
	case "admin-control":
		return true
	}
	switch desc.Domain {
	case "utility", "ops":
		return true
	default:
		return false
	}
}

func isTraceProjectableRujiraFamily(family string) bool {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "rujira-bow",
		"rujira-fin",
		"rujira-ghost-credit",
		"rujira-ghost-mint",
		"rujira-ghost-vault",
		"rujira-merge",
		"rujira-pilot",
		"rujira-revenue",
		"rujira-staking",
		"rujira-thorchain-swap":
		return true
	default:
		return false
	}
}

func isMarketRujiraContractType(contractType string) bool {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	switch contractType {
	case "wasm-rujira-fin/trade",
		"wasm-rujira-thorchain-swap/swap",
		"wasm-rujira-bow/swap",
		"wasm-rujira-pilot/swap":
		return true
	default:
		return false
	}
}

func thorTxTraceRoleForContractType(contractType string) thorTxTraceRole {
	contractType = strings.ToLower(strings.TrimSpace(contractType))
	switch {
	case strings.HasPrefix(contractType, "wasm-calc-"):
		return thorTxTraceRoleControl
	case isInternalRujiraFinancingContractType(contractType):
		return thorTxTraceRoleFinancing
	case isMarketRujiraContractType(contractType):
		return thorTxTraceRoleMarket
	case isTraceProjectableRujiraContractType(contractType):
		return thorTxTraceRoleCustody
	default:
		return thorTxTraceRoleUnknown
	}
}

func thorTxTraceRolePriority(role thorTxTraceRole) int {
	switch role {
	case thorTxTraceRoleMarket:
		return 3
	case thorTxTraceRoleCustody:
		return 2
	case thorTxTraceRoleControl:
		return 1
	default:
		return 0
	}
}

func traceTxIDForAction(action midgardAction) string {
	for _, txID := range midgardActionTxIDs(action) {
		if txID != "" {
			return txID
		}
	}
	return cleanTxID(midgardSyntheticTxID(action))
}

func pickLargestThorTxTransfer(transfers []thorTxTransfer) (thorTxTransfer, bool) {
	if len(transfers) == 0 {
		return thorTxTransfer{}, false
	}
	best := transfers[0]
	for _, candidate := range transfers[1:] {
		if compareNumericStrings(candidate.AmountRaw, best.AmountRaw) > 0 {
			best = candidate
			continue
		}
		if compareNumericStrings(candidate.AmountRaw, best.AmountRaw) == 0 && candidate.Index < best.Index {
			best = candidate
		}
	}
	return best, true
}

func compareNumericStrings(left, right string) int {
	leftInt, ok := new(big.Int).SetString(strings.TrimSpace(left), 10)
	if !ok {
		leftInt = big.NewInt(0)
	}
	rightInt, ok := new(big.Int).SetString(strings.TrimSpace(right), 10)
	if !ok {
		rightInt = big.NewInt(0)
	}
	return leftInt.Cmp(rightInt)
}
