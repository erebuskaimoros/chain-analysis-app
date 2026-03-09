package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errExternalTrackerUnavailable = errors.New("external tracker unavailable")

const externalTrackerPageSize = 50

var (
	dogedInputRe  = regexp.MustCompile(`(?s)<div class="input-row-section2">.*?<a href="/address/([^"]+)">.*?</a>.*?<div class="input-hex">\s*<span>([^<]+)</span>\.<small>([^<]+)</small>\s*DOGE`)
	dogedOutputRe = regexp.MustCompile(`(?s)<div class="output-row-section1">.*?<a href="/address/([^"]+)">.*?</a>.*?<div class="input-hex">\s*<span>([^<]+)</span>\.<small>([^<]+)</small>\s*DOGE`)
)

const etherscanAddressTokenBalanceFeature = "addresstokenbalance"

type externalTransfer struct {
	Chain         string
	Asset         string
	AssetKind     string
	TokenStandard string
	TokenAddress  string
	TokenSymbol   string
	TokenName     string
	TokenDecimals int
	AmountRaw     string
	From          string
	To            string
	TxID          string
	Height        int64
	Time          time.Time
	ActionKey     string
	ActionLabel   string
	Confidence    float64
}

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
		return a.fetchTHORAddressLiveHoldings(ctx, address, prices, nil)
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

func (a *App) thornodeClient() *ThorClient {
	if a != nil && a.thor != nil && len(a.thor.endpoints) > 0 {
		return a.thor
	}
	if a != nil {
		return a.mid
	}
	return nil
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
	TotalBond     string `json:"total_bond"`
	BondProviders struct {
		Providers []struct {
			BondAddress string `json:"bond_address"`
			Bond        string `json:"bond"`
		} `json:"providers"`
	} `json:"bond_providers"`
}

