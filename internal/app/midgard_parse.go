package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func thorDenomToAsset(denom string) string {
	return normalizeTHORDenomAsset(denom)
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
	nativeAsset := nativeAssetForProtocol(sourceProtocolFromAction(action))
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
				if asset != nativeAsset {
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
	protocol := sourceProtocolFromAction(action)
	if strings.TrimSpace(outLeg.TxID) != "" &&
		strings.TrimSpace(inLeg.TxID) != "" &&
		!strings.EqualFold(strings.TrimSpace(outLeg.TxID), strings.TrimSpace(inLeg.TxID)) {
		return "", "", false
	}
	if preferContractFundsAmount(contract.ContractType) {
		if asset, amount, ok := parseContractFunds(contract.Funds, protocol); ok {
			return asset, amount, true
		}
	}
	if asset, amount, ok := findSwapAmountInValue(contract.Msg, protocol); ok {
		return asset, amount, true
	}
	if asset, amount, ok := parseContractFunds(contract.Funds, protocol); ok {
		return asset, amount, true
	}
	return "", "", false
}

func findContractSwapAmount(msg map[string]any) (string, string, bool) {
	if len(msg) == 0 {
		return "", "", false
	}
	return findSwapAmountInValue(msg, sourceProtocolTHOR)
}

func findSwapAmountInValue(value any, protocol string) (string, string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if rawSwap, ok := typed["swap_amount"]; ok {
			if swapMap, ok := rawSwap.(map[string]any); ok {
				amount := stringifyAny(swapMap["amount"])
				denom := normalizeProtocolDenomAsset(protocol, stringifyAny(swapMap["denom"]))
				if denom != "" && amount != "" {
					return denom, amount, true
				}
			}
		}
		for _, child := range typed {
			if asset, amount, ok := findSwapAmountInValue(child, protocol); ok {
				return asset, amount, true
			}
		}
	case []any:
		for _, child := range typed {
			if asset, amount, ok := findSwapAmountInValue(child, protocol); ok {
				return asset, amount, true
			}
		}
	}
	return "", "", false
}

func parseContractFunds(raw, protocol string) (string, string, bool) {
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
		denom := normalizeProtocolDenomAsset(protocol, part[idx:])
		if amount == "" || denom == "" {
			continue
		}
		return denom, amount, true
	}
	return "", "", false
}

func normalizeContractDenom(denom string) string {
	return normalizeProtocolDenomAsset(sourceProtocolTHOR, denom)
}

func normalizeTHORDenomAsset(denom string) string {
	return normalizeProtocolDenomAsset(sourceProtocolTHOR, denom)
}

