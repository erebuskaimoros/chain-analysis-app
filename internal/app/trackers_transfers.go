package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) fetchRadixTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	baseURLs := a.cfg.radixGatewayURLs()
	if len(baseURLs) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}
	address = normalizeAddress(address)
	cursor := ""
	var all []externalTransfer
	truncated := false
	for page := 0; page < maxPages; page++ {
		payload := map[string]any{
			"affected_global_entities_filter": []string{address},
			"limit":                           externalTrackerPageSize,
			"opt_ins": map[string]any{
				"balance_changes": true,
			},
		}
		if strings.TrimSpace(cursor) != "" {
			payload["cursor"] = cursor
		}
		var resp radixStreamTransactionsResponse
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return strings.TrimRight(baseURL, "/") + "/stream/transactions"
		})
		if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, payload, &resp); err != nil {
			return nil, false, err
		}
		if len(resp.Items) == 0 {
			break
		}
		for _, item := range resp.Items {
			ts, _ := time.Parse(time.RFC3339, strings.TrimSpace(item.ConfirmedAt))
			if !ts.IsZero() && ts.Before(start) {
				return dedupeExternalTransfers(all), truncated, nil
			}
			if !ts.IsZero() && ts.After(end) {
				continue
			}
			all = append(all, inferRadixTransfers(address, item.IntentHash, item.StateVersion, ts, item.BalanceChanges.FungibleBalanceChanges)...)
		}
		cursor = strings.TrimSpace(resp.NextCursor)
		if cursor == "" {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, nil
}

func inferRadixTransfers(address, txID string, height int64, ts time.Time, changes []struct {
	EntityAddress   string `json:"entity_address"`
	ResourceAddress string `json:"resource_address"`
	BalanceChange   string `json:"balance_change"`
}) []externalTransfer {
	address = normalizeAddress(address)
	if address == "" || len(changes) == 0 {
		return nil
	}
	positives := map[string]int64{}
	negatives := map[string]int64{}
	for _, change := range changes {
		if !strings.EqualFold(strings.TrimSpace(change.ResourceAddress), radixMainnetXRDResourceAddr) {
			continue
		}
		entity := normalizeAddress(change.EntityAddress)
		amount, sign := signedDecimalAmountToGraphRaw(change.BalanceChange)
		if entity == "" || amount <= 0 || sign == 0 {
			continue
		}
		if sign > 0 {
			positives[entity] += amount
		} else {
			negatives[entity] += amount
		}
	}
	if negatives[address] > 0 {
		recipients := map[string]int64{}
		for entity, amount := range positives {
			if entity == address {
				continue
			}
			recipients[entity] += amount
		}
		return inferRadixDirectionalTransfers(address, txID, height, ts, negatives[address], recipients, true)
	}
	if positives[address] > 0 {
		senders := map[string]int64{}
		for entity, amount := range negatives {
			if entity == address {
				continue
			}
			senders[entity] += amount
		}
		return inferRadixDirectionalTransfers(address, txID, height, ts, positives[address], senders, false)
	}
	return nil
}

func inferRadixDirectionalTransfers(watched, txID string, height int64, ts time.Time, watchedAmount int64, counterparties map[string]int64, outgoing bool) []externalTransfer {
	if watchedAmount <= 0 || len(counterparties) == 0 {
		return nil
	}
	keys := make([]string, 0, len(counterparties))
	total := int64(0)
	for entity, amount := range counterparties {
		if amount <= 0 {
			continue
		}
		keys = append(keys, entity)
		total += amount
	}
	if len(keys) == 0 || total <= 0 {
		return nil
	}
	sort.Strings(keys)
	out := make([]externalTransfer, 0, len(keys))
	remaining := watchedAmount
	for i, entity := range keys {
		share := (watchedAmount * counterparties[entity]) / total
		if i == len(keys)-1 {
			share = remaining
		} else {
			remaining -= share
		}
		if share <= 0 {
			continue
		}
		transfer := externalTransfer{
			Chain:       "XRD",
			Asset:       "XRD.XRD",
			AmountRaw:   strconv.FormatInt(share, 10),
			TxID:        txID,
			Height:      height,
			Time:        ts,
			ActionKey:   "tracker.xrd.transfer",
			ActionLabel: "XRD Transfer (Inferred)",
			Confidence:  0.7,
		}
		if outgoing {
			transfer.From = watched
			transfer.To = entity
		} else {
			transfer.From = entity
			transfer.To = watched
			transfer.Confidence = 0.78
		}
		out = append(out, transfer)
	}
	return out
}

func decimalAmountToGraphRaw(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "+")
	raw = strings.TrimPrefix(raw, "-")
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, ".", 2)
	whole := ""
	fractional := ""
	if len(parts) > 0 {
		whole = parts[0]
	}
	if len(parts) > 1 {
		fractional = parts[1]
	}
	return strconv.FormatInt(humanPartsToRaw(whole, fractional, 8), 10)
}

func signedDecimalAmountToGraphRaw(raw string) (int64, int) {
	raw = strings.TrimSpace(raw)
	sign := 1
	switch {
	case strings.HasPrefix(raw, "-"):
		sign = -1
	case strings.HasPrefix(raw, "+"):
		sign = 1
	default:
		sign = 1
	}
	amountRaw := parseInt64(decimalAmountToGraphRaw(raw))
	if amountRaw <= 0 {
		return 0, 0
	}
	return amountRaw, sign
}

func (a *App) fetchExternalTransfersForAddress(ctx context.Context, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, string, error) {
	chain = normalizeChain(chain, address)
	address = normalizeAddress(address)
	providers := a.cfg.trackerProvidersForChain(chain)
	if len(providers) == 0 {
		return nil, false, "", nil
	}

	if maxPages < 1 {
		maxPages = 1
	}

	startTS := start.Unix()
	if startTS < 0 {
		startTS = 0
	}
	endTS := end.Unix()
	if endTS < startTS {
		endTS = startTS
	}

	var (
		lastErr            error
		lastWarn           string
		unavailableWarning string
		skippedDegraded    []string
		failedProviders    []string
	)
	for idx, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		switch provider {
		case "", "none", "disabled":
			continue
		}
		providerCtx := withTrackerRequestMeta(ctx, provider, chain)
		if a.trackerHealth.isDegraded(provider, chain) && idx < len(providers)-1 {
			skippedDegraded = append(skippedDegraded, provider)
			continue
		}
		source := ledgerTransferSource(provider, chain)
		covered, err := loadLedgerCoverage(providerCtx, a.db, source, address)
		if err != nil {
			lastErr = err
			failedProviders = append(failedProviders, provider)
			continue
		}
		gaps := ledgerGaps(covered, startTS, endTS)
		if len(gaps) == 0 {
			if cached, err := queryLedgerTransfers(providerCtx, a.db, source, address, startTS, endTS); err == nil {
				a.trackerHealth.markCache(provider, chain, true)
				return dedupeExternalTransfers(cached), false, externalTrackerFallbackWarning(chain, provider, skippedDegraded, failedProviders), nil
			}
		}
		a.trackerHealth.markCache(provider, chain, false)

		truncated, warn, err := a.fetchLedgerTransferGaps(providerCtx, provider, chain, address, source, gaps, maxPages)
		if errors.Is(err, errExternalTrackerUnavailable) {
			if unavailableWarning == "" {
				unavailableWarning = externalTrackerUnavailableWarning(chain, provider)
			}
			failedProviders = append(failedProviders, provider)
			continue
		}
		if err != nil {
			lastErr = err
			lastWarn = warn
			failedProviders = append(failedProviders, provider)
			continue
		}
		transfers, err := queryLedgerTransfers(providerCtx, a.db, source, address, startTS, endTS)
		if err != nil {
			lastErr = err
			failedProviders = append(failedProviders, provider)
			continue
		}
		warn = firstNonEmpty(warn, externalTrackerFallbackWarning(chain, provider, skippedDegraded, failedProviders))
		return dedupeExternalTransfers(transfers), truncated, warn, nil
	}

	if unavailableWarning != "" && lastErr == nil {
		return nil, false, unavailableWarning, nil
	}
	if lastErr != nil {
		return nil, false, firstNonEmpty(lastWarn, externalTrackerFallbackWarning(chain, "", skippedDegraded, failedProviders)), lastErr
	}
	return nil, false, "", nil
}

