package app

import (
	"fmt"
	"strings"
)

type assetMetadata struct {
	AssetKind     string
	TokenStandard string
	TokenAddress  string
	TokenSymbol   string
	TokenName     string
	TokenDecimals int
}

func nativeAssetMetadata() assetMetadata {
	return assetMetadata{AssetKind: "native"}
}

func fungibleTokenMetadata(chain, standard, address, symbol, name string, decimals int) assetMetadata {
	return assetMetadata{
		AssetKind:     "fungible_token",
		TokenStandard: strings.ToLower(strings.TrimSpace(standard)),
		TokenAddress:  normalizeTokenAddress(chain, address),
		TokenSymbol:   strings.ToUpper(strings.TrimSpace(symbol)),
		TokenName:     strings.TrimSpace(name),
		TokenDecimals: max(0, decimals),
	}
}

func normalizeTokenAddress(chain, address string) string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	switch chain {
	case "ETH", "BASE", "AVAX", "BSC":
		lower := strings.ToLower(address)
		if !strings.HasPrefix(lower, "0x") {
			lower = "0x" + strings.TrimPrefix(lower, "0x")
		}
		return lower
	default:
		return address
	}
}

func tokenStandardForChain(chain string) string {
	switch strings.ToUpper(strings.TrimSpace(chain)) {
	case "ETH", "BASE", "AVAX", "BSC":
		return "erc20"
	case "TRON":
		return "trc20"
	case "SOL":
		return "spl"
	case "GAIA":
		return "cosmos_denom"
	default:
		return ""
	}
}

func tokenAssetKey(chain, symbol, tokenID string) string {
	chain = strings.ToUpper(strings.TrimSpace(chain))
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	tokenID = strings.TrimSpace(tokenID)
	if chain == "" || tokenID == "" {
		return ""
	}
	if symbol == "" {
		symbol = "TOKEN"
	}
	return fmt.Sprintf("%s.%s-%s", chain, symbol, strings.ToUpper(tokenID))
}

func assetMetadataFromAsset(asset string) assetMetadata {
	asset = normalizeAsset(asset)
	if asset == "" {
		return assetMetadata{}
	}
	parts := strings.SplitN(asset, ".", 2)
	if len(parts) != 2 {
		return assetMetadata{}
	}
	chain := strings.ToUpper(strings.TrimSpace(parts[0]))
	symbolAndID := strings.TrimSpace(parts[1])
	if symbolAndID == "" {
		return assetMetadata{}
	}
	if !strings.Contains(symbolAndID, "-") {
		return nativeAssetMetadata()
	}
	pieces := strings.SplitN(symbolAndID, "-", 2)
	symbol := strings.ToUpper(strings.TrimSpace(pieces[0]))
	tokenID := strings.TrimSpace(pieces[1])
	if tokenID == "" {
		return assetMetadata{}
	}
	return fungibleTokenMetadata(chain, tokenStandardForChain(chain), tokenID, symbol, "", 0)
}

func mergeAssetMetadata(current, inferred assetMetadata) assetMetadata {
	if current.AssetKind == "" {
		current.AssetKind = inferred.AssetKind
	}
	if current.TokenStandard == "" {
		current.TokenStandard = inferred.TokenStandard
	}
	if current.TokenAddress == "" {
		current.TokenAddress = inferred.TokenAddress
	}
	if current.TokenSymbol == "" {
		current.TokenSymbol = inferred.TokenSymbol
	}
	if current.TokenName == "" {
		current.TokenName = inferred.TokenName
	}
	if current.TokenDecimals == 0 {
		current.TokenDecimals = inferred.TokenDecimals
	}
	return current
}
