package app

import (
	"sort"
	"strings"
	"time"
)

func (b *graphBuilder) projectMidgardAction(action midgardAction, baseDepth int) ([]projectedSegment, []frontierAddress, []string) {
	segments, next, warnings, _ := b.projectMidgardActionWithExternal(action, baseDepth, nil)
	return segments, next, warnings
}

func (b *graphBuilder) projectMidgardActionWithExternal(action midgardAction, baseDepth int, externalTransfers []externalTransfer) ([]projectedSegment, []frontierAddress, []string, map[string]struct{}) {
	action, consumedExternalTransfers := b.stitchMidgardAction(action, externalTransfers)
	actionMeta := describeMidgardAction(action)
	actionClass := actionMeta.ActionClass
	actionProtocol := sourceProtocolFromAction(action)
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
			SourceProtocol:   actionProtocol,
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
			seg.CanonicalKey = canonicalSwapSegmentKey(firstNonEmpty(swapTxID, seg.TxID), source.Address, target.Address, seg.Asset, actionProtocol)
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

	if strings.EqualFold(strings.TrimSpace(action.Type), "contract") {
		contractDesc := lookupContractCallDescriptor(actionMeta.ContractType)
		if tracedSegments, tracedNext, tracedWarnings, handled := b.projectRujiraContractActionFromTrace(action, contractDesc, baseDepth); handled {
			return tracedSegments, tracedNext, tracedWarnings, consumedExternalTransfers
		}
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
			return nativeAssetForProtocol(actionProtocol), "0"
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
			walletRef := b.makeBondWalletRef(walletAddress, actionProtocol, baseDepth)
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
				targetRef := b.makeBondWalletRef(midgardRebondNewBondAddress(action), actionProtocol, baseDepth+1)
				if targetRef.ID == "" || targetRef.Key == walletRef.Key {
					continue
				}
				before := len(segments)
				addSegment(walletRef, targetRef, asset, amount, 0.9, txID)
				for i := before; i < len(segments); i++ {
					segments[i].ValidatorAddress = nodeAddress
					segments[i].ValidatorLabel = protocolBondDisplayLabel(actionProtocol, nodeAddress, "")
				}
				continue
			}
			nodeRef := b.makeNodeRef(nodeAddress, actionProtocol, baseDepth+1)
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
		swapOutLegs, suppressedFeeLegs := selectMidgardSwapOutLegs(actionProtocol, legsOut)
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
		poolRef := b.makePoolRef(midgardActionPool(action), actionProtocol, baseDepth+1)
		if poolRef.ID != "" {
			for _, inLeg := range legsIn {
				source := b.makeAddressRef(inLeg.Address, chainFromMidgardCoins(inLeg.Coins), baseDepth)
				txID := strings.ToUpper(strings.TrimSpace(inLeg.TxID))
				if len(inLeg.Coins) == 0 {
					if inferredAsset, inferredAmount, ok := inferContractLegAmount(action, inLeg, midgardActionLeg{}); ok {
						addSegment(source, poolRef, inferredAsset, inferredAmount, 0.72, txID)
					} else {
						addSegment(source, poolRef, nativeAssetForProtocol(actionProtocol), "0", 0.62, txID)
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
						addSegment(poolRef, target, nativeAssetForProtocol(actionProtocol), "0", 0.6, txID)
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
					addSegment(source, target, nativeAssetForProtocol(actionProtocol), "0", 0.62, txID)
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

func selectMidgardSwapOutLegs(protocol string, legsOut []midgardActionLeg) ([]midgardActionLeg, int) {
	if len(legsOut) <= 1 {
		return legsOut, 0
	}
	if !midgardSwapOutLegsHaveExplicitTx(legsOut) {
		return legsOut, 0
	}
	filtered := make([]midgardActionLeg, 0, len(legsOut))
	suppressed := 0
	for _, leg := range legsOut {
		if isMidgardSuppressedSwapOutLeg(protocol, legsOut, leg) {
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

func shouldSkipMidgardActionForFeeOnlyFrontier(action midgardAction, frontierAddress string) bool {
	frontierAddress = normalizeAddress(frontierAddress)
	if frontierAddress == "" || describeMidgardAction(action).ActionClass != "swaps" {
		return false
	}
	for _, leg := range action.In {
		if normalizeAddress(leg.Address) == frontierAddress {
			return false
		}
	}
	if !midgardSwapOutLegsHaveExplicitTx(action.Out) {
		return false
	}
	protocol := sourceProtocolFromAction(action)
	matchedFeeLikeOut := false
	for _, leg := range action.Out {
		if normalizeAddress(leg.Address) != frontierAddress {
			continue
		}
		if isMidgardSuppressedSwapOutLeg(protocol, action.Out, leg) {
			matchedFeeLikeOut = true
			continue
		}
		return false
	}
	return matchedFeeLikeOut
}

func midgardSwapOutLegsHaveExplicitTx(legsOut []midgardActionLeg) bool {
	for _, leg := range legsOut {
		if cleanTxID(leg.TxID) != "" {
			return true
		}
	}
	return false
}

func isMidgardSuppressedSwapOutLeg(protocol string, legsOut []midgardActionLeg, leg midgardActionLeg) bool {
	if cleanTxID(leg.TxID) != "" || !midgardSwapOutLegsHaveExplicitTx(legsOut) {
		return false
	}
	return isMidgardFeeLikeSwapOutLegForProtocol(protocol, leg)
}

func isMidgardFeeLikeSwapOutLeg(leg midgardActionLeg) bool {
	return isMidgardFeeLikeSwapOutLegForProtocol(sourceProtocolTHOR, leg)
}

func isMidgardFeeLikeSwapOutLegForProtocol(protocol string, leg midgardActionLeg) bool {
	if normalizeChain("", leg.Address) != normalizeChain("", protocolAddressPrefix(protocol)+"1dummy") {
		return false
	}
	if len(leg.Coins) == 0 {
		return false
	}
	for _, coin := range leg.Coins {
		if normalizeAsset(coin.Asset) != nativeAssetForProtocol(protocol) {
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

func inferContractActionClass(action midgardAction) string {
	contract := action.Metadata.Contract
	if contract == nil {
		return "liquidity"
	}
	if _, _, ok := findSwapAmountInValue(contract.Msg, sourceProtocolFromAction(action)); ok {
		return "swaps"
	}

	if class := inferContractActionClassFromType(contract.ContractType); class != "" {
		return class
	}

	if _, _, ok := parseContractFunds(contract.Funds, sourceProtocolFromAction(action)); ok {
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
	contractRoute := ""
	if actionType == "contract" && action.Metadata.Contract != nil {
		contractType = strings.ToLower(strings.TrimSpace(action.Metadata.Contract.ContractType))
		contractRoute = midgardContractActionRouteKey(action)
	}
	return strings.Join([]string{
		actionType,
		contractType,
		contractRoute,
		strings.TrimSpace(action.Height),
		strings.TrimSpace(action.Date),
		strings.Join(txIDs, ","),
	}, "|")
}

func midgardContractActionRouteKey(action midgardAction) string {
	receivers, payouts := splitMidgardContractLegs(action)
	receiverKey := strings.Join(midgardActionLegAddressKey(receivers), ",")
	payoutKey := strings.Join(midgardActionLegAddressKey(payouts), ",")
	if receiverKey == "" && payoutKey == "" {
		return ""
	}
	return receiverKey + "->" + payoutKey
}

func midgardActionLegAddressKey(legs []midgardActionLeg) []string {
	if len(legs) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(legs))
	for _, leg := range legs {
		address := normalizeAddress(leg.Address)
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		out = append(out, address)
	}
	sort.Strings(out)
	return out
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
