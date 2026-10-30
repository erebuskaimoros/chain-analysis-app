package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BNB Beacon Chain (Binance Chain, bnb1… addresses) was shut down in December
// 2024, but its explorer still serves the archived transaction index. Before
// THORChain's multichain mainnet, THORChain ran as BEPSwap on this chain: users
// paid BEP2 RUNE and BEP2 tokens into vault addresses with THORChain memos and
// the vaults paid out with OUTBOUND:/REFUND: memos. Those memos are the only
// way to follow pre-mainnet THORChain wallets, so this tracker reads them and
// links the wallets on either side of the vault.

const (
	bnbBeaconRowsPerPage = 100 // the explorer rejects rows > 100
	// bnbBeaconOutboundLookupLimit caps the per-fetch lookups of the inbound
	// transaction behind an OUTBOUND:<hash> memo.
	bnbBeaconOutboundLookupLimit = 25

	bnbBeaconActionTransfer = "tracker.bnb.transfer"
	bnbBeaconActionDeposit  = "tracker.bnb.thorchain_deposit"
	bnbBeaconActionOutbound = "tracker.bnb.thorchain_outbound"
	bnbBeaconActionRefund   = "tracker.bnb.thorchain_refund"
	bnbBeaconActionVault    = "tracker.bnb.thorchain_vault"
	bnbBeaconActionSwap     = "tracker.bnb.memo_link.swap"
	bnbBeaconActionSwitch   = "tracker.bnb.memo_link.switch"
	bnbBeaconActionBond     = "tracker.bnb.memo_link.bond"

	// externalTransfer.ProtocolSide values: which end of a transfer is a
	// THORChain vault rather than a wallet.
	protocolSideFrom = "from"
	protocolSideTo   = "to"
	protocolSideBoth = "both"
)

type bnbBeaconTx struct {
	TxHash        string      `json:"txHash"`
	BlockHeight   int64       `json:"blockHeight"`
	TxType        string      `json:"txType"`
	TimeStamp     int64       `json:"timeStamp"`
	FromAddr      string      `json:"fromAddr"`
	ToAddr        string      `json:"toAddr"`
	Value         json.Number `json:"value"`
	TxAsset       string      `json:"txAsset"`
	Code          int         `json:"code"`
	Memo          string      `json:"memo"`
	HasChildren   int         `json:"hasChildren"`
	MappedTxAsset string      `json:"mappedTxAsset"`
	// sub-tx-list legs name these hash, height and asset.
	Hash   string `json:"hash"`
	Height int64  `json:"height"`
	Asset  string `json:"asset"`
}

type bnbBeaconTxsResponse struct {
	TxNums  int64         `json:"txNums"`
	TxArray []bnbBeaconTx `json:"txArray"`
}

// thorchainMemo is a parsed THORChain memo as used on BNB Beacon Chain.
type thorchainMemo struct {
	Kind   string // swap, add, withdraw, donate, bond, unbond, switch, outbound, refund, vault
	Asset  string
	Target string // destination address (swap/add), node or THOR address (bond/switch), or tx hash (outbound/refund)
	Legacy bool   // OUTBOUND: is the BEPSwap form; mainnet uses OUT:
}

func parseThorchainMemo(memo string) (thorchainMemo, bool) {
	memo = strings.TrimSpace(memo)
	if memo == "" {
		return thorchainMemo{}, false
	}
	parts := strings.Split(memo, ":")
	field := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	head := strings.ToUpper(field(0))
	switch head {
	case "SWAP", "S", "=":
		return thorchainMemo{Kind: "swap", Asset: field(1), Target: field(2)}, true
	case "ADD", "A", "+", "STAKE", "ST":
		return thorchainMemo{Kind: "add", Asset: field(1), Target: field(2)}, true
	case "WITHDRAW", "WD", "-", "UNSTAKE":
		return thorchainMemo{Kind: "withdraw", Asset: field(1)}, true
	case "DONATE", "D":
		return thorchainMemo{Kind: "donate", Asset: field(1)}, true
	case "BOND":
		return thorchainMemo{Kind: "bond", Target: field(1)}, true
	case "UNBOND", "LEAVE":
		return thorchainMemo{Kind: "unbond", Target: field(1)}, true
	case "SWITCH":
		return thorchainMemo{Kind: "switch", Target: field(1)}, true
	case "OUTBOUND":
		return thorchainMemo{Kind: "outbound", Target: field(1), Legacy: true}, true
	case "OUT":
		return thorchainMemo{Kind: "outbound", Target: field(1)}, true
	case "REFUND":
		return thorchainMemo{Kind: "refund", Target: field(1)}, true
	case "YGGDRASIL+", "YGGDRASIL-", "MIGRATE", "RAGNAROK", "CONSOLIDATE", "RESERVE", "NOOP":
		return thorchainMemo{Kind: "vault"}, true
	}
	return thorchainMemo{}, false
}

