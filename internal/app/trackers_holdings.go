package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) fetchAddressLiveHoldings(ctx context.Context, chain, address string, prices priceBook) ([]liveHoldingValue, error) {
	address = normalizeAddress(address)
	if address == "" {
		return nil, nil
	}
	chain = normalizeChain(chain, address)
	if chain == "" {
		return nil, nil
	}
	if chain == "THOR" {
		return a.fetchProtocolAddressLiveHoldings(ctx, sourceProtocolTHOR, address, prices, nil)
	}
	if chain == "MAYA" {
		return a.fetchProtocolAddressLiveHoldings(ctx, sourceProtocolMAYA, address, prices, nil)
	}

	providers := a.cfg.trackerProvidersForChain(chain)
	if len(providers) == 0 {
		return nil, errExternalTrackerUnavailable
	}

	var lastErr error
	for _, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider == "" || provider == "none" || provider == "disabled" {
			continue
		}
		providerCtx := withTrackerRequestMeta(ctx, provider, chain)
		holdings, err := a.fetchAddressLiveHoldingsWithProvider(providerCtx, provider, chain, address, prices)
		if errors.Is(err, errExternalTrackerUnavailable) {
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		return compactLiveHoldings(holdings, prices), nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errExternalTrackerUnavailable
}

type thorBankBalanceResponse struct {
	Balances []struct {
		Denom  string `json:"denom"`
		Amount string `json:"amount"`
	} `json:"balances"`
}

type midgardMemberResponse struct {
	Pools []struct {
		Pool           string `json:"pool"`
		LiquidityUnits string `json:"liquidityUnits"`
	} `json:"pools"`
}

type thornodeNodeAccount struct {
	NodeAddress   string `json:"node_address"`
	Status        string `json:"status"`
	TotalBond     string `json:"total_bond"`
	Bond          string `json:"bond"`
	BondAddress   string `json:"bond_address"`
	BondProviders struct {
		Providers []struct {
			BondAddress string `json:"bond_address"`
			Bond        string `json:"bond"`
		} `json:"providers"`
	} `json:"bond_providers"`
}

func (a *App) fetchTHORAddressLiveHoldings(ctx context.Context, address string, prices priceBook, bondedByAddress map[string]string) ([]liveHoldingValue, error) {
	return a.fetchProtocolAddressLiveHoldings(ctx, sourceProtocolTHOR, address, prices, bondedByAddress)
}

func (a *App) fetchProtocolAddressLiveHoldings(ctx context.Context, protocol, address string, prices priceBook, bondedByAddress map[string]string) ([]liveHoldingValue, error) {
	var (
		holdings []liveHoldingValue
		errs     []error
	)
	type liveHoldingsFetchResult struct {
		holdings []liveHoldingValue
		err      error
	}
	bankResults := make(chan liveHoldingsFetchResult, 1)
	lpResults := make(chan liveHoldingsFetchResult, 1)

	go func() {
		holdings, err := a.fetchProtocolBankHoldings(ctx, protocol, address, prices)
		bankResults <- liveHoldingsFetchResult{holdings: holdings, err: err}
	}()
	go func() {
		holdings, err := a.fetchProtocolLPHoldings(ctx, protocol, address, prices)
		lpResults <- liveHoldingsFetchResult{holdings: holdings, err: err}
	}()

	bankResult := <-bankResults
	if bankResult.err != nil {
		errs = append(errs, bankResult.err)
	} else {
		holdings = append(holdings, bankResult.holdings...)
	}

	lpResult := <-lpResults
	if lpResult.err != nil {
		errs = append(errs, lpResult.err)
	} else {
		holdings = append(holdings, lpResult.holdings...)
	}

	bondHoldings, err := a.fetchProtocolBondHoldings(ctx, protocol, address, prices, bondedByAddress)
	if err != nil {
		errs = append(errs, err)
	} else {
		holdings = append(holdings, bondHoldings...)
	}

	compacted := compactLiveHoldings(holdings, prices)
	if len(compacted) > 0 {
		return compacted, nil
	}
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return nil, nil
}

func (a *App) fetchTHORBankHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	return a.fetchProtocolBankHoldings(ctx, sourceProtocolTHOR, address, prices)
}

func (a *App) fetchProtocolBankHoldings(ctx context.Context, protocol, address string, prices priceBook) ([]liveHoldingValue, error) {
	client := a.protocolNodeClient(protocol)
	if client == nil {
		return nil, errExternalTrackerUnavailable
	}
	var resp thorBankBalanceResponse
	path := "/cosmos/bank/v1beta1/balances/" + url.PathEscape(address)
	if err := client.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	holdings := make([]liveHoldingValue, 0, len(resp.Balances))
	for _, balance := range resp.Balances {
		asset := normalizeProtocolDenomAsset(protocol, balance.Denom)
		amountRaw := strings.TrimSpace(balance.Amount)
		if asset == "" || !hasGraphableLiquidity(amountRaw) {
			continue
		}
		holdings = append(holdings, liveHoldingValue{
			Asset:     asset,
			AmountRaw: amountRaw,
			USDSpot:   prices.usdFor(asset, amountRaw),
		})
	}
	return holdings, nil
}