func (a *App) fetchTHORAddressLiveHoldings(ctx context.Context, address string, prices priceBook, bondedByAddress map[string]string) ([]liveHoldingValue, error) {
	var (
		holdings []liveHoldingValue
		errs     []error
	)

	bankHoldings, err := a.fetchTHORBankHoldings(ctx, address, prices)
	if err != nil {
		errs = append(errs, err)
	} else {
		holdings = append(holdings, bankHoldings...)
	}

	lpHoldings, err := a.fetchTHORLPHoldings(ctx, address, prices)
	if err != nil {
		errs = append(errs, err)
	} else {
		holdings = append(holdings, lpHoldings...)
	}

	bondHoldings, err := a.fetchTHORBondHoldings(ctx, address, prices, bondedByAddress)
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
	client := a.thornodeClient()
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
		asset := thorDenomToAsset(balance.Denom)
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
	if a == nil || a.mid == nil {
		return nil, errExternalTrackerUnavailable
	}
	var (
		resp       midgardMemberResponse
		lastStatus int
	)
	path := "/member/" + url.PathEscape(address)
	err := a.mid.GetJSONObserved(ctx, path, &resp, func(meta RequestAttemptMeta) {
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
		pool, ok := prices.PoolSnapshots[poolAsset]
		if !ok {
			continue
		}
		runeShareRaw := mulDivAmounts(memberPool.LiquidityUnits, pool.RuneDepth, pool.LiquidityUnits)
		assetShareRaw := mulDivAmounts(memberPool.LiquidityUnits, pool.AssetDepth, pool.LiquidityUnits)
		if hasGraphableLiquidity(runeShareRaw) {
			holdings = append(holdings, liveHoldingValue{
				Asset:     "THOR.RUNE",
				AmountRaw: runeShareRaw,
				USDSpot:   prices.usdFor("THOR.RUNE", runeShareRaw),
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
	if bondedByAddress == nil {
		var err error
		bondedByAddress, err = a.fetchTHORBondedRuneIndex(ctx)
		if err != nil {
			return nil, err
		}
	}
	amountRaw := strings.TrimSpace(bondedByAddress[normalizeAddress(address)])
	if !hasGraphableLiquidity(amountRaw) {
		return nil, nil
	}
	return []liveHoldingValue{{
		Asset:     "THOR.RUNE",
		AmountRaw: amountRaw,
		USDSpot:   prices.usdFor("THOR.RUNE", amountRaw),
	}}, nil
}

func (a *App) fetchTHORBondedRuneIndex(ctx context.Context) (map[string]string, error) {
	client := a.thornodeClient()
	if client == nil {
		return nil, errExternalTrackerUnavailable
	}
	var nodes []thornodeNodeAccount
	if err := client.GetJSON(ctx, "/thorchain/nodes", &nodes); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, node := range nodes {
		for _, provider := range node.BondProviders.Providers {
			address := normalizeAddress(provider.BondAddress)
			bond := strings.TrimSpace(provider.Bond)
			if address == "" || !hasGraphableLiquidity(bond) {
				continue
			}
			out[address] = addRawAmounts(out[address], bond)
		}
	}
	return out, nil
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
		if strings.Contains(strings.ToLower(baseURL), "explorer.doged.io") {
			return fmt.Sprintf("%s/api/address/%s", baseURL, url.PathEscape(address))
		}
		return fmt.Sprintf("%s/address/%s", baseURL, url.PathEscape(trackerAddress))
	})
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
		dogeFallbackURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			baseURL = strings.TrimRight(baseURL, "/")
			if !strings.Contains(strings.ToLower(baseURL), "explorer.doged.io") {
				return ""
			}
			return fmt.Sprintf("%s/address/%s", baseURL, url.PathEscape(address))
		})
		if len(dogeFallbackURLs) > 0 {
			if err2 := a.getJSONAbsoluteMulti(ctx, dogeFallbackURLs, nil, &resp); err2 != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	chainFunded := nestedInt64(resp, "chain_stats", "funded_txo_sum")
	chainSpent := nestedInt64(resp, "chain_stats", "spent_txo_sum")
	mempoolFunded := nestedInt64(resp, "mempool_stats", "funded_txo_sum")
	mempoolSpent := nestedInt64(resp, "mempool_stats", "spent_txo_sum")
	balanceRaw := (chainFunded + mempoolFunded) - (chainSpent + mempoolSpent)

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
	allowSuperset := end.Before(time.Now().UTC().Add(-10 * time.Minute))

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
		if cached, truncated, found, err := lookupExternalTransferCache(providerCtx, a.db, provider, chain, address, startTS, endTS, maxPages, allowSuperset); err == nil && found {
			a.trackerHealth.markCache(provider, chain, true)
			return dedupeExternalTransfers(cached), truncated, externalTrackerFallbackWarning(chain, provider, skippedDegraded, failedProviders), nil
		}
		a.trackerHealth.markCache(provider, chain, false)

		transfers, truncated, warn, err := a.fetchExternalTransfersWithProvider(providerCtx, provider, chain, address, start, end, maxPages)
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
		transfers = dedupeExternalTransfers(transfers)
		if err := insertExternalTransferCache(providerCtx, a.db, provider, chain, address, startTS, endTS, maxPages, truncated, transfers); err != nil {
			logError(providerCtx, "external_transfer_cache_write_failed", err, map[string]any{
				"provider": provider,
				"chain":    chain,
				"address":  address,
			})
		}
		warn = firstNonEmpty(warn, externalTrackerFallbackWarning(chain, provider, skippedDegraded, failedProviders))
		return transfers, truncated, warn, nil
	}

	if unavailableWarning != "" && lastErr == nil {
		return nil, false, unavailableWarning, nil
	}
	if lastErr != nil {
		return nil, false, firstNonEmpty(lastWarn, externalTrackerFallbackWarning(chain, "", skippedDegraded, failedProviders)), lastErr
	}
	return nil, false, "", nil
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
			next = append(next, frontierAddress{Address: ref.Address, Chain: ref.Chain})
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

func dedupeExternalTransfers(in []externalTransfer) []externalTransfer {
	if len(in) < 2 {
		return in
	}
	seen := map[string]externalTransfer{}
	for _, item := range in {
		key := externalTransferKey(item)
		if key == "" {
			continue
		}
		if prior, ok := seen[key]; ok && prior.Confidence >= item.Confidence {
			continue
		}
		seen[key] = item
	}
	out := make([]externalTransfer, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time.Equal(out[j].Time) {
			return externalTransferKey(out[i]) < externalTransferKey(out[j])
		}
		return out[i].Time.Before(out[j].Time)
	})
	return out
}

func cosmosCoinToAssetAmount(chain, denom, amount string) (string, string) {
	denom = strings.TrimSpace(strings.ToLower(denom))
	switch denom {
	case "uatom":
		return "GAIA.ATOM", normalizeGraphAmount(amount, 6)
	default:
		return "", ""
	}
}

func humanPartsToRaw(integerPart, fractionalPart string, decimals int) int64 {
	whole := strings.NewReplacer(",", "", " ", "", "\n", "", "\t", "").Replace(strings.TrimSpace(integerPart))
	frac := strings.NewReplacer(",", "", " ", "", "\n", "", "\t", "").Replace(strings.TrimSpace(fractionalPart))
	if whole == "" && frac == "" {
		return 0
	}
	if decimals < 0 {
		decimals = 0
	}
	if len(frac) > decimals {
		frac = frac[:decimals]
	}
	for len(frac) < decimals {
		frac += "0"
	}
	raw := whole + frac
	return parseInt64(raw)
}

func isHTTPStatusError(err error, codes ...int) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	for _, code := range codes {
		if strings.Contains(text, "status="+strconv.Itoa(code)) {
			return true
		}
	}
	return false
}

