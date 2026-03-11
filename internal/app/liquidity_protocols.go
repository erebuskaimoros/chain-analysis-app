package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const (
	sourceProtocolTHOR = "THOR"
	sourceProtocolMAYA = "MAYA"

	thorActionCachePrefix       = "__thor__|"
	mergedTHORActionCachePref   = "__thor_merged__|"
	mayaActionCachePrefix       = "__maya__|"
	radixMainnetXRDResourceAddr = "resource_rdx1tknxxxxxxxxxradxrdxxxxxxxxx009923554798xxxxxxxxxradxrd"
)

var thorLiquiditySupportedChains = map[string]struct{}{
	"THOR": {}, "BTC": {}, "ETH": {}, "LTC": {}, "BCH": {}, "DOGE": {},
	"AVAX": {}, "BSC": {}, "BASE": {}, "GAIA": {}, "SOL": {}, "TRON": {}, "XRP": {},
}

var mayaLiquiditySupportedChains = map[string]struct{}{
	"MAYA": {}, "ARB": {}, "BTC": {}, "ETH": {}, "THOR": {}, "XRD": {},
}

type liquidityEngine struct {
	Protocol           string
	NodeClient         *ThorClient
	MidgardClient      *ThorClient
	LegacyActionClient *ThorClient
	SupportedChains    map[string]struct{}
}

type protocolInboundAddress struct {
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Router  string `json:"router"`
	Halted  bool   `json:"halted"`
}

func normalizeSourceProtocol(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case sourceProtocolMAYA:
		return sourceProtocolMAYA
	default:
		return sourceProtocolTHOR
	}
}

func nativeAssetForProtocol(protocol string) string {
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		return "MAYA.CACAO"
	default:
		return "THOR.RUNE"
	}
}

func protocolAddressPrefix(protocol string) string {
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		return "maya"
	default:
		return "thor"
	}
}

func protocolNodesPath(protocol string) string {
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		return "/mayachain/nodes"
	default:
		return "/thorchain/nodes"
	}
}

func protocolInboundAddressesPath(protocol string) string {
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		return "/mayachain/inbound_addresses"
	default:
		return "/thorchain/inbound_addresses"
	}
}

func protocolBondDisplayLabel(protocol, address, status string) string {
	labelPrefix := "Validator"
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		labelPrefix = "MAYA Validator"
	}
	address = normalizeAddress(address)
	if address == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		return labelPrefix + " " + shortAddress(address)
	case "whitelisted":
		return "Whitelisted " + labelPrefix + " " + shortAddress(address)
	case "standby":
		return "Standby " + labelPrefix + " " + shortAddress(address)
	case "disabled":
		return "Disabled " + labelPrefix + " " + shortAddress(address)
	default:
		if labelPrefix == "Validator" {
			return "Node " + shortAddress(address)
		}
		return labelPrefix + " " + shortAddress(address)
	}
}

func protocolActionCacheKey(protocol, address string) string {
	address = normalizeAddress(address)
	if address == "" {
		return ""
	}
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		return mayaActionCachePrefix + address
	default:
		return thorActionCachePrefix + address
	}
}

func protocolPoolSnapshotKey(protocol, asset string) string {
	asset = normalizeAsset(asset)
	if asset == "" {
		return ""
	}
	return normalizeSourceProtocol(protocol) + "|" + asset
}

func protocolSupportsChain(protocol, chain string) bool {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	if chain == "" {
		return false
	}
	var supported map[string]struct{}
	switch normalizeSourceProtocol(protocol) {
	case sourceProtocolMAYA:
		supported = mayaLiquiditySupportedChains
	default:
		supported = thorLiquiditySupportedChains
	}
	_, ok := supported[chain]
	return ok
}

func (a *App) availableLiquidityEngines() []liquidityEngine {
	if a == nil {
		return nil
	}
	out := make([]liquidityEngine, 0, 2)
	if a.mid != nil {
		out = append(out, liquidityEngine{
			Protocol:           sourceProtocolTHOR,
			NodeClient:         a.thor,
			MidgardClient:      a.mid,
			LegacyActionClient: a.legacyActions,
			SupportedChains:    thorLiquiditySupportedChains,
		})
	}
	if a.mayaMid != nil {
		out = append(out, liquidityEngine{
			Protocol:        sourceProtocolMAYA,
			NodeClient:      a.mayaNode,
			MidgardClient:   a.mayaMid,
			SupportedChains: mayaLiquiditySupportedChains,
		})
	}
	return out
}

func (a *App) liquidityEngine(protocol string) (liquidityEngine, bool) {
	for _, engine := range a.availableLiquidityEngines() {
		if engine.Protocol == normalizeSourceProtocol(protocol) {
			return engine, true
		}
	}
	return liquidityEngine{}, false
}

func (a *App) actionSourceProtocolsForSeed(seed frontierAddress) []string {
	address := normalizeAddress(seed.Address)
	if address == "" {
		return nil
	}
	chain := normalizeChain(seed.Chain, address)
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	for _, engine := range a.availableLiquidityEngines() {
		if chain == "" || protocolSupportsChain(engine.Protocol, chain) {
			if _, ok := seen[engine.Protocol]; ok {
				continue
			}
			seen[engine.Protocol] = struct{}{}
			out = append(out, engine.Protocol)
		}
	}
	sort.Strings(out)
	return out
}

func annotateMidgardActions(actions []midgardAction, protocol string) []midgardAction {
	if len(actions) == 0 {
		return nil
	}
	out := append([]midgardAction(nil), actions...)
	protocol = normalizeSourceProtocol(protocol)
	for i := range out {
		out[i].SourceProtocol = protocol
	}
	return out
}

func mergeSourcedMidgardActions(groups ...[]midgardAction) []midgardAction {
	seen := map[string]struct{}{}
	out := make([]midgardAction, 0)
	for _, group := range groups {
		for _, action := range group {
			key := sourcedMidgardActionKey(action)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, action)
		}
	}
	sortMidgardActionsNewestFirst(out)
	return out
}

func sourcedMidgardActionKey(action midgardAction) string {
	key := midgardActionKey(action)
	if key == "" {
		key = midgardSyntheticTxID(action)
	}
	if key == "" {
		key = strings.Join([]string{
			strings.TrimSpace(action.Type),
			strings.TrimSpace(action.Date),
			strings.TrimSpace(action.Height),
		}, "|")
	}
	if key == "" {
		return ""
	}
	return normalizeSourceProtocol(action.SourceProtocol) + "|" + key
}

func sourceProtocolFromAction(action midgardAction) string {
	return normalizeSourceProtocol(action.SourceProtocol)
}

func (a *App) protocolInboundAddresses(ctx context.Context, protocol string) ([]protocolInboundAddress, error) {
	engine, ok := a.liquidityEngine(protocol)
	if !ok || engine.NodeClient == nil {
		return nil, fmt.Errorf("%s liquidity engine unavailable", normalizeSourceProtocol(protocol))
	}
	var entries []protocolInboundAddress
	if err := engine.NodeClient.GetJSON(ctx, protocolInboundAddressesPath(protocol), &entries); err != nil {
		return nil, err
	}
	return entries, nil
}