func (a *App) fetchTHORLPHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	return a.fetchProtocolLPHoldings(ctx, sourceProtocolTHOR, address, prices)
}

func (a *App) fetchProtocolLPHoldings(ctx context.Context, protocol, address string, prices priceBook) ([]liveHoldingValue, error) {
	midgardClient := a.protocolMidgardClient(protocol)
	if a == nil || midgardClient == nil {
		return nil, errExternalTrackerUnavailable
	}
	var (
		resp       midgardMemberResponse
		lastStatus int
	)
	path := "/member/" + url.PathEscape(address)
	err := midgardClient.GetJSONObserved(ctx, path, &resp, func(meta RequestAttemptMeta) {
		if meta.StatusCode > 0 {
			lastStatus = meta.StatusCode
		}
	})
	if err != nil {
		if lastStatus == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if len(resp.Pools) == 0 || len(prices.PoolSnapshots) == 0 {
		return nil, nil
	}
	holdings := make([]liveHoldingValue, 0, len(resp.Pools)*2)
	for _, memberPool := range resp.Pools {
		poolAsset := normalizeAsset(memberPool.Pool)
		pool, ok := prices.PoolSnapshots[protocolPoolSnapshotKey(protocol, poolAsset)]
		if !ok {
			continue
		}
		runeShareRaw := mulDivAmounts(memberPool.LiquidityUnits, pool.RuneDepth, pool.LiquidityUnits)
		assetShareRaw := mulDivAmounts(memberPool.LiquidityUnits, pool.AssetDepth, pool.LiquidityUnits)
		if hasGraphableLiquidity(runeShareRaw) {
			holdings = append(holdings, liveHoldingValue{
				Asset:     nativeAssetForProtocol(protocol),
				AmountRaw: runeShareRaw,
				USDSpot:   prices.usdFor(nativeAssetForProtocol(protocol), runeShareRaw),
			})
		}
		if hasGraphableLiquidity(assetShareRaw) {
			holdings = append(holdings, liveHoldingValue{
				Asset:     poolAsset,
				AmountRaw: assetShareRaw,
				USDSpot:   prices.usdFor(poolAsset, assetShareRaw),
			})
		}
	}
	return holdings, nil
}

func (a *App) fetchTHORBondHoldings(ctx context.Context, address string, prices priceBook, bondedByAddress map[string]string) ([]liveHoldingValue, error) {
	return a.fetchProtocolBondHoldings(ctx, sourceProtocolTHOR, address, prices, bondedByAddress)
}

func (a *App) fetchProtocolBondHoldings(ctx context.Context, protocol, address string, prices priceBook, bondedByAddress map[string]string) ([]liveHoldingValue, error) {
	if bondedByAddress == nil {
		var err error
		bondedByAddress, err = a.fetchProtocolBondedNativeIndex(ctx, protocol)
		if err != nil {
			return nil, err
		}
	}
	amountRaw := strings.TrimSpace(bondedByAddress[normalizeAddress(address)])
	if !hasGraphableLiquidity(amountRaw) {
		return nil, nil
	}
	return []liveHoldingValue{{
		Asset:     nativeAssetForProtocol(protocol),
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(nativeAssetForProtocol(protocol), amountRaw),
	}}, nil
}

func (a *App) fetchTHORBondedRuneIndex(ctx context.Context) (map[string]string, error) {
	bondedByAddress, _, _, err := a.fetchProtocolBondIndexes(ctx, sourceProtocolTHOR)
	if err != nil {
		return nil, err
	}
	return bondedByAddress, nil
}

func (a *App) fetchProtocolBondedNativeIndex(ctx context.Context, protocol string) (map[string]string, error) {
	bondedByAddress, _, _, err := a.fetchProtocolBondIndexes(ctx, protocol)
	if err != nil {
		return nil, err
	}
	return bondedByAddress, nil
}

func (a *App) fetchTHORBondIndexes(ctx context.Context) (map[string]string, map[string]string, map[string]string, error) {
	return a.fetchProtocolBondIndexes(ctx, sourceProtocolTHOR)
}

// protocolBondIndexes is one /nodes snapshot indexed three ways.
type protocolBondIndexes struct {
	bondedByAddress map[string]string
	totalBondByNode map[string]string
	statusByNode    map[string]string
}

const bondIndexesCacheTTL = time.Minute

// fetchProtocolBondIndexes returns bonded amounts by bond address, total bond
// by node, and node status. The node list is large and changes slowly, so one
// snapshot is shared for bondIndexesCacheTTL across lookups and chunks.
func (a *App) fetchProtocolBondIndexes(ctx context.Context, protocol string) (map[string]string, map[string]string, map[string]string, error) {
	cache := &a.bondIndexesTHOR
	if normalizeSourceProtocol(protocol) == sourceProtocolMAYA {
		cache = &a.bondIndexesMAYA
	}
	cache.init(bondIndexesCacheTTL, nil)
	indexes, err := cache.get(ctx, func(ctx context.Context) (protocolBondIndexes, error) {
		bonded, totals, statuses, err := a.fetchProtocolBondIndexesFresh(ctx, protocol)
		return protocolBondIndexes{bondedByAddress: bonded, totalBondByNode: totals, statusByNode: statuses}, err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return indexes.bondedByAddress, indexes.totalBondByNode, indexes.statusByNode, nil
}

func (a *App) fetchProtocolBondIndexesFresh(ctx context.Context, protocol string) (map[string]string, map[string]string, map[string]string, error) {
	client := a.protocolNodeClient(protocol)
	if client == nil {
		return nil, nil, nil, errExternalTrackerUnavailable
	}
	var nodes []thornodeNodeAccount
	if err := client.GetJSON(ctx, protocolNodesPath(protocol), &nodes); err != nil {
		return nil, nil, nil, err
	}
	bondedByAddress := map[string]string{}
	totalBondByNode := map[string]string{}
	statusByNode := map[string]string{}
	for _, node := range nodes {
		nodeAddress := normalizeAddress(node.NodeAddress)
		totalBond := firstNonEmpty(strings.TrimSpace(node.TotalBond), strings.TrimSpace(node.Bond))
		nodeStatus := strings.TrimSpace(node.Status)
		if nodeAddress != "" && nodeStatus != "" {
			statusByNode[nodeAddress] = nodeStatus
		}
		if nodeAddress != "" && hasGraphableLiquidity(totalBond) {
			totalBondByNode[nodeAddress] = addRawAmounts(totalBondByNode[nodeAddress], totalBond)
		}
		for _, provider := range node.BondProviders.Providers {
			address := normalizeAddress(provider.BondAddress)
			bond := strings.TrimSpace(provider.Bond)
			if address == "" || !hasGraphableLiquidity(bond) {
				continue
			}
			bondedByAddress[address] = addRawAmounts(bondedByAddress[address], bond)
		}
		if address := normalizeAddress(node.BondAddress); address != "" && hasGraphableLiquidity(totalBond) {
			bondedByAddress[address] = addRawAmounts(bondedByAddress[address], totalBond)
		}
	}
	return bondedByAddress, totalBondByNode, statusByNode, nil
}

func mulDivAmounts(multiplier, multiplicand, divisor string) string {
	multiplier = strings.TrimSpace(multiplier)
	multiplicand = strings.TrimSpace(multiplicand)
	divisor = strings.TrimSpace(divisor)
	if multiplier == "" || multiplicand == "" || divisor == "" {
		return ""
	}
	left, ok := new(big.Int).SetString(multiplier, 10)
	if !ok {
		return ""
	}
	right, ok := new(big.Int).SetString(multiplicand, 10)
	if !ok {
		return ""
	}
	denom, ok := new(big.Int).SetString(divisor, 10)
	if !ok || denom.Sign() == 0 {
		return ""
	}
	product := new(big.Int).Mul(left, right)
	return new(big.Int).Div(product, denom).String()
}

func (a *App) fetchAddressLiveHoldingsWithProvider(ctx context.Context, provider, chain, address string, prices priceBook) ([]liveHoldingValue, error) {
	switch provider {
	case "utxo":
		return a.fetchUTXOAddressLiveHoldings(ctx, chain, address, prices)
	case "etherscan":
		if len(a.cfg.etherscanAPIURLs()) == 0 || strings.TrimSpace(a.cfg.EtherscanAPIKey) == "" {
			return nil, errExternalTrackerUnavailable
		}
		return a.fetchEtherscanLikeAddressLiveHoldings(ctx, chain, address, prices, etherscanLikeConfig{
			BaseURL:      a.cfg.EtherscanAPIURL,
			APIKey:       a.cfg.EtherscanAPIKey,
			IncludeChain: true,
		})
	case "blockscout":
		baseURLs := a.cfg.blockscoutAPIURLsForChain(chain)
		if len(baseURLs) == 0 {
			return nil, errExternalTrackerUnavailable
		}
		return a.fetchEtherscanLikeAddressLiveHoldings(ctx, chain, address, prices, etherscanLikeConfig{
			BaseURL:      a.cfg.BlockscoutAPIURLs[strings.ToUpper(strings.TrimSpace(chain))],
			APIKey:       strings.TrimSpace(a.cfg.BlockscoutAPIKeys[strings.ToUpper(strings.TrimSpace(chain))]),
			IncludeChain: false,
		})
	case "nodereal":
		return a.fetchNodeRealAddressLiveHoldings(ctx, chain, address, prices)
	case "avacloud":
		return nil, errExternalTrackerUnavailable
	case "solana":
		return a.fetchSolanaAddressLiveHoldings(ctx, address, prices)
	case "trongrid":
		return a.fetchTronAddressLiveHoldings(ctx, address, prices)
	case "xrpl":
		return a.fetchXRPLAddressLiveHoldings(ctx, address, prices)
	case "cosmos":
		return a.fetchGaiaAddressLiveHoldings(ctx, address, prices)
	case "radix":
		return a.fetchRadixAddressLiveHoldings(ctx, address, prices)
	default:
		return nil, errExternalTrackerUnavailable
	}
}

func compactLiveHoldings(holdings []liveHoldingValue, prices priceBook) []liveHoldingValue {
	if len(holdings) == 0 {
		return nil
	}
	type aggregate struct {
		AmountRaw int64
		USDSpot   float64
	}
	byAsset := map[string]aggregate{}
	for _, holding := range holdings {
		asset := normalizeAsset(holding.Asset)
		amountRaw := strings.TrimSpace(holding.AmountRaw)
		if asset == "" || !hasGraphableLiquidity(amountRaw) {
			continue
		}
		current := byAsset[asset]
		current.AmountRaw += parseInt64(amountRaw)
		current.USDSpot += holding.USDSpot
		byAsset[asset] = current
	}
	if len(byAsset) == 0 {
		return nil
	}
	out := make([]liveHoldingValue, 0, len(byAsset))
	for asset, aggregate := range byAsset {
		amountRaw := strconv.FormatInt(aggregate.AmountRaw, 10)
		usdSpot := prices.usdFor(asset, amountRaw)
		if usdSpot <= 0 && aggregate.USDSpot > 0 {
			usdSpot = aggregate.USDSpot
		}
		out = append(out, liveHoldingValue{
			Asset:     asset,
			AmountRaw: amountRaw,
			USDSpot:   usdSpot,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].USDSpot == out[j].USDSpot {
			return out[i].Asset < out[j].Asset
		}
		return out[i].USDSpot > out[j].USDSpot
	})
	return out
}

func (a *App) fetchUTXOAddressLiveHoldings(ctx context.Context, chain, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.utxoTrackerURLsForChain(chain)
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	chain = strings.ToUpper(strings.TrimSpace(chain))
	address = normalizeAddress(address)
	trackerAddress := address
	if chain == "BCH" && !strings.HasPrefix(strings.ToLower(trackerAddress), "bitcoincash:") {
		trackerAddress = "bitcoincash:" + trackerAddress
	}

	var resp map[string]any
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		baseURL = strings.TrimRight(baseURL, "/")
		if chain == "DOGE" {
			return fmt.Sprintf("%s/api/address/%s", baseURL, url.PathEscape(address))
		}
		return fmt.Sprintf("%s/address/%s", baseURL, url.PathEscape(trackerAddress))
	})
	var balanceRaw int64
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
		dogeFallbackURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			baseURL = strings.TrimRight(baseURL, "/")
			if chain != "DOGE" {
				return ""
			}
			return fmt.Sprintf("%s/address/%s", baseURL, url.PathEscape(address))
		})
		if len(dogeFallbackURLs) > 0 {
			// doged's JSON address endpoint currently 404s, but the HTML page
			// still embeds the confirmed UTXO balance in a balances blob.
			rawBalance, err2 := a.fetchDogedHTMLAddressBalance(ctx, dogeFallbackURLs)
			if err2 != nil {
				return nil, fmt.Errorf("%v; doged html fallback failed: %w", err, err2)
			}
			balanceRaw = rawBalance
		} else {
			return nil, err
		}
	} else {
		chainFunded := nestedInt64(resp, "chain_stats", "funded_txo_sum")
		chainSpent := nestedInt64(resp, "chain_stats", "spent_txo_sum")
		mempoolFunded := nestedInt64(resp, "mempool_stats", "funded_txo_sum")
		mempoolSpent := nestedInt64(resp, "mempool_stats", "spent_txo_sum")
		balanceRaw = (chainFunded + mempoolFunded) - (chainSpent + mempoolSpent)

		if balanceRaw == 0 {
			balanceRaw = firstNonZeroInt64(
				nestedInt64(resp, "balanceSat"),
				nestedInt64(resp, "balance_sat"),
				nestedInt64(resp, "confirmedBalanceSat"),
				nestedInt64(resp, "confirmed_balance_sat"),
			)
		}
		if balanceRaw == 0 {
			if balanceDecimal := firstNonEmpty(
				nestedString(resp, "balance"),
				nestedString(resp, "confirmedBalance"),
				nestedString(resp, "confirmed_balance"),
			); balanceDecimal != "" {
				parts := strings.SplitN(strings.TrimSpace(balanceDecimal), ".", 2)
				whole := ""
				fractional := ""
				if len(parts) > 0 {
					whole = parts[0]
				}
				if len(parts) > 1 {
					fractional = parts[1]
				}
				balanceRaw = humanPartsToRaw(whole, fractional, 8)
			}
		}
	}
	if balanceRaw < 0 {
		balanceRaw = 0
	}

	asset := nativeAssetForChain(chain)
	amountRaw := strconv.FormatInt(balanceRaw, 10)
	return []liveHoldingValue{{
		Asset:     asset,
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(asset, amountRaw),
	}}, nil
}