// fetchLedgerTransferGaps fetches each uncovered range from one provider and
// stores the results. Ranges whose fetch was truncated are stored but not
// marked covered, because providers differ in which end they truncate.
func (a *App) fetchLedgerTransferGaps(ctx context.Context, provider, chain, address, source string, gaps []ledgerInterval, maxPages int) (bool, string, error) {
	truncatedAny := false
	var lastWarn string
	for _, gap := range gaps {
		fetchedAt := time.Now().UTC()
		transfers, truncated, warn, err := a.fetchExternalTransfersWithProvider(ctx, provider, chain, address, time.Unix(gap.From, 0).UTC(), time.Unix(gap.To, 0).UTC(), maxPages)
		if err != nil {
			return truncatedAny, warn, err
		}
		lastWarn = firstNonEmpty(warn, lastWarn)
		transfers = dedupeExternalTransfers(transfers)
		if err := upsertLedgerTransfers(ctx, a.db, source, address, transfers); err != nil {
			logError(ctx, "external_transfer_cache_write_failed", err, map[string]any{"provider": provider, "chain": chain, "address": address})
			continue
		}
		if truncated {
			truncatedAny = true
			continue
		}
		if err := markLedgerCovered(ctx, a.db, source, address, gap.From, ledgerCoverageEnd(gap.To, fetchedAt), fetchedAt); err != nil {
			logError(ctx, "external_transfer_cache_write_failed", err, map[string]any{"provider": provider, "chain": chain, "address": address})
		}
	}
	return truncatedAny, lastWarn, nil
}

func externalTrackerUnavailableWarning(chain, provider string) string {
	chain = firstNonEmpty(chain, "external")
	switch provider {
	case "etherscan":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_ETHERSCAN_API_KEY to follow native-chain flows", chain)
	case "utxo":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_UTXO_TRACKERS to follow native-chain flows", chain)
	case "cosmos":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_COSMOS_TRACKERS to follow native-chain flows", chain)
	case "trongrid":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_TRONGRID_URL to follow native-chain flows", chain)
	case "solana":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_SOLANA_RPC_URL to follow native-chain flows", chain)
	case "xrpl":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_XRP_RPC_URL to follow native-chain flows", chain)
	case "radix":
		return fmt.Sprintf("%s tracker unavailable; configure CHAIN_ANALYSIS_RADIX_GATEWAY_URL to follow native-chain flows", chain)
	default:
		return fmt.Sprintf("%s tracker unavailable", chain)
	}
}

func externalTrackerFallbackWarning(chain, provider string, skippedDegraded, failedProviders []string) string {
	chain = firstNonEmpty(chain, "external")
	skippedDegraded = dedupeProviders(skippedDegraded)
	failedProviders = dedupeProviders(failedProviders)
	switch {
	case provider != "" && len(skippedDegraded) > 0:
		return fmt.Sprintf("%s tracker fell back to %s after skipping degraded provider(s): %s", chain, provider, strings.Join(skippedDegraded, ", "))
	case provider != "" && len(failedProviders) > 0:
		return fmt.Sprintf("%s tracker fell back to %s after provider failure(s): %s", chain, provider, strings.Join(failedProviders, ", "))
	case provider == "" && len(skippedDegraded) > 0:
		return fmt.Sprintf("%s tracker skipped degraded provider(s): %s", chain, strings.Join(skippedDegraded, ", "))
	case provider == "" && len(failedProviders) > 0:
		return fmt.Sprintf("%s tracker provider failure(s): %s", chain, strings.Join(failedProviders, ", "))
	default:
		return ""
	}
}

func (a *App) fetchExternalTransfersWithProvider(ctx context.Context, provider, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, string, error) {
	switch provider {
	case "etherscan", "blockscout", "avacloud", "nodereal":
		transfers, truncated, err := a.fetchEVMTransfers(ctx, provider, chain, address, start, end, maxPages)
		return transfers, truncated, "", err
	case "utxo":
		transfers, truncated, err := a.fetchEsploraTransfers(ctx, chain, address, start, end, maxPages)
		return transfers, truncated, "", err
	case "solana":
		return a.fetchSolanaTransfers(ctx, address, start, end, maxPages)
	case "trongrid":
		return a.fetchTronTransfers(ctx, address, start, end, maxPages)
	case "xrpl":
		return a.fetchXRPLTransfers(ctx, address, start, end, maxPages)
	case "cosmos":
		transfers, truncated, err := a.fetchGaiaTransfers(ctx, address, start, end, maxPages)
		return transfers, truncated, "", err
	case "radix":
		transfers, truncated, err := a.fetchRadixTransfers(ctx, address, start, end, maxPages)
		return transfers, truncated, "", err
	default:
		return nil, false, fmt.Sprintf("%s tracker provider %q unavailable", firstNonEmpty(chain, "external"), provider), errExternalTrackerUnavailable
	}
}

func (b *graphBuilder) projectExternalTransfer(transfer externalTransfer, baseDepth int) ([]projectedSegment, []frontierAddress) {
	actionKey := firstNonEmpty(transfer.ActionKey, "tracker.transfer")
	actionLabel := firstNonEmpty(transfer.ActionLabel, "Native Transfer")
	if !b.prices.supportsGraphAsset(transfer.Asset) {
		return nil, nil
	}
	meta := mergeAssetMetadata(assetMetadata{
		AssetKind:     transfer.AssetKind,
		TokenStandard: transfer.TokenStandard,
		TokenAddress:  transfer.TokenAddress,
		TokenSymbol:   transfer.TokenSymbol,
		TokenName:     transfer.TokenName,
		TokenDecimals: transfer.TokenDecimals,
	}, assetMetadataFromAsset(transfer.Asset))
	source := b.makeAddressRef(transfer.From, transfer.Chain, baseDepth)
	target := b.makeAddressRef(transfer.To, transfer.Chain, baseDepth+1)
	if source.ID == "" || target.ID == "" || source.Key == target.Key {
		return nil, nil
	}
	seg := projectedSegment{
		Source:        source,
		Target:        target,
		ActionClass:   "transfers",
		ActionKey:     actionKey,
		ActionLabel:   actionLabel,
		ActionDomain:  "native_chain",
		Asset:         normalizeAsset(transfer.Asset),
		AssetKind:     meta.AssetKind,
		TokenStandard: meta.TokenStandard,
		TokenAddress:  meta.TokenAddress,
		TokenSymbol:   meta.TokenSymbol,
		TokenName:     meta.TokenName,
		TokenDecimals: meta.TokenDecimals,
		AmountRaw:     strings.TrimSpace(transfer.AmountRaw),
		USDSpot:       b.prices.usdFor(transfer.Asset, transfer.AmountRaw),
		TxID:          strings.ToUpper(strings.TrimSpace(transfer.TxID)),
		Height:        transfer.Height,
		Time:          transfer.Time,
		Confidence:    transfer.Confidence,
		ActorIDs:      mergeInt64s(source.ActorIDs, target.ActorIDs),
	}
	if !hasGraphableLiquidity(seg.AmountRaw) {
		return nil, nil
	}
	if b.minUSD > 0 && seg.USDSpot > 0 && seg.USDSpot < b.minUSD {
		return nil, nil
	}
	var next []frontierAddress
	for _, ref := range []flowRef{source, target} {
		if shouldExpandAddressRef(ref) {
			next = append(next, frontierAddress{Address: ref.Address, Chain: ref.Chain, Depth: ref.Depth})
		}
	}
	return []projectedSegment{seg}, uniqueFrontierAddresses(next)
}

func externalTransferKey(transfer externalTransfer) string {
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(transfer.Chain)),
		strings.ToUpper(strings.TrimSpace(transfer.TxID)),
		normalizeAddress(transfer.From),
		normalizeAddress(transfer.To),
		normalizeAsset(transfer.Asset),
		strings.TrimSpace(transfer.AmountRaw),
	}, "|")
}

func (a *App) fetchEVMTransfers(ctx context.Context, provider, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "etherscan":
		return a.fetchEtherscanTransfers(ctx, chain, address, start, end, maxPages)
	case "blockscout":
		return a.fetchBlockscoutTransfers(ctx, chain, address, start, end, maxPages)
	case "avacloud":
		return a.fetchAvaCloudTransfers(ctx, chain, address, start, end, maxPages)
	case "nodereal":
		return a.fetchNodeRealTransfers(ctx, chain, address, start, end, maxPages)
	default:
		return nil, false, errExternalTrackerUnavailable
	}
}