func bnbBeaconAsset(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return ""
	}
	return "BNB." + symbol
}

// bnbBeaconAmountRaw converts the explorer's decimal value to 1e8 base units.
// BEP2 tokens all use 8 decimals.
func bnbBeaconAmountRaw(value json.Number) string {
	raw := strings.TrimSpace(value.String())
	if strings.ContainsAny(raw, "eE") {
		f, ok := new(big.Float).SetString(raw)
		if !ok {
			return ""
		}
		raw = f.Text('f', 8)
	}
	amount := decimalAmountToGraphRaw(raw)
	if parseInt64(amount) <= 0 {
		return ""
	}
	return amount
}

func (c Config) bnbBeaconExplorerURLs() []string {
	return parseURLListValue(c.BNBBeaconExplorerURL)
}

func (a *App) fetchBNBBeaconTransfers(ctx context.Context, address string, start, end time.Time, maxPages int) ([]externalTransfer, bool, error) {
	baseURLs := a.cfg.bnbBeaconExplorerURLs()
	if len(baseURLs) == 0 {
		return nil, false, errExternalTrackerUnavailable
	}
	address = normalizeAddress(address)
	if maxPages < 1 {
		maxPages = 1
	}

	// The explorer lists newest first and caps page*rows, so walk backwards
	// by moving endTime to the oldest row seen instead of paging.
	startMS := start.UnixMilli()
	endMS := end.UnixMilli()
	var (
		all       []externalTransfer
		seen      = map[string]struct{}{}
		truncated bool
		lookups   int
	)
	for page := 0; page < maxPages; page++ {
		query := url.Values{}
		query.Set("address", address)
		query.Set("page", "1")
		query.Set("rows", strconv.Itoa(bnbBeaconRowsPerPage))
		if startMS > 0 {
			query.Set("startTime", strconv.FormatInt(startMS, 10))
		}
		query.Set("endTime", strconv.FormatInt(endMS, 10))
		rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
			return strings.TrimRight(baseURL, "/") + "/api/v1/txs?" + query.Encode()
		})
		var resp bnbBeaconTxsResponse
		if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &resp); err != nil {
			return nil, false, err
		}
		oldest := endMS
		fresh := 0
		for _, tx := range resp.TxArray {
			if tx.TimeStamp > 0 && tx.TimeStamp < oldest {
				oldest = tx.TimeStamp
			}
			if _, ok := seen[tx.TxHash]; ok {
				continue
			}
			seen[tx.TxHash] = struct{}{}
			fresh++
			transfers, err := a.bnbBeaconTxTransfers(ctx, baseURLs, address, tx, &lookups)
			if err != nil {
				return nil, false, err
			}
			all = append(all, transfers...)
		}
		if len(resp.TxArray) < bnbBeaconRowsPerPage {
			break
		}
		if fresh == 0 || oldest >= endMS {
			// More than a page of transactions share one millisecond; the
			// rest cannot be reached by time windows.
			truncated = true
			break
		}
		endMS = oldest
		if page+1 >= maxPages {
			truncated = true
		}
	}
	return dedupeExternalTransfers(all), truncated, nil
}