func (a *App) fetchDogedHTMLAddressBalance(ctx context.Context, rawURLs []string) (int64, error) {
	body, err := a.getTextAbsoluteMulti(ctx, rawURLs, nil)
	if err != nil {
		return 0, err
	}
	return parseDogedHTMLAddressBalance(body)
}

func parseDogedHTMLAddressBalance(body string) (int64, error) {
	text := html.UnescapeString(body)
	match := dogedBalancesRe.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0, errors.New("doged balances blob not found")
	}

	payload := strings.NewReplacer(
		`\\`, `\`,
		`\/`, `/`,
	).Replace(match[1])

	var balances struct {
		Main struct {
			SatsAmount int64  `json:"satsAmount"`
			BalanceSat int64  `json:"balanceSat"`
			Balance    string `json:"balance"`
			UTXOs      []struct {
				SatsAmount int64 `json:"satsAmount"`
			} `json:"utxos"`
		} `json:"main"`
	}
	if err := json.Unmarshal([]byte(payload), &balances); err != nil {
		return 0, err
	}

	var balanceRaw int64
	for _, utxo := range balances.Main.UTXOs {
		if utxo.SatsAmount > 0 {
			balanceRaw += utxo.SatsAmount
		}
	}
	if balanceRaw == 0 {
		balanceRaw = firstNonZeroInt64(balances.Main.SatsAmount, balances.Main.BalanceSat)
	}
	if balanceRaw == 0 && strings.TrimSpace(balances.Main.Balance) != "" {
		parts := strings.SplitN(strings.TrimSpace(balances.Main.Balance), ".", 2)
		whole := ""
		fractional := ""
		if len(parts) > 0 {
			whole = parts[0]
		}
		if len(parts) > 1 {
			fractional = parts[1]
		}
		balanceRaw = humanPartsToRaw(whole, fractional, 8)
	}
	return balanceRaw, nil
}

func (a *App) fetchEtherscanLikeAddressLiveHoldings(ctx context.Context, chain, address string, prices priceBook, cfg etherscanLikeConfig) ([]liveHoldingValue, error) {
	baseURLs := cfg.baseURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	chainID := evmChainID(chain)
	if cfg.IncludeChain && chainID == "" {
		return nil, errExternalTrackerUnavailable
	}

	params := url.Values{}
	params.Set("module", "account")
	params.Set("action", "balance")
	params.Set("address", address)
	params.Set("tag", "latest")
	if cfg.IncludeChain && chainID != "" {
		params.Set("chainid", chainID)
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		params.Set("apikey", cfg.APIKey)
	}

	var resp etherscanLikeEnvelope
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "?" + params.Encode()
	})
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
		return nil, err
	}

	var weiValue string
	if err := json.Unmarshal(resp.Result, &weiValue); err != nil {
		weiValue = strings.Trim(strings.TrimSpace(string(resp.Result)), `"`)
	}
	weiValue = strings.TrimSpace(weiValue)
	if weiValue == "" {
		weiValue = "0"
	}
	amountRaw := normalizeGraphAmount(weiValue, 18)
	if amountRaw == "" {
		amountRaw = "0"
	}
	asset := evmNativeAsset(chain)
	holdings := []liveHoldingValue{{
		Asset:     asset,
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(asset, amountRaw),
	}}
	tokenHoldings, shouldFallbackTokenLookups, err := a.fetchEtherscanLikeAddressTokenHoldings(ctx, chain, address, prices, cfg)
	if shouldFallbackTokenLookups {
		etherscanFallback, fallbackErr := a.fetchEtherscanLikeAddressTokenHoldingsViaTokenBalance(ctx, chain, address, prices, cfg)
		if fallbackErr == nil && len(etherscanFallback) > 0 {
			tokenHoldings = etherscanFallback
		}
	}
	if len(tokenHoldings) == 0 && strings.EqualFold(strings.TrimSpace(chain), "ETH") && (shouldFallbackTokenLookups || err != nil) {
		ethplorerHoldings, fallbackErr := a.fetchEthplorerAddressTokenHoldings(ctx, address, prices)
		if fallbackErr == nil && len(ethplorerHoldings) > 0 {
			tokenHoldings = ethplorerHoldings
		}
	}
	if err == nil && len(tokenHoldings) > 0 {
		holdings = append(holdings, tokenHoldings...)
	}
	return holdings, nil
}