type etherscanLikeEnvelope struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type etherscanLikeTx struct {
	BlockNumber     string `json:"blockNumber"`
	TimeStamp       string `json:"timeStamp"`
	Hash            string `json:"hash"`
	From            string `json:"from"`
	To              string `json:"to"`
	Value           string `json:"value"`
	IsError         string `json:"isError"`
	TxReceiptStatus string `json:"txreceipt_status"`
	ContractAddress string `json:"contractAddress"`
	TokenName       string `json:"tokenName"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenDecimal    string `json:"tokenDecimal"`
}

type etherscanLikeConfig struct {
	BaseURL             string
	APIKey              string
	IncludeChain        bool
	SupportsBlockByTime bool
}

func (c etherscanLikeConfig) baseURLs() []string {
	urls := parseURLListValue(c.BaseURL)
	if len(urls) > 0 {
		return urls
	}
	if trimmed := strings.TrimSpace(c.BaseURL); trimmed != "" {
		return []string{trimmed}
	}
	return nil
}

func (a *App) fetchEtherscanTransfers(ctx context.Context, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	if len(a.cfg.etherscanAPIURLs()) == 0 || strings.TrimSpace(a.cfg.EtherscanAPIKey) == "" {
		return nil, false, errExternalTrackerUnavailable
	}
	return a.fetchEtherscanLikeTransfers(ctx, chain, address, start, end, maxPages, etherscanLikeConfig{
		BaseURL:             a.cfg.EtherscanAPIURL,
		APIKey:              a.cfg.EtherscanAPIKey,
		IncludeChain:        true,
		SupportsBlockByTime: true,
	})
}

func (a *App) fetchBlockscoutTransfers(ctx context.Context, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	if len(a.cfg.blockscoutAPIURLsForChain(chain)) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}
	return a.fetchEtherscanLikeTransfers(ctx, chain, address, start, end, maxPages, etherscanLikeConfig{
		BaseURL:      strings.Join(a.cfg.blockscoutAPIURLsForChain(chain), "|"),
		APIKey:       strings.TrimSpace(a.cfg.BlockscoutAPIKeys[strings.ToUpper(strings.TrimSpace(chain))]),
		IncludeChain: false,
	})
}

func (a *App) fetchEtherscanLikeTransfers(ctx context.Context, chain, address string, start, end time.Time, maxPages int, cfg etherscanLikeConfig) ([]externalTransfer, bool, error) {
	chainID := evmChainID(chain)
	if cfg.IncludeChain && chainID == "" {
		return nil, false, errExternalTrackerUnavailable
	}
	startBlock, endBlock, hasBlockRange, err := a.resolveEtherscanBlockRange(ctx, cfg, chain, chainID, start, end)
	if err != nil {
		return nil, false, err
	}
	var all []externalTransfer
	truncated := false
	for _, action := range []string{"txlist", "txlistinternal", "tokentx"} {
		transfers, actionTruncated, err := a.fetchEtherscanLikeActionTransfers(ctx, cfg, chain, chainID, address, start, end, maxPages, action, startBlock, endBlock, hasBlockRange)
		if err != nil {
			return nil, false, err
		}
		all = append(all, transfers...)
		truncated = truncated || actionTruncated
	}
	return dedupeExternalTransfers(all), truncated, nil
}

func (a *App) resolveEtherscanBlockRange(ctx context.Context, cfg etherscanLikeConfig, chain, chainID string, start, end time.Time) (int64, int64, bool, error) {
	if !cfg.SupportsBlockByTime {
		return 0, 0, false, nil
	}
	startBlock, ok, err := a.fetchEtherscanBlockNumberByTime(ctx, cfg, chain, chainID, start, "before")
	if err != nil {
		return 0, 0, false, err
	}
	if !ok {
		return 0, 0, false, nil
	}
	endBlock, ok, err := a.fetchEtherscanBlockNumberByTime(ctx, cfg, chain, chainID, end, "after")
	if err != nil {
		return 0, 0, false, err
	}
	if !ok {
		return 0, 0, false, nil
	}
	if startBlock < 0 {
		startBlock = 0
	}
	if endBlock < startBlock {
		endBlock = startBlock
	}
	return startBlock, endBlock, true, nil
}

func (a *App) fetchEtherscanBlockNumberByTime(ctx context.Context, cfg etherscanLikeConfig, chain, chainID string, ts time.Time, closest string) (int64, bool, error) {
	if !cfg.SupportsBlockByTime || ts.IsZero() {
		return 0, false, nil
	}
	timestamp := ts.UTC().Unix()
	if timestamp <= 0 {
		return 0, false, nil
	}
	provider := "etherscan"
	if meta, ok := trackerRequestMetaFromContext(ctx); ok && strings.TrimSpace(meta.Provider) != "" {
		provider = meta.Provider
	}
	if cached, ok := a.lookupTrackerBlockNumber(provider, chain, closest, timestamp); ok {
		return cached, true, nil
	}

	params := url.Values{}
	params.Set("module", "block")
	params.Set("action", "getblocknobytime")
	params.Set("timestamp", strconv.FormatInt(timestamp, 10))
	params.Set("closest", closest)
	if cfg.IncludeChain && chainID != "" {
		params.Set("chainid", chainID)
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		params.Set("apikey", cfg.APIKey)
	}

	var resp etherscanLikeEnvelope
	rawURLs := mapTrackerURLs(cfg.baseURLs(), func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "?" + params.Encode()
	})
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
		if ctx.Err() != nil {
			return 0, false, err
		}
		return 0, false, nil
	}
	var raw string
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		raw = strings.Trim(strings.TrimSpace(string(resp.Result)), `"`)
	}
	block := parseInt64(raw)
	if block <= 0 {
		return 0, false, nil
	}
	a.storeTrackerBlockNumber(provider, chain, closest, timestamp, block)
	return block, true, nil
}

func isEmptyEtherscanLikeResult(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return true
	}
	switch {
	case strings.Contains(raw, "no transactions"),
		strings.Contains(raw, "no internal transactions"),
		strings.Contains(raw, "no token transfers"),
		strings.Contains(raw, "no token balance"),
		strings.Contains(raw, "no tokens"),
		strings.Contains(raw, "no records found"),
		strings.Contains(raw, "no data found"):
		return true
	default:
		return false
	}
}

func isRetryableEtherscanLikeResult(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return false
	}
	return strings.Contains(raw, "rate limit") ||
		strings.Contains(raw, "max calls per sec") ||
		strings.Contains(raw, "query timeout") ||
		strings.Contains(raw, "timeout")
}

func isUnsupportedEtherscanLikeResult(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return false
	}
	return strings.Contains(raw, "requires paid plan") ||
		strings.Contains(raw, "paid plan") ||
		strings.Contains(raw, "paid tier") ||
		strings.Contains(raw, "not supported") ||
		strings.Contains(raw, "unsupported") ||
		strings.Contains(raw, "pro endpoint") ||
		strings.Contains(raw, "advanced api")
}