func evmChainID(chain string) string {
	switch strings.ToUpper(strings.TrimSpace(chain)) {
	case "ETH":
		return "1"
	case "BSC":
		return "56"
	case "BASE":
		return "8453"
	case "AVAX":
		return "43114"
	default:
		return ""
	}
}

func evmNativeAsset(chain string) string {
	switch strings.ToUpper(strings.TrimSpace(chain)) {
	case "ETH":
		return "ETH.ETH"
	case "BSC":
		return "BSC.BNB"
	case "BASE":
		return "BASE.ETH"
	case "AVAX":
		return "AVAX.AVAX"
	default:
		return strings.ToUpper(strings.TrimSpace(chain)) + "." + strings.ToUpper(strings.TrimSpace(chain))
	}
}

func nativeAssetForChain(chain string) string {
	switch strings.ToUpper(strings.TrimSpace(chain)) {
	case "BTC":
		return "BTC.BTC"
	case "BCH":
		return "BCH.BCH"
	case "DOGE":
		return "DOGE.DOGE"
	case "LTC":
		return "LTC.LTC"
	case "SOL":
		return "SOL.SOL"
	case "TRON":
		return "TRON.TRX"
	case "XRP":
		return "XRP.XRP"
	case "GAIA":
		return "GAIA.ATOM"
	default:
		return evmNativeAsset(chain)
	}
}

func evmTokenAsset(chain, symbol, contract string) string {
	contract = normalizeTokenAddress(chain, contract)
	if contract == "" {
		return ""
	}
	return tokenAssetKey(chain, symbol, contract)
}

func normalizeGraphAmount(raw string, decimals int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return ""
	}
	switch {
	case decimals == 8:
		return n.String()
	case decimals > 8:
		return n.Div(n, pow10(decimals-8)).String()
	default:
		return n.Mul(n, pow10(8-decimals)).String()
	}
}

func pow10(exp int) *big.Int {
	if exp <= 0 {
		return big.NewInt(1)
	}
	out := big.NewInt(1)
	ten := big.NewInt(10)
	for i := 0; i < exp; i++ {
		out.Mul(out, ten)
	}
	return out
}

func tronBase58FromHex(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.ToLower(raw), "0x")
	if raw == "" {
		return ""
	}
	data, err := hex.DecodeString(raw)
	if err != nil || len(data) == 0 {
		return ""
	}
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	payload := append(append([]byte{}, data...), second[:4]...)
	return base58Encode(payload)
}