type etherscanTokenContractMeta struct {
	Contract string
	Symbol   string
	Name     string
	Decimals int
}

func (a *App) fetchEtherscanLikeAddressTokenHoldings(ctx context.Context, chain, address string, prices priceBook, cfg etherscanLikeConfig) ([]liveHoldingValue, bool, error) {
	chainID := evmChainID(chain)
	if cfg.IncludeChain && chainID == "" {
		return nil, false, errExternalTrackerUnavailable
	}
	if a.isTrackerFeatureUnsupportedFromContext(ctx, etherscanAddressTokenBalanceFeature) {
		return nil, true, nil
	}
	params := url.Values{}
	params.Set("module", "account")
	params.Set("action", "addresstokenbalance")
	params.Set("address", address)
	params.Set("page", "1")
	params.Set("offset", "100")
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
		// Treat unsupported token-balance endpoints as non-fatal.
		if isHTTPStatusError(err, 400, 404, 405) {
			a.markTrackerFeatureUnsupportedFromContext(ctx, etherscanAddressTokenBalanceFeature)
			return nil, true, nil
		}
		return nil, false, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(resp.Result, &rows); err != nil {
		var resultText string
		if err2 := json.Unmarshal(resp.Result, &resultText); err2 == nil {
			switch {
			case isEmptyEtherscanLikeResult(resultText):
				a.markTrackerFeatureSupportedFromContext(ctx, etherscanAddressTokenBalanceFeature)
				return nil, false, nil
			case isUnsupportedEtherscanLikeResult(resultText):
				a.markTrackerFeatureUnsupportedFromContext(ctx, etherscanAddressTokenBalanceFeature)
				return nil, true, nil
			}
		}
		return nil, false, nil
	}
	a.markTrackerFeatureSupportedFromContext(ctx, etherscanAddressTokenBalanceFeature)
	if len(rows) == 0 {
		return nil, false, nil
	}
	holdings := make([]liveHoldingValue, 0, len(rows))
	for _, row := range rows {
		contract := firstNonEmpty(
			stringifyAny(row["TokenAddress"]),
			stringifyAny(row["tokenAddress"]),
			stringifyAny(row["contractAddress"]),
		)
		symbol := firstNonEmpty(
			stringifyAny(row["TokenSymbol"]),
			stringifyAny(row["tokenSymbol"]),
		)
		decimalsRaw := firstNonEmpty(
			stringifyAny(row["TokenDivisor"]),
			stringifyAny(row["tokenDecimal"]),
			stringifyAny(row["decimals"]),
		)
		quantityRaw := firstNonEmpty(
			stringifyAny(row["TokenQuantity"]),
			stringifyAny(row["tokenQuantity"]),
			stringifyAny(row["balance"]),
		)
		tokenPriceRaw := firstNonEmpty(
			stringifyAny(row["TokenPriceUSD"]),
			stringifyAny(row["tokenPriceUSD"]),
			stringifyAny(row["tokenPriceUsd"]),
		)
		decimals := max(0, int(parseInt64(decimalsRaw)))
		amountRaw := normalizeGraphAmount(quantityRaw, decimals)
		asset := evmTokenAsset(chain, symbol, contract)
		if asset == "" || !hasGraphableLiquidity(amountRaw) {
			continue
		}
		usdSpot := prices.usdFor(asset, amountRaw)
		if usdSpot <= 0 {
			tokenPrice := parseFlexibleFloat64(tokenPriceRaw)
			if tokenPrice > 0 {
				usdSpot = (float64(parseInt64(amountRaw)) / 1e8) * tokenPrice
			}
		}
		holdings = append(holdings, liveHoldingValue{
			Asset:     asset,
			AmountRaw: amountRaw,
			USDSpot:   usdSpot,
		})
	}
	return holdings, false, nil
}

