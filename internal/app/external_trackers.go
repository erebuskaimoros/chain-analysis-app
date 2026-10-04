package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errExternalTrackerUnavailable = errors.New("external tracker unavailable")

const externalTrackerPageSize = 50

var (
	dogedInputRe    = regexp.MustCompile(`(?s)<div class="input-row-section2">.*?<a href="/address/([^"]+)">.*?</a>.*?<div class="input-hex">\s*<span>([^<]+)</span>\.<small>([^<]+)</small>\s*DOGE`)
	dogedOutputRe   = regexp.MustCompile(`(?s)<div class="output-row-section1">.*?<a href="/address/([^"]+)">.*?</a>.*?<div class="input-hex">\s*<span>([^<]+)</span>\.<small>([^<]+)</small>\s*DOGE`)
	dogedBalancesRe = regexp.MustCompile(`(?s)var\s+balances\s*=\s*JSON\.parse\('((?:\\.|[^'])*)'\)`)
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

func (a *App) thornodeClient() *ThorClient {
	if a != nil && a.thor != nil && len(a.thor.endpoints) > 0 {
		return a.thor
	}
	if a != nil {
		return a.mid
	}
	return nil
}

func (a *App) protocolNodeClient(protocol string) *ThorClient {
	engine, ok := a.liquidityEngine(protocol)
	if !ok {
		return nil
	}
	return engine.NodeClient
}

func (a *App) protocolMidgardClient(protocol string) *ThorClient {
	engine, ok := a.liquidityEngine(protocol)
	if !ok {
		return nil
	}
	return engine.MidgardClient
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
	case "ARB":
		return "42161"
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
	case "ARB":
		return "ARB.ETH"
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
	case "MAYA":
		return "MAYA.CACAO"
	case "XRD":
		return "XRD.XRD"
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