// bnbBeaconTxTransfers turns one explorer row into transfers that touch the
// watched address, adding memo-derived links between the wallets on either
// side of a THORChain vault.
func (a *App) bnbBeaconTxTransfers(ctx context.Context, baseURLs []string, watched string, tx bnbBeaconTx, lookups *int) ([]externalTransfer, error) {
	if tx.Code != 0 || !strings.EqualFold(strings.TrimSpace(tx.TxType), "TRANSFER") {
		return nil, nil
	}
	legs := []bnbBeaconTx{tx}
	if tx.HasChildren != 0 {
		children, err := a.fetchBNBBeaconSubTxs(ctx, baseURLs, tx)
		if err != nil {
			return nil, err
		}
		legs = children
	}
	var out []externalTransfer
	for _, leg := range legs {
		from := normalizeAddress(leg.FromAddr)
		to := normalizeAddress(leg.ToAddr)
		if from != watched && to != watched {
			continue
		}
		transfers, err := a.bnbBeaconLegTransfers(ctx, baseURLs, watched, leg, lookups)
		if err != nil {
			return nil, err
		}
		out = append(out, transfers...)
	}
	return out, nil
}

func (a *App) bnbBeaconLegTransfers(ctx context.Context, baseURLs []string, watched string, leg bnbBeaconTx, lookups *int) ([]externalTransfer, error) {
	base := externalTransfer{
		Chain:       "BNB",
		Asset:       bnbBeaconAsset(leg.TxAsset),
		AmountRaw:   bnbBeaconAmountRaw(leg.Value),
		From:        normalizeAddress(leg.FromAddr),
		To:          normalizeAddress(leg.ToAddr),
		TxID:        cleanTxID(leg.TxHash),
		Height:      leg.BlockHeight,
		Time:        time.UnixMilli(leg.TimeStamp).UTC(),
		Memo:        strings.TrimSpace(leg.Memo),
		ActionKey:   bnbBeaconActionTransfer,
		ActionLabel: "BNB Beacon Transfer",
		Confidence:  0.99,
	}
	if base.Asset == "" || base.AmountRaw == "" || base.From == "" || base.To == "" || base.TxID == "" {
		return nil, nil
	}
	memo, ok := parseThorchainMemo(base.Memo)
	if !ok {
		return []externalTransfer{base}, nil
	}

	switch memo.Kind {
	case "switch", "bond", "unbond":
		target := normalizeAddress(memo.Target)
		if memo.Kind == "unbond" || !strings.HasPrefix(target, "thor1") {
			break
		}
		// The vault only burns or escrows the BEP2 RUNE; the real
		// counterparty is the native THORChain address in the memo.
		link := base
		link.To = target
		link.Confidence = 0.95
		if memo.Kind == "switch" {
			link.ActionKey, link.ActionLabel = bnbBeaconActionSwitch, "RUNE Switch (BEP2 → Native)"
		} else {
			link.ActionKey, link.ActionLabel = bnbBeaconActionBond, "Bond (BEP2 RUNE)"
		}
		return []externalTransfer{link}, nil
	case "outbound", "refund":
		vaultLeg := base
		vaultLeg.ProtocolSide = protocolSideFrom
		if memo.Kind == "refund" {
			vaultLeg.ActionKey, vaultLeg.ActionLabel = bnbBeaconActionRefund, "THORChain Refund"
		} else {
			vaultLeg.ActionKey, vaultLeg.ActionLabel = bnbBeaconActionOutbound, "THORChain Outbound"
		}
		out := []externalTransfer{vaultLeg}
		// Mainnet outbounds are in Midgard. BEPSwap outbounds are not, so
		// look up the inbound they answer to find who paid for them.
		if memo.Kind == "outbound" && memo.Legacy && vaultLeg.To == watched && *lookups < bnbBeaconOutboundLookupLimit {
			*lookups++
			origin, err := a.fetchBNBBeaconTx(ctx, baseURLs, memo.Target)
			if err != nil {
				logError(ctx, "bnb_beacon_outbound_lookup_failed", err, map[string]any{"tx": memo.Target})
				return out, nil
			}
			if link, ok := bnbBeaconSwapLink(origin); ok && link.To == watched {
				link.ActionLabel = "THORChain Swap (via outbound)"
				out = append(out, link)
			}
		}
		return out, nil
	case "vault":
		vaultLeg := base
		vaultLeg.ProtocolSide = protocolSideBoth
		vaultLeg.ActionKey, vaultLeg.ActionLabel = bnbBeaconActionVault, "THORChain Vault Transfer"
		return []externalTransfer{vaultLeg}, nil
	}

	deposit := base
	deposit.ProtocolSide = protocolSideTo
	deposit.ActionKey = bnbBeaconActionDeposit
	deposit.ActionLabel = "THORChain Deposit (" + strings.ToUpper(memo.Kind) + ")"
	out := []externalTransfer{deposit}
	if link, ok := bnbBeaconSwapLink(leg); ok {
		out = append(out, link)
	}
	return out, nil
}