func (a *App) fetchEtherscanLikeAddressTokenHoldingsViaTokenBalance(ctx context.Context, chain, address string, prices priceBook, cfg etherscanLikeConfig) ([]liveHoldingValue, error) {
	contracts, err := a.fetchEtherscanLikeRecentTokenContracts(ctx, chain, address, cfg)
	if err != nil || len(contracts) == 0 {
		return nil, err
	}
	holdings := make([]liveHoldingValue, 0, len(contracts))
	for _, contract := range contracts {
		amountRaw, err := a.fetchEtherscanLikeTokenBalance(ctx, chain, address, contract, cfg)
		if err != nil || !hasGraphableLiquidity(amountRaw) {
			continue
		}
		asset := evmTokenAsset(chain, contract.Symbol, contract.Contract)
		if asset == "" {
			continue
		}
		holdings = append(holdings, liveHoldingValue{
			Asset:     asset,
			AmountRaw: amountRaw,
			USDSpot:   prices.usdFor(asset, amountRaw),
		})
	}
	return holdings, nil
}

func (a *App) fetchEtherscanLikeRecentTokenContracts(ctx context.Context, chain, address string, cfg etherscanLikeConfig) ([]etherscanTokenContractMeta, error) {
	chainID := evmChainID(chain)
	if cfg.IncludeChain && chainID == "" {
		return nil, errExternalTrackerUnavailable
	}
	params := url.Values{}
	params.Set("module", "account")
	params.Set("action", "tokentx")
	params.Set("address", address)
	params.Set("page", "1")
	params.Set("offset", "100")
	params.Set("sort", "desc")
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
		return nil, err
	}
	var rows []etherscanLikeTx
	if err := json.Unmarshal(resp.Result, &rows); err != nil {
		return nil, nil
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]etherscanTokenContractMeta, 0, 64)
	seen := map[string]struct{}{}
	for _, row := range rows {
		contract := normalizeTokenAddress(chain, row.ContractAddress)
		if contract == "" {
			continue
		}
		if _, ok := seen[contract]; ok {
			continue
		}
		seen[contract] = struct{}{}
		out = append(out, etherscanTokenContractMeta{
			Contract: contract,
			Symbol:   strings.TrimSpace(row.TokenSymbol),
			Name:     strings.TrimSpace(row.TokenName),
			Decimals: max(0, int(parseInt64(strings.TrimSpace(row.TokenDecimal)))),
		})
		if len(out) >= 64 {
			break
		}
	}
	return out, nil
}