func (a *App) fetchEtherscanLikeActionTransfers(ctx context.Context, cfg etherscanLikeConfig, chain, chainID, address string, start, end time.Time, maxPages int, action string, startBlock, endBlock int64, hasBlockRange bool) ([]externalTransfer, bool, error) {
	var out []externalTransfer
	address = normalizeAddress(address)
	offset := externalTrackerPageSize
	if offset < 1 {
		offset = 50
	}
	if maxPages < 1 {
		maxPages = 1
	}
	truncated := false
	for page := 1; page <= maxPages; page++ {
		params := url.Values{}
		params.Set("module", "account")
		params.Set("action", action)
		params.Set("address", address)
		params.Set("page", strconv.Itoa(page))
		params.Set("offset", strconv.Itoa(offset))
		params.Set("sort", "desc")
		if cfg.IncludeChain && chainID != "" {
			params.Set("chainid", chainID)
		}
		if hasBlockRange {
			params.Set("startblock", strconv.FormatInt(startBlock, 10))
			params.Set("endblock", strconv.FormatInt(endBlock, 10))
		}
		if strings.TrimSpace(cfg.APIKey) != "" {
			params.Set("apikey", cfg.APIKey)
		}

		var rows []etherscanLikeTx
		stopPaging := false
		for attempt := 1; attempt <= 3; attempt++ {
			var resp etherscanLikeEnvelope
			rawURLs := mapTrackerURLs(cfg.baseURLs(), func(baseURL string) string {
				return strings.TrimRight(baseURL, "/") + "?" + params.Encode()
			})
			if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
				return nil, false, err
			}

			rows = nil
			if len(resp.Result) == 0 || string(resp.Result) == "\"\"" {
				stopPaging = true
				break
			}
			if err := json.Unmarshal(resp.Result, &rows); err == nil {
				break
			} else {
				var resultText string
				if err2 := json.Unmarshal(resp.Result, &resultText); err2 != nil {
					return nil, false, err
				}
				switch {
				case isEmptyEtherscanLikeResult(resultText):
					stopPaging = true
				case isRetryableEtherscanLikeResult(resultText):
					if attempt < 3 {
						if !sleepWithContext(ctx, time.Duration(attempt)*250*time.Millisecond) {
							return nil, false, ctx.Err()
						}
						continue
					}
					truncated = true
					stopPaging = true
				default:
					return nil, false, fmt.Errorf("etherscan %s returned non-array result: status=%s message=%s result=%q", action, strings.TrimSpace(resp.Status), strings.TrimSpace(resp.Message), strings.TrimSpace(resultText))
				}
				break
			}
		}
		if stopPaging {
			break
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			if !isSuccessfulEVMRow(row.IsError, row.TxReceiptStatus) {
				continue
			}
			ts := time.Unix(parseInt64(row.TimeStamp), 0).UTC()
			if !ts.IsZero() && ts.Before(start) {
				return out, truncated, nil
			}
			if !ts.IsZero() && ts.After(end) {
				continue
			}
			from := normalizeAddress(row.From)
			to := normalizeAddress(row.To)
			if from == "" || to == "" {
				continue
			}
			if from != address && to != address {
				continue
			}
			switch action {
			case "tokentx":
				transfer, ok := newEVMTokenTransfer(chain, from, to, row.Hash, parseInt64(row.BlockNumber), ts, row.Value, row.TokenDecimal, row.TokenSymbol, row.TokenName, row.ContractAddress, 0.97)
				if ok {
					out = append(out, transfer)
				}
			case "txlistinternal":
				transfer, ok := newEVMNativeTransfer(chain, from, to, row.Hash, parseInt64(row.BlockNumber), ts, row.Value, 0.97, "tracker.evm.internal_transfer", chain+" Internal Transfer")
				if ok {
					out = append(out, transfer)
				}
			default:
				transfer, ok := newEVMNativeTransfer(chain, from, to, row.Hash, parseInt64(row.BlockNumber), ts, row.Value, 0.97, "tracker.evm.native_transfer", chain+" Native Transfer")
				if ok {
					out = append(out, transfer)
				}
			}
		}

		if len(rows) < offset {
			break
		}
		if page == maxPages {
			truncated = true
		}
	}
	return out, truncated, nil
}

func (a *App) fetchAvaCloudTransfers(ctx context.Context, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	baseURLs := a.cfg.avaCloudBaseURLs()
	chainID := evmChainID(chain)
	if len(baseURLs) == 0 || chainID == "" {
		return nil, false, errExternalTrackerUnavailable
	}
	headers := map[string]string{}
	if strings.TrimSpace(a.cfg.AvaCloudAPIKey) != "" {
		headers["x-glacier-api-key"] = strings.TrimSpace(a.cfg.AvaCloudAPIKey)
	}
	var all []externalTransfer
	pageToken := ""
	truncated := false
	for page := 0; page < maxPages; page++ {
		params := url.Values{}
		params.Set("pageSize", strconv.Itoa(externalTrackerPageSize))
		params.Set("sortOrder", "desc")
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var resp map[string]any
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return fmt.Sprintf("%s/v1/chains/%s/addresses/%s/transactions?%s", strings.TrimRight(baseURL, "/"), chainID, url.PathEscape(address), params.Encode())
		})
		if err := a.getJSONAbsoluteMulti(ctx, rawURLs, headers, &resp); err != nil {
			return nil, false, err
		}
		rows := nestedSlice(resp, "transactions")
		if len(rows) == 0 {
			break
		}
		for _, rawRow := range rows {
			row, _ := rawRow.(map[string]any)
			if row == nil {
				continue
			}
			parentHash := firstNonEmpty(nestedString(row, "txHash"), nestedString(row, "nativeTransaction", "txHash"))
			parentHeight := firstNonZeroInt64(nestedInt64(row, "blockNumber"), nestedInt64(row, "nativeTransaction", "blockNumber"))
			parentTime := firstNonZeroTime(parseFlexibleTime(nestedValue(row, "blockTimestamp")), parseFlexibleTime(nestedValue(row, "nativeTransaction", "blockTimestamp")))
			if !parentTime.IsZero() && parentTime.Before(start) {
				return dedupeExternalTransfers(all), truncated, nil
			}
			if !parentTime.IsZero() && parentTime.After(end) {
				continue
			}
			if !isFlexibleSuccess(nestedValue(row, "txStatus")) && !isFlexibleSuccess(nestedValue(row, "nativeTransaction", "txStatus")) {
				continue
			}

			native := nestedMap(row, "nativeTransaction")
			if len(native) > 0 {
				from := extractFlexibleAddress(nestedValue(native, "from"))
				to := extractFlexibleAddress(nestedValue(native, "to"))
				if from == address || to == address {
					if transfer, ok := newEVMNativeTransfer(chain, from, to, firstNonEmpty(nestedString(native, "txHash"), parentHash), firstNonZeroInt64(nestedInt64(native, "blockNumber"), parentHeight), firstNonZeroTime(parseFlexibleTime(nestedValue(native, "blockTimestamp")), parentTime), stringifyAny(nestedValue(native, "value")), 0.97, "tracker.evm.native_transfer", chain+" Native Transfer"); ok {
						all = append(all, transfer)
					}
				}
			}

			for _, rawInternal := range nestedSlice(row, "internalTransactions") {
				internal, _ := rawInternal.(map[string]any)
				if internal == nil {
					continue
				}
				from := extractFlexibleAddress(nestedValue(internal, "from"))
				to := extractFlexibleAddress(nestedValue(internal, "to"))
				if from != address && to != address {
					continue
				}
				if transfer, ok := newEVMNativeTransfer(chain, from, to, parentHash, firstNonZeroInt64(nestedInt64(internal, "blockNumber"), parentHeight), firstNonZeroTime(parseFlexibleTime(nestedValue(internal, "blockTimestamp")), parentTime), stringifyAny(nestedValue(internal, "value")), 0.95, "tracker.evm.internal_transfer", chain+" Internal Transfer"); ok {
					all = append(all, transfer)
				}
			}

			for _, rawToken := range nestedSlice(row, "erc20Transfers") {
				tokenTx, _ := rawToken.(map[string]any)
				if tokenTx == nil {
					continue
				}
				from := extractFlexibleAddress(nestedValue(tokenTx, "from"))
				to := extractFlexibleAddress(nestedValue(tokenTx, "to"))
				if from != address && to != address {
					continue
				}
				tokenMeta := nestedMap(tokenTx, "erc20Token")
				if transfer, ok := newEVMTokenTransfer(
					chain,
					from,
					to,
					firstNonEmpty(nestedString(tokenTx, "txHash"), parentHash),
					firstNonZeroInt64(nestedInt64(tokenTx, "blockNumber"), parentHeight),
					firstNonZeroTime(parseFlexibleTime(nestedValue(tokenTx, "blockTimestamp")), parentTime),
					stringifyAny(nestedValue(tokenTx, "value")),
					stringifyAny(nestedValue(tokenMeta, "decimals")),
					firstNonEmpty(nestedString(tokenMeta, "symbol"), nestedString(tokenTx, "symbol")),
					firstNonEmpty(nestedString(tokenMeta, "name"), nestedString(tokenTx, "name")),
					extractFlexibleAddress(firstNonEmptyAny(nestedValue(tokenMeta, "address"), nestedValue(tokenMeta, "contractAddress"), nestedValue(tokenTx, "contractAddress"))),
					0.97,
				); ok {
					all = append(all, transfer)
				}
			}
		}
		pageToken = strings.TrimSpace(nestedString(resp, "nextPageToken"))
		if pageToken == "" {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, nil
}