func base58Encode(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	alphabet := "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	x := new(big.Int).SetBytes(data)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	var encoded []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}
	for _, b := range data {
		if b != 0 {
			break
		}
		encoded = append(encoded, alphabet[0])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

func parseXRPLTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	const rippleEpochOffset = 946684800
	return time.Unix(value+rippleEpochOffset, 0).UTC()
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		switch t := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(t) != "" {
				return t
			}
		default:
			if strings.TrimSpace(stringifyAny(value)) != "" {
				return value
			}
		}
	}
	return nil
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func nestedValue(raw any, path ...string) any {
	current := raw
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func nestedMap(raw any, path ...string) map[string]any {
	out, _ := nestedValue(raw, path...).(map[string]any)
	return out
}

func nestedSlice(raw any, path ...string) []any {
	out, _ := nestedValue(raw, path...).([]any)
	return out
}

func nestedString(raw any, path ...string) string {
	return strings.TrimSpace(stringifyAny(nestedValue(raw, path...)))
}

func nestedInt64(raw any, path ...string) int64 {
	return parseFlexibleInt64(nestedValue(raw, path...))
}

func parseFlexibleInt64(value any) int64 {
	switch t := value.(type) {
	case nil:
		return 0
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		return parseFlexibleInt64(t.String())
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return 0
		}
		if strings.HasPrefix(strings.ToLower(raw), "0x") {
			n, err := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(raw), "0x"), 16, 64)
			if err == nil {
				return n
			}
		}
		return parseInt64(raw)
	default:
		return parseFlexibleInt64(stringifyAny(value))
	}
}

func parseFlexibleFloat64(value any) float64 {
	switch t := value.(type) {
	case nil:
		return 0
	case float64:
		return t
	case float32:
		return float64(t)
	case int64:
		return float64(t)
	case int:
		return float64(t)
	case json.Number:
		if n, err := t.Float64(); err == nil {
			return n
		}
		return parseFlexibleFloat64(t.String())
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return 0
		}
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return parseFlexibleFloat64(stringifyAny(value))
	}
}

func parseFlexibleTime(value any) time.Time {
	switch t := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return t.UTC()
	case string:
		raw := strings.TrimSpace(t)
		if raw == "" {
			return time.Time{}
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts.UTC()
		}
		if n := parseFlexibleInt64(raw); n > 0 {
			return unixFlexible(n)
		}
	case int64, int, float64, json.Number:
		if n := parseFlexibleInt64(value); n > 0 {
			return unixFlexible(n)
		}
	}
	return time.Time{}
}

func unixFlexible(value int64) time.Time {
	switch {
	case value > 1e15:
		return time.UnixMilli(value / 1e3).UTC()
	case value > 1e12:
		return time.UnixMilli(value).UTC()
	default:
		return time.Unix(value, 0).UTC()
	}
}

func extractFlexibleAddress(value any) string {
	switch t := value.(type) {
	case nil:
		return ""
	case string:
		return normalizeAddress(t)
	case map[string]any:
		return normalizeAddress(firstNonEmpty(
			nestedString(t, "address"),
			nestedString(t, "addr"),
			nestedString(t, "hash"),
		))
	default:
		return normalizeAddress(stringifyAny(value))
	}
}

func isFlexibleSuccess(value any) bool {
	switch t := value.(type) {
	case nil:
		return true
	case bool:
		return t
	case int64:
		return t == 1
	case int:
		return t == 1
	case float64:
		return int64(t) == 1
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n == 1
		}
		return isFlexibleSuccess(t.String())
	case string:
		raw := strings.TrimSpace(strings.ToLower(t))
		switch raw {
		case "", "1", "0x1", "success", "succeeded", "ok", "confirmed", "true":
			return true
		default:
			return false
		}
	default:
		return isFlexibleSuccess(stringifyAny(value))
	}
}