func (a *App) fetchEtherscanLikeTokenBalance(ctx context.Context, chain, address string, contract etherscanTokenContractMeta, cfg etherscanLikeConfig) (string, error) {
	chainID := evmChainID(chain)
	if cfg.IncludeChain && chainID == "" {
		return "", errExternalTrackerUnavailable
	}
	params := url.Values{}
	params.Set("module", "account")
	params.Set("action", "tokenbalance")
	params.Set("contractaddress", contract.Contract)
	params.Set("address", address)
	params.Set("tag", "latest")
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
		return "", err
	}
	var raw string
	if err := json.Unmarshal(resp.Result, &raw); err != nil {
		raw = strings.Trim(strings.TrimSpace(string(resp.Result)), `"`)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	return normalizeGraphAmount(raw, contract.Decimals), nil
}

func (a *App) fetchEthplorerAddressTokenHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.ethplorerAPIURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	apiKey := strings.TrimSpace(a.cfg.EthplorerAPIKey)
	if apiKey == "" {
		apiKey = "freekey"
	}
	var resp map[string]any
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "/getAddressInfo/" + url.PathEscape(address) + "?apiKey=" + url.QueryEscape(apiKey)
	})
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
		return nil, err
	}
	tokens := nestedSlice(resp, "tokens")
	if len(tokens) == 0 {
		return nil, nil
	}
	holdings := make([]liveHoldingValue, 0, len(tokens))
	for _, tokenValue := range tokens {
		tokenMap, ok := tokenValue.(map[string]any)
		if !ok {
			continue
		}
		tokenInfo := nestedMap(tokenMap, "tokenInfo")
		contract := nestedString(tokenInfo, "address")
		symbol := nestedString(tokenInfo, "symbol")
		decimals := max(0, int(parseFlexibleInt64(tokenInfo["decimals"])))
		rawBalance := firstNonEmpty(
			nestedString(tokenMap, "rawBalance"),
			stringifyAny(tokenMap["balance"]),
		)
		amountRaw := normalizeGraphAmount(rawBalance, decimals)
		asset := evmTokenAsset("ETH", symbol, contract)
		if asset == "" || !hasGraphableLiquidity(amountRaw) {
			continue
		}
		usdSpot := prices.usdFor(asset, amountRaw)
		if usdSpot <= 0 {
			tokenPrice := parseFlexibleFloat64(nestedValue(tokenInfo, "price", "rate"))
			if tokenPrice > 0 {
				usdSpot = (float64(parseInt64(amountRaw)) / 1e8) * tokenPrice
			}
		}
		holdings = append(holdings, liveHoldingValue{
			Asset:     asset,
			AmountRaw: amountRaw,
			USDSpot:   usdSpot,
		})
	}
	return holdings, nil
}

