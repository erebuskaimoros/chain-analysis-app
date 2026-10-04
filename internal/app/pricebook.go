package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) loadProtocolDirectory(ctx context.Context) (protocolDirectory, error) {
	a.protocolDirectory.init(protocolDirectoryCacheTTL, cloneProtocolDirectory)
	return a.protocolDirectory.get(ctx, a.loadProtocolDirectoryFresh)
}

func (a *App) loadProtocolDirectoryFresh(ctx context.Context) (protocolDirectory, error) {
	out := protocolDirectory{
		AddressKinds: map[string]protocolAddress{},
		SupportedChains: map[string]struct{}{
			"BTC": {}, "ETH": {}, "LTC": {}, "BCH": {}, "DOGE": {},
			"AVAX": {}, "BSC": {}, "GAIA": {}, "BASE": {},
			"SOL": {}, "TRON": {}, "XRP": {}, "THOR": {},
			"MAYA": {}, "ARB": {}, "XRD": {},
		},
	}

	// Populate from hardcoded known address maps.
	for addr, label := range knownAddressLabels {
		out.AddressKinds[normalizeAddress(addr)] = protocolAddress{
			Kind:  "known",
			Label: label,
		}
	}
	for addr, label := range frontierBlacklist {
		out.AddressKinds[normalizeAddress(addr)] = protocolAddress{
			Kind:  "module",
			Label: label,
		}
	}
	for addr := range graphExcludedAddresses {
		if _, exists := out.AddressKinds[normalizeAddress(addr)]; !exists {
			out.AddressKinds[normalizeAddress(addr)] = protocolAddress{
				Kind:  "excluded",
				Label: "Protocol",
			}
		}
	}

	for _, protocol := range []string{sourceProtocolTHOR, sourceProtocolMAYA} {
		inboundCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		entries, err := a.protocolInboundAddresses(inboundCtx, protocol)
		cancel()
		if err != nil {
			logError(ctx, "protocol_inbound_addresses_failed", err, map[string]any{
				"protocol": protocol,
			})
			continue
		}
		for _, entry := range entries {
			chain := strings.ToUpper(strings.TrimSpace(entry.Chain))
			if chain == "" || !protocolSupportsChain(protocol, chain) {
				continue
			}
			out.SupportedChains[chain] = struct{}{}
			if address := normalizeAddress(entry.Address); address != "" {
				out.AddressKinds[address] = protocolAddress{
					Kind:  "inbound",
					Chain: chain,
					Label: protocol + " " + chain + " Inbound",
				}
			}
			if router := normalizeAddress(entry.Router); router != "" {
				out.AddressKinds[router] = protocolAddress{
					Kind:  "router",
					Chain: chain,
					Label: protocol + " " + chain + " Router",
				}
			}
		}
	}

	return out, nil
}

func (a *App) buildPriceBook(ctx context.Context) (priceBook, error) {
	a.priceBook.init(priceBookCacheTTL, clonePriceBook)
	return a.priceBook.get(ctx, a.buildPriceBookFresh)
}

func (a *App) buildPriceBookFresh(ctx context.Context) (priceBook, error) {
	book := priceBook{
		NativeUSD:     map[string]float64{},
		AssetUSD:      map[string]float64{},
		PoolAssets:    map[string]struct{}{},
		PoolSnapshots: map[string]MidgardPool{},
		PoolProtocols: map[string]string{},
		HasPoolData:   true,
	}
	assetUSDs := map[string][]float64{}
	nativeUSDs := map[string][]float64{}
	var firstErr error
	for _, engine := range a.availableLiquidityEngines() {
		pools, err := a.fetchPoolsForProtocol(ctx, engine.Protocol)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, pool := range pools {
			if !strings.EqualFold(pool.Status, "available") && pool.Status != "" {
				continue
			}
			asset := normalizeAsset(pool.Asset)
			if asset == "" {
				continue
			}
			book.PoolAssets[asset] = struct{}{}
			key := protocolPoolSnapshotKey(engine.Protocol, asset)
			book.PoolSnapshots[key] = pool
			book.PoolProtocols[key] = engine.Protocol

			assetPriceUSD := parseFloat64(pool.AssetPriceUSD)
			if assetPriceUSD > 0 {
				assetUSDs[asset] = append(assetUSDs[asset], assetPriceUSD)
			}

			nativeDepth := float64(parseInt64(pool.RuneDepth)) / 1e8
			assetDepth := float64(parseInt64(pool.AssetDepth)) / 1e8
			if nativeDepth > 0 && assetDepth > 0 && isStableAsset(asset) {
				nativeUSDs[engine.Protocol] = append(nativeUSDs[engine.Protocol], assetDepth/nativeDepth)
			}
		}
	}

	for asset, prices := range assetUSDs {
		sort.Float64s(prices)
		book.AssetUSD[asset] = prices[len(prices)/2]
	}
	for protocol, prices := range nativeUSDs {
		sort.Float64s(prices)
		median := prices[len(prices)/2]
		book.NativeUSD[protocol] = median
		book.AssetUSD[nativeAssetForProtocol(protocol)] = median
	}
	if len(book.AssetUSD) == 0 && firstErr != nil {
		return priceBook{}, firstErr
	}
	if len(book.NativeUSD) == 0 {
		return book, fmt.Errorf("no stable pools available for USD normalization")
	}
	return book, nil
}

func parseFloat64(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return f
}

func (p priceBook) usdFor(asset, amountRaw string) float64 {
	asset = normalizeAsset(asset)
	if asset == "" || amountRaw == "" {
		return 0
	}
	price, ok := p.AssetUSD[asset]
	if !ok || price <= 0 {
		return 0
	}
	amount := float64(parseInt64(amountRaw)) / 1e8
	return amount * price
}

func (p priceBook) hasPoolAsset(asset string) bool {
	asset = normalizeAsset(asset)
	if asset == "" || len(p.PoolAssets) == 0 {
		return false
	}
	_, ok := p.PoolAssets[asset]
	return ok
}

func (p priceBook) supportsGraphAsset(asset string) bool {
	meta := assetMetadataFromAsset(asset)
	if meta.AssetKind != "fungible_token" {
		return true
	}
	if !p.HasPoolData || len(p.PoolAssets) == 0 {
		return true
	}
	return p.hasPoolAsset(asset)
}