func (a *App) fetchNodeRealTransfers(ctx context.Context, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	rawURLs := mapTrackerURLs(a.cfg.nodeRealBSCURLs(), func(baseURL string) string {
		baseURL = strings.TrimRight(baseURL, "/")
		if key := strings.TrimSpace(a.cfg.NodeRealAPIKey); key != "" && !strings.HasSuffix(baseURL, "/"+key) {
			return baseURL + "/" + key
		}
		return baseURL
	})
	if len(rawURLs) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}

	var all []externalTransfer
	tokenHashes := map[string]struct{}{}
	truncated := false
	for _, addressType := range []string{"from", "to"} {
		pageKey := ""
		for page := 0; page < maxPages; page++ {
			payload := map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "nr_getTransactionByAddress",
				"params": []any{map[string]any{
					"address":     address,
					"addressType": addressType,
					"category":    []string{"external", "internal", "20"},
					"order":       "desc",
					"maxCount":    fmt.Sprintf("0x%x", externalTrackerPageSize),
				}},
			}
			if pageKey != "" {
				payload["params"].([]any)[0].(map[string]any)["pageKey"] = pageKey
			}
			var resp map[string]any
			if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, payload, &resp); err != nil {
				return nil, false, err
			}
			result := nestedMap(resp, "result")
			rows := nestedSlice(result, "transfers")
			if len(rows) == 0 {
				rows = nestedSlice(result, "transactions")
			}
			if len(rows) == 0 {
				break
			}
			for _, rawRow := range rows {
				row, _ := rawRow.(map[string]any)
				if row == nil {
					continue
				}
				ts := parseFlexibleTime(firstNonEmptyAny(nestedValue(row, "blockTimeStamp"), nestedValue(row, "blockTimestamp"), nestedValue(row, "metadata", "blockTimestamp")))
				if !ts.IsZero() && ts.Before(start) {
					pageKey = ""
					break
				}
				if !ts.IsZero() && ts.After(end) {
					continue
				}
				if !isFlexibleSuccess(firstNonEmptyAny(nestedValue(row, "receiptsStatus"), nestedValue(row, "receiptStatus"), nestedValue(row, "txStatus"))) {
					continue
				}
				category := strings.TrimSpace(strings.ToLower(nestedString(row, "category")))
				txHash := cleanTxID(firstNonEmpty(nestedString(row, "hash"), nestedString(row, "txHash")))
				if txHash == "" {
					continue
				}
				if category == "20" {
					tokenHashes[txHash] = struct{}{}
					continue
				}
				from := normalizeAddress(firstNonEmpty(nestedString(row, "from"), nestedString(row, "fromAddress")))
				to := normalizeAddress(firstNonEmpty(nestedString(row, "to"), nestedString(row, "toAddress")))
				if from != address && to != address {
					continue
				}
				label := chain + " Native Transfer"
				key := "tracker.evm.native_transfer"
				if category == "internal" {
					label = chain + " Internal Transfer"
					key = "tracker.evm.internal_transfer"
				}
				if transfer, ok := newEVMNativeTransfer(chain, from, to, txHash, firstNonZeroInt64(nestedInt64(row, "blockNum"), nestedInt64(row, "blockNumber")), ts, stringifyAny(firstNonEmptyAny(nestedValue(row, "value"), nestedValue(row, "nativeValue"))), 0.94, key, label); ok {
					all = append(all, transfer)
				}
			}
			if pageKey == "" {
				pageKey = strings.TrimSpace(nestedString(result, "pageKey"))
			}
			if pageKey == "" {
				break
			}
			if page+1 >= maxPages {
				truncated = true
			}
		}
	}

	if len(tokenHashes) > 0 {
		transfers, err := a.fetchNodeRealTokenTransfersByHashBatch(ctx, rawURLs, chain, address, tokenHashes)
		if err != nil {
			return nil, false, err
		}
		for _, transfer := range transfers {
			if !transfer.Time.IsZero() && transfer.Time.Before(start) {
				continue
			}
			if !transfer.Time.IsZero() && transfer.Time.After(end) {
				continue
			}
			all = append(all, transfer)
		}
	}
	return dedupeExternalTransfers(all), truncated, nil
}

func (a *App) fetchNodeRealTokenTransfersByHashBatch(ctx context.Context, rawURLs []string, chain, watched string, hashes map[string]struct{}) ([]externalTransfer, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	hashList := make([]string, 0, len(hashes))
	for h := range hashes {
		hashList = append(hashList, h)
	}
	batch := make([]map[string]any, len(hashList))
	for i, h := range hashList {
		batch[i] = map[string]any{
			"jsonrpc": "2.0",
			"id":      i,
			"method":  "nr_getTransactionDetail",
			"params":  []any{h},
		}
	}
	var responses []struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, batch, &responses); err != nil {
		return nil, err
	}
	byID := make(map[int]json.RawMessage, len(responses))
	for _, r := range responses {
		if r.Error != nil {
			continue
		}
		byID[r.ID] = r.Result
	}
	var all []externalTransfer
	for i, txHash := range hashList {
		raw := byID[i]
		if raw == nil {
			continue
		}
		all = append(all, parseNodeRealTokenTransfers(chain, watched, txHash, raw)...)
	}
	return all, nil
}

func parseNodeRealTokenTransfers(chain, watched, txHash string, data json.RawMessage) []externalTransfer {
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	if len(result) == 0 || !isFlexibleSuccess(firstNonEmptyAny(nestedValue(result, "receiptsStatus"), nestedValue(result, "receiptStatus"), nestedValue(result, "txStatus"))) {
		return nil
	}
	ts := parseFlexibleTime(firstNonEmptyAny(nestedValue(result, "blockTimeStamp"), nestedValue(result, "blockTimestamp")))
	height := firstNonZeroInt64(nestedInt64(result, "blockNum"), nestedInt64(result, "blockNumber"))
	var out []externalTransfer
	for _, field := range []string{"tokenTransfers", "erc20Transfers"} {
		for _, rawItem := range nestedSlice(result, field) {
			item, _ := rawItem.(map[string]any)
			if item == nil {
				continue
			}
			from := normalizeAddress(firstNonEmpty(nestedString(item, "from"), nestedString(item, "fromAddress")))
			to := normalizeAddress(firstNonEmpty(nestedString(item, "to"), nestedString(item, "toAddress")))
			if from != watched && to != watched {
				continue
			}
			contractMap := nestedMap(item, "rawContract")
			contractAddr := extractFlexibleAddress(firstNonEmptyAny(nestedValue(contractMap, "address"), nestedValue(item, "contractAddress"), nestedValue(item, "tokenAddress")))
			if transfer, ok := newEVMTokenTransfer(
				chain,
				from,
				to,
				txHash,
				height,
				ts,
				stringifyAny(firstNonEmptyAny(nestedValue(item, "value"), nestedValue(item, "rawValue"))),
				stringifyAny(firstNonEmptyAny(nestedValue(contractMap, "decimal"), nestedValue(item, "decimal"), nestedValue(item, "tokenDecimal"))),
				firstNonEmpty(nestedString(item, "asset"), nestedString(item, "symbol"), nestedString(item, "tokenSymbol")),
				firstNonEmpty(nestedString(item, "tokenName"), nestedString(item, "name")),
				contractAddr,
				0.95,
			); ok {
				out = append(out, transfer)
			}
		}
	}
	return out
}

func isSuccessfulEVMRow(isError, txReceiptStatus string) bool {
	if strings.TrimSpace(isError) == "1" {
		return false
	}
	if status := strings.TrimSpace(txReceiptStatus); status != "" && status != "1" {
		return false
	}
	return true
}

func newEVMNativeTransfer(chain, from, to, txID string, height int64, ts time.Time, rawValue string, confidence float64, actionKey, actionLabel string) (externalTransfer, bool) {
	amount := normalizeGraphAmount(rawValue, 18)
	if !hasGraphableLiquidity(amount) {
		return externalTransfer{}, false
	}
	from = normalizeAddress(from)
	to = normalizeAddress(to)
	if from == "" || to == "" {
		return externalTransfer{}, false
	}
	return externalTransfer{
		Chain:       chain,
		Asset:       evmNativeAsset(chain),
		AssetKind:   "native",
		AmountRaw:   amount,
		From:        from,
		To:          to,
		TxID:        txID,
		Height:      height,
		Time:        ts,
		ActionKey:   actionKey,
		ActionLabel: actionLabel,
		Confidence:  confidence,
	}, true
}