// bnbBeaconSwapLink returns a direct sender-to-recipient link for a swap
// deposit whose memo pays out to another BNB address. Midgard consumes the
// link for mainnet swaps it already knows (same sender, tx and coin), so it
// only survives for BEPSwap-era swaps.
func bnbBeaconSwapLink(tx bnbBeaconTx) (externalTransfer, bool) {
	memo, ok := parseThorchainMemo(tx.Memo)
	if !ok || memo.Kind != "swap" {
		return externalTransfer{}, false
	}
	from := normalizeAddress(tx.FromAddr)
	dest := normalizeAddress(memo.Target)
	if !strings.HasPrefix(dest, "bnb1") || dest == from {
		return externalTransfer{}, false
	}
	link := externalTransfer{
		Chain:       "BNB",
		Asset:       bnbBeaconAsset(tx.TxAsset),
		AmountRaw:   bnbBeaconAmountRaw(tx.Value),
		From:        from,
		To:          dest,
		TxID:        cleanTxID(tx.TxHash),
		Height:      tx.BlockHeight,
		Time:        time.UnixMilli(tx.TimeStamp).UTC(),
		Memo:        strings.TrimSpace(tx.Memo),
		ActionKey:   bnbBeaconActionSwap,
		ActionLabel: "THORChain Swap (memo destination)",
		Confidence:  0.9,
	}
	if link.Asset == "" || link.AmountRaw == "" || link.From == "" || link.TxID == "" {
		return externalTransfer{}, false
	}
	return link, true
}

func (a *App) fetchBNBBeaconTx(ctx context.Context, baseURLs []string, txHash string) (bnbBeaconTx, error) {
	txHash = cleanTxID(txHash)
	if txHash == "" {
		return bnbBeaconTx{}, fmt.Errorf("empty tx hash")
	}
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "/api/v1/tx?txHash=" + url.QueryEscape(txHash)
	})
	var tx bnbBeaconTx
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &tx); err != nil {
		return bnbBeaconTx{}, err
	}
	return tx, nil
}

// fetchBNBBeaconSubTxs returns the legs of a multi-send, whose list row has
// no recipient, value or asset.
func (a *App) fetchBNBBeaconSubTxs(ctx context.Context, baseURLs []string, parent bnbBeaconTx) ([]bnbBeaconTx, error) {
	rawURLs := mapTrackerURLs(baseURLs, func(baseURL string) string {
		return strings.TrimRight(baseURL, "/") + "/api/v1/sub-tx-list?txHash=" + url.QueryEscape(parent.TxHash) + "&page=1&rows=" + strconv.Itoa(bnbBeaconRowsPerPage)
	})
	var raw map[string]json.RawMessage
	if err := a.getJSONAbsoluteMulti(ctx, rawURLs, nil, &raw); err != nil {
		return nil, err
	}
	// The list sits under a single array-valued key; take whichever it is.
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var legs []bnbBeaconTx
		if err := json.Unmarshal(raw[key], &legs); err != nil || len(legs) == 0 {
			continue
		}
		for i := range legs {
			legs[i].TxHash = firstNonEmpty(legs[i].TxHash, legs[i].Hash, parent.TxHash)
			legs[i].TxAsset = firstNonEmpty(legs[i].TxAsset, legs[i].Asset)
			legs[i].BlockHeight = firstNonZeroInt64(legs[i].BlockHeight, legs[i].Height)
			legs[i].TxType = firstNonEmpty(legs[i].TxType, parent.TxType)
			legs[i].Memo = firstNonEmpty(legs[i].Memo, parent.Memo)
			legs[i].BlockHeight = firstNonZeroInt64(legs[i].BlockHeight, parent.BlockHeight)
			legs[i].TimeStamp = firstNonZeroInt64(legs[i].TimeStamp, parent.TimeStamp)
			legs[i].Code = parent.Code
			legs[i].HasChildren = 0
		}
		return legs, nil
	}
	return nil, nil
}