func normalizeProtocolDenomAsset(protocol, denom string) string {
	denom = strings.TrimSpace(denom)
	if denom == "" {
		return ""
	}
	lower := strings.ToLower(denom)
	switch lower {
	case "rune":
		return "THOR.RUNE"
	case "cacao":
		return "MAYA.CACAO"
	case "tcy":
		return "THOR.TCY"
	}
	protocol = normalizeSourceProtocol(protocol)
	prefix := protocol
	if prefix == "" {
		prefix = sourceProtocolTHOR
	}
	if strings.HasPrefix(lower, "x/") {
		symbol := strings.TrimSpace(denom[2:])
		if symbol == "" {
			return ""
		}
		if asset := normalizeAsset(symbol); strings.Contains(asset, ".") {
			return asset
		}
		return prefix + "." + strings.ToUpper(symbol)
	}
	asset := normalizeAsset(denom)
	if strings.Contains(asset, ".") {
		return asset
	}
	return prefix + "." + strings.ToUpper(denom)
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
		strings.HasPrefix(lower, "maya") ||
		strings.HasPrefix(lower, "bc1") ||
		strings.HasPrefix(lower, "ltc") ||
		strings.HasPrefix(lower, "account_rdx") ||
		strings.HasPrefix(lower, "bitcoincash:") ||
		strings.HasPrefix(candidate, "T") ||
		isLikelyDOGEAddress(candidate) {
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
	if candidate == "" || (!strings.HasPrefix(candidate, "thor") && !strings.HasPrefix(candidate, "maya")) {
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

// cleanTxID normalises a transaction ID to the form Midgard reports, so IDs
// from external trackers and Midgard legs compare equal: trimmed, upper-case,
// and without the 0x prefix that EVM explorers put on 32-byte hashes. Other
// IDs keep their characters. A zero hash means no transaction.
func cleanTxID(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if hash, ok := strings.CutPrefix(raw, "0X"); ok && isHexTxHash(hash) {
		raw = hash
	}
	if raw == "" || isZeroTxID(raw) {
		return ""
	}
	return raw
}

// isHexTxHash reports whether an upper-cased ID is a 32-byte hash written as
// 64 hex digits, the shape of an EVM transaction hash after its 0x prefix.
func isHexTxHash(txID string) bool {
	if len(txID) != 64 {
		return false
	}
	for _, r := range txID {
		if (r < '0' || r > '9') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
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

func canonicalSwapSegmentKey(txID, source, target, asset, protocol string) string {
	txID = cleanTxID(txID)
	source = normalizeAddress(source)
	target = normalizeAddress(target)
	asset = normalizeAsset(asset)
	if txID == "" || source == "" || target == "" || source == target {
		return ""
	}
	if asset == "" {
		asset = nativeAssetForProtocol(protocol)
	}
	return strings.Join([]string{"swap", txID, source, target, asset}, "|")
}

func normalizeAsset(asset string) string {
	trimmed := strings.TrimSpace(asset)
	upper := strings.ToUpper(trimmed)
	if upper == "" || strings.Contains(upper, ".") {
		return upper
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "rune":
		return "THOR.RUNE"
	case "cacao":
		return "MAYA.CACAO"
	}
	if strings.HasPrefix(lower, "x/") {
		return normalizeContractDenom(trimmed)
	}
	parts := strings.SplitN(upper, "-", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + "." + parts[1]
	}
	return upper
}

func isLikelyDOGEAddress(address string) bool {
	address = strings.TrimSpace(address)
	if len(address) < 26 || len(address) > 40 {
		return false
	}
	if !strings.HasPrefix(address, "D") {
		return false
	}
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	for _, r := range address {
		if !strings.ContainsRune(alphabet, r) {
			return false
		}
	}
	return true
}

func normalizeChain(chain, address string) string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	address = strings.TrimSpace(address)
	lower := strings.ToLower(address)
	inferred := ""
	switch {
	case strings.HasPrefix(lower, "maya"):
		inferred = "MAYA"
	case strings.HasPrefix(lower, "thor"):
		inferred = "THOR"
	case isLikelyEVMAddress(address):
		inferred = "ETH"
	case isLikelyDOGEAddress(address):
		inferred = "DOGE"
	case strings.HasPrefix(lower, "bc1"):
		inferred = "BTC"
	case strings.HasPrefix(lower, "ltc1"):
		inferred = "LTC"
	case strings.HasPrefix(lower, "bitcoincash:"):
		inferred = "BCH"
	case strings.HasPrefix(lower, "cosmos1"):
		inferred = "GAIA"
	case strings.HasPrefix(lower, "account_rdx"),
		strings.HasPrefix(lower, "component_rdx"),
		strings.HasPrefix(lower, "resource_rdx"):
		inferred = "XRD"
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

func mergeNodeSourceProtocol(node *FlowNode, protocol string) {
	if node == nil {
		return
	}
	protocol = normalizeSourceProtocol(protocol)
	if protocol == "" {
		return
	}
	if node.Metrics == nil {
		node.Metrics = map[string]any{}
	}
	values := stringSliceMetric(node.Metrics["source_protocols"])
	values = appendUniqueString(values, protocol)
	node.Metrics["source_protocols"] = values
	if existing := strings.TrimSpace(getString(node.Metrics, "source_protocol")); existing == "" {
		node.Metrics["source_protocol"] = protocol
	}
}

func stringSliceMetric(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringifyAny(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func mergeEdgeTransactionAsset(edge *FlowEdge, txID, sourceProtocol string, height int64, when time.Time, asset, amountRaw string, usd float64, meta assetMetadata, direction string) {
	txID = strings.TrimSpace(txID)
	sourceProtocol = normalizeSourceProtocol(sourceProtocol)
	for i := range edge.Transactions {
		tx := &edge.Transactions[i]
		if tx.TxID != txID || normalizeSourceProtocol(tx.SourceProtocol) != sourceProtocol {
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
		TxID:           txID,
		SourceProtocol: sourceProtocol,
		Height:         height,
		Time:           when,
		USDSpot:        usd,
		Assets:         []FlowAssetValue{},
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
		if tx.InboundTxID != "" {
			edge.TxIDs = appendUniqueString(edge.TxIDs, tx.InboundTxID)
		}
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
	seen := map[string]frontierAddress{}
	for _, item := range in {
		if item.Address == "" {
			continue
		}
		key := frontierKey(item.Chain, item.Address)
		if key == "" {
			continue
		}
		if existing, ok := seen[key]; ok {
			if item.Depth > 0 && (existing.Depth == 0 || item.Depth < existing.Depth) {
				existing.Depth = item.Depth
				seen[key] = existing
			}
			continue
		}
		item.Address = normalizeAddress(item.Address)
		item.Chain = normalizeChain(item.Chain, item.Address)
		seen[key] = item
	}
	out := make([]frontierAddress, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return frontierKey(out[i].Chain, out[i].Address) < frontierKey(out[j].Chain, out[j].Address)
	})
	return out
}

func maxGraphNodeDepth(maxHops int) int {
	if maxHops < 1 {
		return 2
	}
	return maxHops + 1
}

func filterProjectedSegmentsToMaxDepth(segments []projectedSegment, maxDepth int) ([]projectedSegment, []frontierAddress) {
	if maxDepth <= 0 || len(segments) == 0 {
		return segments, nil
	}
	filtered := make([]projectedSegment, 0, len(segments))
	nextAddresses := make([]frontierAddress, 0, len(segments))
	for _, segment := range segments {
		if segment.Source.Depth > maxDepth || segment.Target.Depth > maxDepth {
			continue
		}
		filtered = append(filtered, segment)
		for _, ref := range []flowRef{segment.Source, segment.Target} {
			if shouldExpandAddressRef(ref) && ref.Depth < maxDepth {
				nextAddresses = append(nextAddresses, frontierAddress{
					Address: ref.Address,
					Chain:   ref.Chain,
					Depth:   ref.Depth,
				})
			}
		}
	}
	return filtered, uniqueFrontierAddresses(nextAddresses)
}

func filterProjectedSegmentsToFrontierStep(segments []projectedSegment, frontier frontierAddress, nextDepth, maxDepth int) ([]projectedSegment, []frontierAddress) {
	if len(segments) == 0 {
		return nil, nil
	}
	frontier = normalizeFrontierAddress(encodeFrontierAddress(frontier))
	if frontier.Address == "" {
		return segments, nil
	}
	frontierAddrKey := frontierKey(frontier.Chain, frontier.Address)
	filtered := make([]projectedSegment, 0, len(segments))
	nextAddresses := make([]frontierAddress, 0, len(segments))
	for _, segment := range segments {
		matchesSource := flowRefMatchesFrontier(frontierAddrKey, frontier, segment.Source)
		matchesTarget := flowRefMatchesFrontier(frontierAddrKey, frontier, segment.Target)
		if !matchesSource && !matchesTarget {
			continue
		}
		filtered = append(filtered, segment)
		if nextDepth > maxDepth {
			continue
		}
		var candidate flowRef
		switch {
		case matchesSource && !matchesTarget:
			candidate = segment.Target
		case matchesTarget && !matchesSource:
			candidate = segment.Source
		default:
			continue
		}
		if !shouldExpandAddressRef(candidate) || flowRefMatchesFrontier(frontierAddrKey, frontier, candidate) {
			continue
		}
		nextAddresses = append(nextAddresses, frontierAddress{
			Address: candidate.Address,
			Chain:   candidate.Chain,
			Depth:   nextDepth,
		})
	}
	return filtered, uniqueFrontierAddresses(nextAddresses)
}

func flowRefMatchesFrontier(frontierAddrKey string, frontier frontierAddress, ref flowRef) bool {
	if frontier.Address == "" || ref.Address == "" {
		return false
	}
	if frontierAddrKey != "" {
		if refAddrKey := frontierKey(ref.Chain, ref.Address); refAddrKey != "" && refAddrKey == frontierAddrKey {
			return true
		}
	}
	return normalizeAddress(ref.Address) == normalizeAddress(frontier.Address)
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