func newEVMTokenTransfer(chain, from, to, txID string, height int64, ts time.Time, rawValue, decimalsRaw, symbol, name, contract string, confidence float64) (externalTransfer, bool) {
	decimals := max(0, int(parseInt64(strings.TrimSpace(decimalsRaw))))
	amount := normalizeGraphAmount(rawValue, decimals)
	if !hasGraphableLiquidity(amount) {
		return externalTransfer{}, false
	}
	from = normalizeAddress(from)
	to = normalizeAddress(to)
	contract = normalizeTokenAddress(chain, contract)
	asset := evmTokenAsset(chain, symbol, contract)
	if from == "" || to == "" || asset == "" {
		return externalTransfer{}, false
	}
	meta := fungibleTokenMetadata(chain, "erc20", contract, symbol, name, decimals)
	return externalTransfer{
		Chain:         chain,
		Asset:         asset,
		AssetKind:     meta.AssetKind,
		TokenStandard: meta.TokenStandard,
		TokenAddress:  meta.TokenAddress,
		TokenSymbol:   meta.TokenSymbol,
		TokenName:     meta.TokenName,
		TokenDecimals: meta.TokenDecimals,
		AmountRaw:     amount,
		From:          from,
		To:            to,
		TxID:          txID,
		Height:        height,
		Time:          ts,
		ActionKey:     "tracker.evm.token_transfer",
		ActionLabel:   chain + " Token Transfer",
		Confidence:    confidence,
	}, true
}

type esploraTx struct {
	TxID   string `json:"txid"`
	Status struct {
		Confirmed   bool  `json:"confirmed"`
		BlockHeight int64 `json:"block_height"`
		BlockTime   int64 `json:"block_time"`
	} `json:"status"`
	Vin  []esploraVin  `json:"vin"`
	Vout []esploraVout `json:"vout"`
}

type esploraVin struct {
	Prevout *esploraVout `json:"prevout"`
}

type esploraVout struct {
	ScriptPubKeyAddress string `json:"scriptpubkey_address"`
	Value               int64  `json:"value"`
}

func (a *App) fetchEsploraTransfers(ctx context.Context, chain, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	baseURLs := a.cfg.utxoTrackerURLsForChain(chain)
	if len(baseURLs) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}
	for _, baseURL := range baseURLs {
		if strings.Contains(strings.ToLower(baseURL), "explorer.doged.io") {
			return a.fetchDogedTransfers(ctx, address, start, end, maxPages)
		}
	}
	address = normalizeAddress(address)
	trackerAddress := address
	if chain == "BCH" && !strings.HasPrefix(strings.ToLower(trackerAddress), "bitcoincash:") {
		trackerAddress = "bitcoincash:" + trackerAddress
	}
	var out []externalTransfer
	var lastSeen string
	truncated := false
	supportsChainPaging := true
	for page := 0; page < maxPages; page++ {
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			baseURL = strings.TrimRight(baseURL, "/")
			rawURL := fmt.Sprintf("%s/address/%s/txs/chain", baseURL, url.PathEscape(trackerAddress))
			if supportsChainPaging && lastSeen != "" {
				rawURL += "/" + url.PathEscape(lastSeen)
			}
			return rawURL
		})
		var txs []esploraTx
		if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &txs); err != nil {
			if page == 0 && isHTTPStatusError(err, 404, 405) {
				supportsChainPaging = false
				rawURLs = mapTrackerURLs(baseURLs, func(baseURL string) string {
					baseURL = strings.TrimRight(baseURL, "/")
					return fmt.Sprintf("%s/address/%s/txs", baseURL, url.PathEscape(trackerAddress))
				})
				if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &txs); err != nil {
					return nil, false, err
				}
			} else {
				return nil, false, err
			}
		}
		if len(txs) == 0 {
			break
		}
		for _, tx := range txs {
			ts := time.Unix(tx.Status.BlockTime, 0).UTC()
			if tx.Status.BlockTime > 0 && ts.Before(start) {
				return dedupeExternalTransfers(out), truncated, nil
			}
			if tx.Status.BlockTime > 0 && ts.After(end) {
				continue
			}
			out = append(out, inferEsploraTransfers(chain, address, tx)...)
		}
		lastSeen = txs[len(txs)-1].TxID
		if !supportsChainPaging {
			if len(txs) >= 25 {
				truncated = true
			}
			break
		}
		if len(txs) < 25 {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(out), truncated, nil
}

func inferEsploraTransfers(chain, watched string, tx esploraTx) []externalTransfer {
	watched = normalizeAddress(watched)
	inputs := map[string]int64{}
	for _, vin := range tx.Vin {
		if vin.Prevout == nil {
			continue
		}
		addr := normalizeAddress(vin.Prevout.ScriptPubKeyAddress)
		if addr == "" {
			continue
		}
		inputs[addr] += vin.Prevout.Value
	}

	outputs := map[string]int64{}
	for _, vout := range tx.Vout {
		addr := normalizeAddress(vout.ScriptPubKeyAddress)
		if addr == "" {
			continue
		}
		outputs[addr] += vout.Value
	}

	return inferUTXOTransfers(chain, watched, tx.TxID, tx.Status.BlockHeight, time.Unix(tx.Status.BlockTime, 0).UTC(), inputs, outputs)
}

func inferUTXOTransfers(chain, watched, txID string, height int64, ts time.Time, inputs, outputs map[string]int64) []externalTransfer {
	watched = normalizeAddress(watched)
	inputSelf := inputs[watched]
	outputSelf := outputs[watched]
	asset := nativeAssetForChain(chain)
	var out []externalTransfer

	if inputSelf > 0 {
		recipients := map[string]int64{}
		for addr, value := range outputs {
			if addr == watched {
				continue
			}
			if _, wasInput := inputs[addr]; wasInput {
				continue
			}
			recipients[addr] += value
		}
		for addr, value := range recipients {
			out = append(out, externalTransfer{
				Chain:       chain,
				Asset:       asset,
				AmountRaw:   strconv.FormatInt(value, 10),
				From:        watched,
				To:          addr,
				TxID:        txID,
				Height:      height,
				Time:        ts,
				ActionKey:   "tracker.utxo.transfer",
				ActionLabel: chain + " Transfer",
				Confidence:  0.82,
			})
		}
		return out
	}

	if outputSelf <= 0 {
		return nil
	}

	senders := make([]string, 0, len(inputs))
	for addr := range inputs {
		if addr == watched {
			continue
		}
		senders = append(senders, addr)
	}
	sort.Strings(senders)
	if len(senders) == 1 {
		return []externalTransfer{{
			Chain:       chain,
			Asset:       asset,
			AmountRaw:   strconv.FormatInt(outputSelf, 10),
			From:        senders[0],
			To:          watched,
			TxID:        txID,
			Height:      height,
			Time:        ts,
			ActionKey:   "tracker.utxo.transfer",
			ActionLabel: chain + " Transfer",
			Confidence:  0.9,
		}}
	}

	nonSelfTotal := int64(0)
	for _, addr := range senders {
		nonSelfTotal += inputs[addr]
	}
	if nonSelfTotal <= 0 {
		return nil
	}
	remaining := outputSelf
	for i, addr := range senders {
		share := (outputSelf * inputs[addr]) / nonSelfTotal
		if i == len(senders)-1 {
			share = remaining
		} else {
			remaining -= share
		}
		if share <= 0 {
			continue
		}
		out = append(out, externalTransfer{
			Chain:       chain,
			Asset:       asset,
			AmountRaw:   strconv.FormatInt(share, 10),
			From:        addr,
			To:          watched,
			TxID:        txID,
			Height:      height,
			Time:        ts,
			ActionKey:   "tracker.utxo.transfer",
			ActionLabel: chain + " Transfer (Inferred)",
			Confidence:  0.64,
		})
	}
	return out
}

type cosmosTxsResponse struct {
	Txs         []cosmosTx         `json:"txs"`
	TxResponses []cosmosTxResponse `json:"tx_responses"`
	Pagination  struct {
		NextKey string `json:"next_key"`
		Total   string `json:"total"`
	} `json:"pagination"`
}

type cosmosTx struct {
	Body struct {
		Messages []json.RawMessage `json:"messages"`
	} `json:"body"`
}

type cosmosTxResponse struct {
	TxHash    string `json:"txhash"`
	Height    string `json:"height"`
	Timestamp string `json:"timestamp"`
	Code      int64  `json:"code"`
}

type cosmosMsgSend struct {
	Type        string `json:"@type"`
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      []struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	} `json:"amount"`
}

func (a *App) fetchGaiaTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	baseURLs := a.cfg.cosmosTrackerURLsForChain("GAIA")
	if len(baseURLs) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}
	address = normalizeAddress(address)
	var all []externalTransfer
	truncated := false
	for _, query := range []string{
		fmt.Sprintf("message.sender='%s'", address),
		fmt.Sprintf("transfer.recipient='%s'", address),
	} {
		transfers, actionTruncated, err := a.fetchGaiaQueryTransfers(ctx, baseURLs, address, query, start, end, maxPages)
		if err != nil {
			return nil, false, err
		}
		all = append(all, transfers...)
		truncated = truncated || actionTruncated
	}
	return dedupeExternalTransfers(all), truncated, nil
}