func (a *App) fetchNodeRealAddressLiveHoldings(ctx context.Context, chain, address string, prices priceBook) ([]liveHoldingValue, error) {
	rawURLs := mapTrackerURLs(a.cfg.nodeRealBSCURLs(), func(baseURL string) string {
		baseURL = strings.TrimRight(baseURL, "/")
		if key := strings.TrimSpace(a.cfg.NodeRealAPIKey); key != "" && !strings.HasSuffix(baseURL, "/"+key) {
			return baseURL + "/" + key
		}
		return baseURL
	})
	if len(rawURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}

	var resp struct {
		Result string `json:"result"`
	}
	if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_getBalance",
		"params":  []any{address, "latest"},
	}, &resp); err != nil {
		return nil, err
	}
	wei := strings.TrimSpace(resp.Result)
	wei = strings.TrimPrefix(strings.ToLower(wei), "0x")
	if wei == "" {
		wei = "0"
	}
	weiValue, ok := new(big.Int).SetString(wei, 16)
	if !ok {
		return nil, fmt.Errorf("invalid nodereal balance result: %q", resp.Result)
	}
	amountRaw := normalizeGraphAmount(weiValue.String(), 18)
	asset := evmNativeAsset(chain)
	return []liveHoldingValue{{
		Asset:     asset,
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(asset, amountRaw),
	}}, nil
}

type cosmosBalanceResponse struct {
	Balances []cosmosBalance `json:"balances"`
}

type cosmosBalance struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

func (a *App) fetchGaiaAddressLiveHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.cosmosTrackerURLsForChain("GAIA")
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	var resp cosmosBalanceResponse
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "/cosmos/bank/v1beta1/balances/" + url.PathEscape(address)
	})
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
		return nil, err
	}
	holdings := make([]liveHoldingValue, 0, len(resp.Balances))
	for _, balance := range resp.Balances {
		asset, amountRaw := cosmosCoinToAssetAmount("GAIA", balance.Denom, balance.Amount)
		if asset == "" {
			continue
		}
		holdings = append(holdings, liveHoldingValue{
			Asset:     asset,
			AmountRaw: amountRaw,
			USDSpot:   prices.usdFor(asset, amountRaw),
		})
	}
	return holdings, nil
}

func (a *App) fetchSolanaAddressLiveHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.solanaRPCURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	var resp struct {
		Value int64 `json:"value"`
	}
	if err := a.postRPCJSON(ctx, baseURLs, "getBalance", []any{
		address,
		map[string]any{"commitment": "finalized"},
	}, &resp); err != nil {
		return nil, err
	}
	amountRaw := normalizeGraphAmount(strconv.FormatInt(resp.Value, 10), 9)
	asset := nativeAssetForChain("SOL")
	return []liveHoldingValue{{
		Asset:     asset,
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(asset, amountRaw),
	}}, nil
}

func (a *App) fetchTronAddressLiveHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.tronGridURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	headers := map[string]string{}
	if strings.TrimSpace(a.cfg.TronGridAPIKey) != "" {
		headers["TRON-PRO-API-KEY"] = a.cfg.TronGridAPIKey
	}
	var resp struct {
		Data []struct {
			Balance json.Number `json:"balance"`
		} `json:"data"`
	}
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return fmt.Sprintf("%s/v1/accounts/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(address))
	})
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, headers, &resp); err != nil {
		return nil, err
	}
	balanceRaw := int64(0)
	if len(resp.Data) > 0 {
		balanceRaw = parseFlexibleInt64(resp.Data[0].Balance)
	}
	amountRaw := normalizeGraphAmount(strconv.FormatInt(balanceRaw, 10), 6)
	asset := nativeAssetForChain("TRON")
	return []liveHoldingValue{{
		Asset:     asset,
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(asset, amountRaw),
	}}, nil
}

func (a *App) fetchXRPLAddressLiveHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.xrplRPCURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	var resp struct {
		Result struct {
			AccountData struct {
				Balance string `json:"Balance"`
			} `json:"account_data"`
		} `json:"result"`
	}
	payload := map[string]any{
		"method": "account_info",
		"params": []any{map[string]any{
			"account":      address,
			"ledger_index": "validated",
			"strict":       true,
		}},
	}
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "/"
	})
	if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, payload, &resp); err != nil {
		return nil, err
	}
	amountRaw := normalizeGraphAmount(strings.TrimSpace(resp.Result.AccountData.Balance), 6)
	asset := nativeAssetForChain("XRP")
	return []liveHoldingValue{{
		Asset:     asset,
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor(asset, amountRaw),
	}}, nil
}

type radixFungiblesPageResponse struct {
	Address    string `json:"address"`
	NextCursor string `json:"next_cursor"`
	Items      []struct {
		ResourceAddress string `json:"resource_address"`
		Amount          string `json:"amount"`
	} `json:"items"`
}

type radixStreamTransactionsResponse struct {
	NextCursor string `json:"next_cursor"`
	Items      []struct {
		IntentHash     string `json:"intent_hash"`
		StateVersion   int64  `json:"state_version"`
		ConfirmedAt    string `json:"confirmed_at"`
		BalanceChanges struct {
			FungibleBalanceChanges []struct {
				EntityAddress   string `json:"entity_address"`
				ResourceAddress string `json:"resource_address"`
				BalanceChange   string `json:"balance_change"`
			} `json:"fungible_balance_changes"`
		} `json:"balance_changes"`
	} `json:"items"`
}

func (a *App) fetchRadixAddressLiveHoldings(ctx context.Context, address string, prices priceBook) ([]liveHoldingValue, error) {
	baseURLs := a.cfg.radixGatewayURLs()
	if len(baseURLs) == 0 {
		return nil, errExternalTrackerUnavailable
	}
	address = normalizeAddress(address)
	payload := map[string]any{
		"address":           address,
		"aggregation_level": "Global",
		"limit":             100,
	}
	var resp radixFungiblesPageResponse
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "/state/entity/page/fungibles/"
	})
	if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, payload, &resp); err != nil {
		return nil, err
	}
	for _, item := range resp.Items {
		if !strings.EqualFold(strings.TrimSpace(item.ResourceAddress), radixMainnetXRDResourceAddr) {
			continue
		}
		amountRaw := decimalAmountToGraphRaw(item.Amount)
		if !hasGraphableLiquidity(amountRaw) {
			continue
		}
		return []liveHoldingValue{{
			Asset:     "XRD.XRD",
			AmountRaw: amountRaw,
			USDSpot:   prices.usdFor("XRD.XRD", amountRaw),
		}}, nil
	}
	return nil, nil
}