func mapTrackerURLs(baseURLs []string, build func(string) string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(baseURLs))
	for _, baseURL := range baseURLs {
		candidate := strings.TrimSpace(build(strings.TrimSpace(baseURL)))
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func (a *App) rotateTrackerURLs(rawURLs []string) []string {
	filtered := mapTrackerURLs(rawURLs, func(rawURL string) string { return rawURL })
	if len(filtered) < 2 {
		return filtered
	}
	idx := int(a.trackerEndpointRR.Add(1)-1) % len(filtered)
	rotated := make([]string, len(filtered))
	for i := range filtered {
		rotated[i] = filtered[(idx+i)%len(filtered)]
	}
	return rotated
}

func (a *App) lookupTrackerBlockNumber(provider, chain, closest string, timestamp int64) (int64, bool) {
	if a == nil || a.trackerBlockNums == nil {
		return 0, false
	}
	return a.trackerBlockNums.get(provider, chain, closest, timestamp)
}

func (a *App) storeTrackerBlockNumber(provider, chain, closest string, timestamp, block int64) {
	if a == nil || a.trackerBlockNums == nil {
		return
	}
	a.trackerBlockNums.set(provider, chain, closest, timestamp, block)
}

func (a *App) getJSONAbsolute(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	return a.getJSONAbsoluteMulti(ctx, []string{rawURL}, headers, out)
}

func (a *App) getJSONAbsoluteMulti(ctx context.Context, rawURLs []string, headers map[string]string, out any) error {
	var lastErr error
	candidates := a.rotateTrackerURLs(rawURLs)
	for idx, rawURL := range candidates {
		if err := a.getJSONAbsoluteSingle(ctx, rawURL, headers, out); err != nil {
			lastErr = err
			if idx < len(candidates)-1 && !sleepWithContext(ctx, backoffForAttempt(idx+1)) {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errExternalTrackerUnavailable
}

func (a *App) getJSONAbsoluteSingle(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	if meta, ok := trackerRequestMetaFromContext(ctx); ok && a.trackerThrottle != nil {
		release, err := a.trackerThrottle.acquire(ctx, meta.Provider, meta.Chain)
		if err != nil {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, 0, nil, err)
			return err
		}
		defer release()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, 0, nil, err)
		}
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := fmt.Errorf("GET %s failed: status=%d body=%s", rawURL, resp.StatusCode, trimForLog(string(body), 200))
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, httpErr)
		}
		return httpErr
	}
	if out == nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
		}
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok {
		a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
	}
	return nil
}

func (a *App) postJSONAbsolute(ctx context.Context, rawURL string, headers map[string]string, payload any, out any) error {
	return a.postJSONAbsoluteMulti(ctx, []string{rawURL}, headers, payload, out)
}

func (a *App) postJSONAbsoluteMulti(ctx context.Context, rawURLs []string, headers map[string]string, payload any, out any) error {
	var lastErr error
	candidates := a.rotateTrackerURLs(rawURLs)
	for idx, rawURL := range candidates {
		if err := a.postJSONAbsoluteSingle(ctx, rawURL, headers, payload, out); err != nil {
			lastErr = err
			if idx < len(candidates)-1 && !sleepWithContext(ctx, backoffForAttempt(idx+1)) {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errExternalTrackerUnavailable
}

func (a *App) postJSONAbsoluteSingle(ctx context.Context, rawURL string, headers map[string]string, payload any, out any) error {
	if meta, ok := trackerRequestMetaFromContext(ctx); ok && a.trackerThrottle != nil {
		release, err := a.trackerThrottle.acquire(ctx, meta.Provider, meta.Chain)
		if err != nil {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, 0, nil, err)
			return err
		}
		defer release()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "thorchain-chain-analysis/1.0")
	for key, value := range headers {
		if strings.TrimSpace(value) == "" {
			continue
		}
		req.Header.Set(key, value)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, 0, nil, err)
		}
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		httpErr := fmt.Errorf("POST %s failed: status=%d body=%s", rawURL, resp.StatusCode, trimForLog(string(body), 200))
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, httpErr)
		}
		return httpErr
	}
	if out == nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
		}
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		if meta, ok := trackerRequestMetaFromContext(ctx); ok {
			a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, err)
		}
		return err
	}
	if meta, ok := trackerRequestMetaFromContext(ctx); ok {
		a.trackerHealth.recordAttempt(meta.Provider, meta.Chain, resp.StatusCode, resp.Header, nil)
	}
	return nil
}

func (a *App) postRPCJSON(ctx context.Context, rawURLs []string, method string, params []any, out any) error {
	var env solanaRPCEnvelope
	if err := a.postJSONAbsoluteMulti(ctx, rawURLs, nil, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}, &env); err != nil {
		return err
	}
	if env.Error != nil {
		return fmt.Errorf("rpc %s failed: %v", method, env.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Result, out)
}