func (a *App) fetchGaiaQueryTransfers(ctx context.Context, baseURLs []string, watched, query string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	var out []externalTransfer
	if maxPages < 1 {
		maxPages = 1
	}
	truncated := false
	for page := 0; page < maxPages; page++ {
		params := url.Values{}
		params.Set("query", query)
		params.Set("pagination.limit", strconv.Itoa(externalTrackerPageSize))
		params.Set("pagination.offset", strconv.Itoa(page*externalTrackerPageSize))
		params.Set("order_by", "ORDER_BY_DESC")

		var resp cosmosTxsResponse
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return strings.TrimRight(baseURL, "/") + "/cosmos/tx/v1beta1/txs?" + params.Encode()
		})
		if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
			return nil, false, err
		}
		if len(resp.TxResponses) == 0 || len(resp.Txs) == 0 {
			break
		}
		limit := len(resp.Txs)
		if len(resp.TxResponses) < limit {
			limit = len(resp.TxResponses)
		}
		for i := 0; i < limit; i++ {
			txResp := resp.TxResponses[i]
			if txResp.Code != 0 {
				continue
			}
			ts, _ := time.Parse(time.RFC3339, strings.TrimSpace(txResp.Timestamp))
			if !ts.IsZero() && ts.Before(start) {
				return out, truncated, nil
			}
			if !ts.IsZero() && ts.After(end) {
				continue
			}
			height := parseInt64(txResp.Height)
			for _, rawMsg := range resp.Txs[i].Body.Messages {
				var msg cosmosMsgSend
				if err := json.Unmarshal(rawMsg, &msg); err != nil {
					continue
				}
				if msg.Type != "/cosmos.bank.v1beta1.MsgSend" {
					continue
				}
				from := normalizeAddress(msg.FromAddress)
				to := normalizeAddress(msg.ToAddress)
				if from != watched && to != watched {
					continue
				}
				for _, coin := range msg.Amount {
					asset, amount := cosmosCoinToAssetAmount("GAIA", coin.Denom, coin.Amount)
					if asset == "" || !hasGraphableLiquidity(amount) {
						continue
					}
					out = append(out, externalTransfer{
						Chain:       "GAIA",
						Asset:       asset,
						AmountRaw:   amount,
						From:        from,
						To:          to,
						TxID:        txResp.TxHash,
						Height:      height,
						Time:        ts,
						ActionKey:   "tracker.gaia.transfer",
						ActionLabel: "GAIA Transfer",
						Confidence:  0.97,
					})
				}
			}
		}
		if len(resp.TxResponses) < externalTrackerPageSize {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return out, truncated, nil
}

type dogedAddressTxResponse struct {
	Data []struct {
		TxHash      string `json:"txHash"`
		BlockHeight int64  `json:"blockHeight"`
		Timestamp   int64  `json:"timestamp"`
	} `json:"data"`
}

func (a *App) fetchDogedTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	baseURLs := a.cfg.utxoTrackerURLsForChain("DOGE")
	if len(baseURLs) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}
	address = strings.TrimSpace(address)
	var all []externalTransfer
	truncated := false
	for page := 0; page < maxPages; page++ {
		params := url.Values{}
		params.Set("length", strconv.Itoa(externalTrackerPageSize))
		params.Set("start", strconv.Itoa(page*externalTrackerPageSize))
		params.Set("order", "desc")
		var resp dogedAddressTxResponse
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return fmt.Sprintf("%s/api/address/%s/transactions?%s", strings.TrimRight(baseURL, "/"), url.PathEscape(address), params.Encode())
		})
		if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
			return nil, false, err
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, row := range resp.Data {
			ts := time.Unix(row.Timestamp, 0).UTC()
			if !ts.IsZero() && ts.Before(start) {
				return dedupeExternalTransfers(all), truncated, nil
			}
			if !ts.IsZero() && ts.After(end) {
				continue
			}
			transfers, err := a.fetchDogedTxTransfers(ctx, baseURLs, address, row.TxHash, row.BlockHeight, ts)
			if err != nil {
				return nil, false, err
			}
			all = append(all, transfers...)
		}
		if len(resp.Data) < externalTrackerPageSize {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, nil
}

func (a *App) fetchDogedTxTransfers(ctx context.Context, baseURLs []string, watched, txID string, height int64, ts time.Time) ([]externalTransfer, error) {
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return fmt.Sprintf("%s/tx/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(txID))
	})
	var (
		body    []byte
		lastErr error
	)
	for _, rawURL := range a.rotateTrackerURLs(rawURLs) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")
		resp, err := a.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("GET %s failed: status=%d body=%s", rawURL, resp.StatusCode, trimForLog(string(body), 200))
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, lastErr
	}
	text := html.UnescapeString(string(body))
	inputs := map[string]int64{}
	for _, match := range dogedInputRe.FindAllStringSubmatch(text, -1) {
		if len(match) < 4 {
			continue
		}
		addr := normalizeAddress(match[1])
		amount := humanPartsToRaw(match[2], match[3], 8)
		if addr == "" || amount <= 0 {
			continue
		}
		inputs[addr] += amount
	}
	outputs := map[string]int64{}
	for _, match := range dogedOutputRe.FindAllStringSubmatch(text, -1) {
		if len(match) < 4 {
			continue
		}
		addr := normalizeAddress(match[1])
		amount := humanPartsToRaw(match[2], match[3], 8)
		if addr == "" || amount <= 0 {
			continue
		}
		outputs[addr] += amount
	}
	return inferUTXOTransfers("DOGE", watched, txID, height, ts, inputs, outputs), nil
}

type solanaRPCEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  any             `json:"error"`
}

type solanaSignature struct {
	Signature string `json:"signature"`
	BlockTime int64  `json:"blockTime"`
	Slot      int64  `json:"slot"`
}

func (a *App) fetchSolanaTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, string, error) {
	address = strings.TrimSpace(address)
	baseURLs := a.cfg.solanaRPCURLs()
	if len(baseURLs) == 0 {
		return nil, false, "SOL tracker unavailable", nil
	}
	var all []externalTransfer
	before := ""
	truncated := false
	for page := 0; page < maxPages; page++ {
		params := []any{address, map[string]any{"limit": externalTrackerPageSize}}
		if before != "" {
			params[1].(map[string]any)["before"] = before
		}
		var sigResp []solanaSignature
		if err := a.postRPCJSON(ctx, baseURLs, "getSignaturesForAddress", params, &sigResp); err != nil {
			return nil, false, "", err
		}
		if len(sigResp) == 0 {
			break
		}
		// Filter signatures to the time window before batching.
		var inRange []solanaSignature
		earlyExit := false
		for _, sig := range sigResp {
			ts := time.Unix(sig.BlockTime, 0).UTC()
			if sig.BlockTime > 0 && ts.Before(start) {
				earlyExit = true
				break
			}
			if sig.BlockTime > 0 && ts.After(end) {
				continue
			}
			inRange = append(inRange, sig)
		}
		if len(inRange) > 0 {
			transfers, err := a.fetchSolanaTransactionTransfersBatch(ctx, address, inRange)
			if err != nil {
				return nil, false, "", err
			}
			all = append(all, transfers...)
		}
		if earlyExit {
			return dedupeExternalTransfers(all), truncated, "", nil
		}
		before = sigResp[len(sigResp)-1].Signature
		if len(sigResp) < externalTrackerPageSize {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, "", nil
}

func (a *App) fetchSolanaTransactionTransfersBatch(ctx context.Context, address string, sigs []solanaSignature) ([]externalTransfer, error) {
	if len(sigs) == 0 {
		return nil, nil
	}
	baseURLs := a.cfg.solanaRPCURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	// Build a JSON-RPC batch request for all signatures at once.
	batch := make([]map[string]any, len(sigs))
	for i, sig := range sigs {
		batch[i] = map[string]any{
			"jsonrpc": "2.0",
			"id":      i,
			"method":  "getTransaction",
			"params": []any{
				sig.Signature,
				map[string]any{
					"encoding":                       "jsonParsed",
					"maxSupportedTransactionVersion": 0,
				},
			},
		}
	}
	var responses []struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := a.postJSONAbsoluteMulti(ctx, baseURLs, nil, batch, &responses); err != nil {
		return nil, err
	}
	// Index responses by ID for ordered processing.
	byID := make(map[int]json.RawMessage, len(responses))
	for _, r := range responses {
		if r.Error != nil {
			continue
		}
		byID[r.ID] = r.Result
	}
	var all []externalTransfer
	for i, sig := range sigs {
		raw := byID[i]
		if raw == nil {
			continue
		}
		ts := time.Unix(sig.BlockTime, 0).UTC()
		transfers := parseSolanaTransactionTransfers(address, sig.Signature, ts, sig.Slot, raw)
		all = append(all, transfers...)
	}
	return all, nil
}

func parseSolanaTransactionTransfers(address, signature string, ts time.Time, slot int64, data json.RawMessage) []externalTransfer {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	meta, _ := raw["meta"].(map[string]any)
	if meta != nil && meta["err"] != nil {
		return nil
	}
	tx, _ := raw["transaction"].(map[string]any)
	if tx == nil {
		return nil
	}
	message, _ := tx["message"].(map[string]any)
	if message == nil {
		return nil
	}
	instructions, _ := message["instructions"].([]any)
	address = strings.TrimSpace(address)
	var out []externalTransfer
	for _, rawInst := range instructions {
		inst, _ := rawInst.(map[string]any)
		if !strings.EqualFold(stringifyAny(inst["program"]), "system") {
			continue
		}
		parsed, _ := inst["parsed"].(map[string]any)
		if parsed == nil || !strings.EqualFold(stringifyAny(parsed["type"]), "transfer") {
			continue
		}
		info, _ := parsed["info"].(map[string]any)
		if info == nil {
			continue
		}
		source := stringifyAny(info["source"])
		destination := stringifyAny(info["destination"])
		if source == "" || destination == "" {
			continue
		}
		if source != address && destination != address {
			continue
		}
		lamports := stringifyAny(info["lamports"])
		amount := normalizeGraphAmount(lamports, 9)
		if !hasGraphableLiquidity(amount) {
			continue
		}
		out = append(out, externalTransfer{
			Chain:       "SOL",
			Asset:       "SOL.SOL",
			AmountRaw:   amount,
			From:        source,
			To:          destination,
			TxID:        signature,
			Height:      slot,
			Time:        ts,
			ActionKey:   "tracker.sol.transfer",
			ActionLabel: "SOL Transfer",
			Confidence:  0.97,
		})
	}
	return out
}

type tronTxPage struct {
	Data []tronTx `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

type tronTx struct {
	TxID        string `json:"txID"`
	BlockNumber int64  `json:"blockNumber"`
	BlockTimeMS int64  `json:"block_timestamp"`
	RawData     struct {
		Contract []struct {
			Type      string `json:"type"`
			Parameter struct {
				Value struct {
					Amount       json.Number `json:"amount"`
					OwnerAddress string      `json:"owner_address"`
					ToAddress    string      `json:"to_address"`
				} `json:"value"`
			} `json:"parameter"`
		} `json:"contract"`
	} `json:"raw_data"`
}

func (a *App) fetchTronTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, string, error) {
	baseURLs := a.cfg.tronGridURLs()
	if len(baseURLs) == 0 {
		return nil, false, "TRON tracker unavailable", nil
	}
	headers := map[string]string{}
	if strings.TrimSpace(a.cfg.TronGridAPIKey) != "" {
		headers["TRON-PRO-API-KEY"] = a.cfg.TronGridAPIKey
	}
	address = strings.TrimSpace(address)
	var all []externalTransfer
	fingerprint := ""
	truncated := false
	for page := 0; page < maxPages; page++ {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(externalTrackerPageSize))
		params.Set("only_confirmed", "true")
		params.Set("order_by", "block_timestamp,desc")
		if fingerprint != "" {
			params.Set("fingerprint", fingerprint)
		}
		var resp tronTxPage
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return fmt.Sprintf("%s/v1/accounts/%s/transactions?%s", strings.TrimRight(baseURL, "/"), url.PathEscape(address), params.Encode())
		})
		if err := a.getJSONAbsoluteMulti(ctx, rawURLs, headers, &resp); err != nil {
			return nil, false, "", err
		}
		if len(resp.Data) == 0 {
			break
		}
		for _, tx := range resp.Data {
			ts := time.UnixMilli(tx.BlockTimeMS).UTC()
			if !ts.IsZero() && ts.Before(start) {
				return dedupeExternalTransfers(all), truncated, "", nil
			}
			if !ts.IsZero() && ts.After(end) {
				continue
			}
			for _, contract := range tx.RawData.Contract {
				if contract.Type != "TransferContract" {
					continue
				}
				source := tronBase58FromHex(contract.Parameter.Value.OwnerAddress)
				target := tronBase58FromHex(contract.Parameter.Value.ToAddress)
				if source == "" || target == "" {
					continue
				}
				if source != address && target != address {
					continue
				}
				amount := normalizeGraphAmount(contract.Parameter.Value.Amount.String(), 6)
				if !hasGraphableLiquidity(amount) {
					continue
				}
				all = append(all, externalTransfer{
					Chain:       "TRON",
					Asset:       "TRON.TRX",
					AmountRaw:   amount,
					From:        source,
					To:          target,
					TxID:        tx.TxID,
					Height:      tx.BlockNumber,
					Time:        ts,
					ActionKey:   "tracker.tron.transfer",
					ActionLabel: "TRON Transfer",
					Confidence:  0.97,
				})
			}
		}
		fingerprint = strings.TrimSpace(resp.Meta.Fingerprint)
		if fingerprint == "" {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, "", nil
}

type xrplAccountTxResult struct {
	Result struct {
		Marker       any `json:"marker"`
		Transactions []struct {
			Validated    bool   `json:"validated"`
			LedgerIdx    int64  `json:"ledger_index"`
			CloseTimeISO string `json:"close_time_iso"`
			Meta         struct {
				TransactionResult string `json:"TransactionResult"`
				DeliveredAmount   any    `json:"delivered_amount"`
			} `json:"meta"`
			Tx struct {
				Account         string `json:"Account"`
				Destination     string `json:"Destination"`
				Hash            string `json:"hash"`
				TransactionType string `json:"TransactionType"`
				Date            int64  `json:"date"`
				Amount          any    `json:"Amount"`
			} `json:"tx_json"`
		} `json:"transactions"`
	} `json:"result"`
}

func (a *App) fetchXRPLTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, string, error) {
	baseURLs := a.cfg.xrplRPCURLs()
	if len(baseURLs) == 0 {
		return nil, false, "XRP tracker unavailable", nil
	}
	address = strings.TrimSpace(address)
	var all []externalTransfer
	var marker any
	truncated := false
	for page := 0; page < maxPages; page++ {
		payload := map[string]any{
			"method": "account_tx",
			"params": []any{map[string]any{
				"account":          address,
				"ledger_index_min": -1,
				"ledger_index_max": -1,
				"binary":           false,
				"forward":          false,
				"limit":            externalTrackerPageSize,
				"api_version":      2,
			}},
		}
		if marker != nil {
			payload["params"].([]any)[0].(map[string]any)["marker"] = marker
		}
		var resp xrplAccountTxResult
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return strings.TrimRight(baseURL, "/") + "/"
		})
		if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, payload, &resp); err != nil {
			return nil, false, "", err
		}
		rows := resp.Result.Transactions
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if !row.Validated || row.Meta.TransactionResult != "tesSUCCESS" || row.Tx.TransactionType != "Payment" {
				continue
			}
			amountRaw := ""
			switch delivered := row.Meta.DeliveredAmount.(type) {
			case string:
				amountRaw = delivered
			}
			if amountRaw == "" {
				if direct, ok := row.Tx.Amount.(string); ok {
					amountRaw = direct
				}
			}
			amount := normalizeGraphAmount(amountRaw, 6)
			if !hasGraphableLiquidity(amount) {
				continue
			}
			ts := parseXRPLTime(row.Tx.Date)
			if !ts.IsZero() && ts.Before(start) {
				return dedupeExternalTransfers(all), truncated, "", nil
			}
			if !ts.IsZero() && ts.After(end) {
				continue
			}
			if row.Tx.Account != address && row.Tx.Destination != address {
				continue
			}
			all = append(all, externalTransfer{
				Chain:       "XRP",
				Asset:       "XRP.XRP",
				AmountRaw:   amount,
				From:        row.Tx.Account,
				To:          row.Tx.Destination,
				TxID:        row.Tx.Hash,
				Height:      row.LedgerIdx,
				Time:        ts,
				ActionKey:   "tracker.xrp.transfer",
				ActionLabel: "XRP Transfer",
				Confidence:  0.97,
			})
		}
		marker = resp.Result.Marker
		if marker == nil {
			break
		}
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, "", nil
}
