package app

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Prices at transaction time. Flows are valued with the asset's USD price in
// the hour (or day) they happened, instead of today's spot price. Sources, in
// order: THORChain pool history from Midgard, a $1 peg for stablecoins,
// DefiLlama for EVM tokens without a pool, and today's spot price as a labeled
// fallback. Past prices never change, so fetched points are kept in
// price_points permanently.

const (
	priceSourcePoolHistory = "midgard_pool_history"
	priceSourceRuneHistory = "midgard_rune_history"
	priceSourceStablePeg   = "stable_peg"
	priceSourceDefiLlama   = "defillama"
	priceSourceSpot        = "spot_fallback"

	priceIntervalHour = "hour"
	priceIntervalDay  = "day"

	// Assets needed on at most this many distinct days get hourly prices;
	// wider spans use one daily-interval request per asset.
	hourlyPriceDayLimit = 3
	priceHistoryMaxDays = 365
	secondsPerDay       = int64(86400)
)

// priceHistory is the in-memory view of historical prices one build uses.
// Projection only reads it; loading happens before each frontier is projected.
type priceHistory struct {
	mu     sync.Mutex
	hourly map[string]map[int64]float64
	daily  map[string]map[int64]float64
	source map[string]string
	loaded map[string]map[int64]bool // asset -> day start -> attempted
}

func newPriceHistory() *priceHistory {
	return &priceHistory{
		hourly: map[string]map[int64]float64{},
		daily:  map[string]map[int64]float64{},
		source: map[string]string{},
		loaded: map[string]map[int64]bool{},
	}
}

func dayStartUnix(t time.Time) int64 {
	return t.Unix() - t.Unix()%secondsPerDay
}

func (h *priceHistory) set(asset, interval string, bucket int64, usd float64, source string) {
	if h == nil || !validPrice(usd) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	target := h.hourly
	if interval == priceIntervalDay {
		target = h.daily
	}
	if target[asset] == nil {
		target[asset] = map[int64]float64{}
	}
	target[asset][bucket] = usd
	h.source[asset] = source
}

func (h *priceHistory) markLoaded(asset string, day int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.loaded[asset] == nil {
		h.loaded[asset] = map[int64]bool{}
	}
	h.loaded[asset][day] = true
}

func (h *priceHistory) isLoaded(asset string, day int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.loaded[asset][day]
}

// priceAt returns the asset's USD price around t: the hour bucket containing t
// (or the nearest within three hours), else the day bucket.
func (h *priceHistory) priceAt(asset string, t time.Time) (float64, string, bool) {
	if h == nil || t.IsZero() {
		return 0, "", false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	hour := t.Unix() - t.Unix()%3600
	if buckets := h.hourly[asset]; buckets != nil {
		for _, offset := range []int64{0, -3600, 3600, -7200, 7200, -10800, 10800} {
			if usd, ok := buckets[hour+offset]; ok {
				return usd, h.source[asset], true
			}
		}
	}
	if buckets := h.daily[asset]; buckets != nil {
		if usd, ok := buckets[dayStartUnix(t)]; ok {
			return usd, h.source[asset], true
		}
	}
	return 0, "", false
}

// historyPoolAsset maps an asset to the THORChain pool whose history prices
// it: the asset itself, or the L1 asset behind synth/trade/secured notation.
func historyPoolAsset(prices priceBook, asset string) string {
	asset = normalizeAsset(asset)
	if asset == "THOR.RUNE" || prices.hasPoolAsset(asset) {
		return asset
	}
	for _, sep := range []string{"/", "~", "-"} {
		if chain, rest, ok := strings.Cut(asset, sep); ok && !strings.Contains(chain, ".") {
			if candidate := chain + "." + rest; prices.hasPoolAsset(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// defiLlamaCoin returns the DefiLlama coin id for an EVM token asset such as
// ETH.USDC-0XA0B8..., or "" when the asset is not an EVM token.
func defiLlamaCoin(asset string) string {
	chain, rest, ok := strings.Cut(normalizeAsset(asset), ".")
	if !ok {
		return ""
	}
	network := map[string]string{"ETH": "ethereum", "BSC": "bsc", "AVAX": "avax", "BASE": "base", "ARB": "arbitrum"}[chain]
	if network == "" {
		return ""
	}
	_, contract, ok := strings.Cut(rest, "-")
	contract = strings.ToLower(strings.TrimSpace(contract))
	if !ok || !strings.HasPrefix(contract, "0x") {
		return ""
	}
	return network + ":" + contract
}

// priceNeeds collects, per asset, the days a set of flows needs priced.
type priceNeeds map[string]map[int64]struct{}

func (n priceNeeds) add(asset string, t time.Time) {
	asset = normalizeAsset(asset)
	if asset == "" || t.IsZero() {
		return
	}
	if n[asset] == nil {
		n[asset] = map[int64]struct{}{}
	}
	n[asset][dayStartUnix(t)] = struct{}{}
}

func priceNeedsForFlows(actions []midgardAction, transfers []externalTransfer) priceNeeds {
	needs := priceNeeds{}
	for _, action := range actions {
		at := parseMidgardActionTime(action.Date)
		for _, legs := range [][]midgardActionLeg{action.In, action.Out} {
			for _, leg := range legs {
				for _, coin := range leg.Coins {
					needs.add(coin.Asset, at)
				}
			}
		}
	}
	for _, transfer := range transfers {
		needs.add(transfer.Asset, transfer.Time)
	}
	return needs
}

// preloadPriceHistory makes sure history holds prices for every (asset, day)
// in needs, reading price_points first and fetching what is missing.
// Failures are logged and leave the asset to the stable peg or spot fallback.
func (a *App) preloadPriceHistory(ctx context.Context, history *priceHistory, prices priceBook, needs priceNeeds) {
	if history == nil || len(needs) == 0 {
		return
	}
	now := time.Now().UTC()
	llamaDays := map[int64][]string{}
	for asset, days := range needs {
		var missing []int64
		for day := range days {
			if !history.isLoaded(asset, day) {
				missing = append(missing, day)
			}
		}
		if len(missing) == 0 {
			continue
		}
		pool := historyPoolAsset(prices, asset)
		coin := ""
		if pool == "" && !isStableAsset(asset) {
			coin = defiLlamaCoin(asset)
		}
		for _, day := range missing {
			history.markLoaded(asset, day)
		}
		if pool == "" && coin == "" {
			continue
		}
		missing = a.loadStoredPricePoints(ctx, history, asset, missing, now)
		if len(missing) == 0 {
			continue
		}
		if coin != "" {
			for _, day := range missing {
				llamaDays[day] = append(llamaDays[day], asset)
			}
			continue
		}
		if err := a.fetchPoolPriceHistory(ctx, history, asset, pool, missing, now); err != nil {
			logError(ctx, "price_history_fetch_failed", err, map[string]any{"asset": asset, "pool": pool, "days": len(missing)})
		}
	}
	for day, assets := range llamaDays {
		if err := a.fetchDefiLlamaPrices(ctx, history, assets, day, now); err != nil {
			logError(ctx, "price_history_fetch_failed", err, map[string]any{"source": priceSourceDefiLlama, "assets": len(assets)})
		}
	}
}

// loadStoredPricePoints fills history from price_points and returns the days
// still missing. Days that are not over yet are always re-fetched.
func (a *App) loadStoredPricePoints(ctx context.Context, history *priceHistory, asset string, days []int64, now time.Time) []int64 {
	var missing []int64
	today := dayStartUnix(now)
	for _, day := range days {
		if day >= today {
			missing = append(missing, day)
			continue
		}
		rows, err := a.db.QueryContext(ctx, `
			SELECT interval, bucket_ts, usd, source FROM price_points
			WHERE asset = ? AND bucket_ts >= ? AND bucket_ts < ?
		`, asset, day, day+secondsPerDay)
		if err != nil {
			missing = append(missing, day)
			continue
		}
		found := false
		for rows.Next() {
			var interval, source string
			var bucket int64
			var usd float64
			if rows.Scan(&interval, &bucket, &usd, &source) == nil {
				history.set(asset, interval, bucket, usd, source)
				found = true
			}
		}
		rows.Close()
		if !found {
			missing = append(missing, day)
		}
	}
	return missing
}

func (a *App) storePricePoint(ctx context.Context, asset, interval string, bucket int64, usd float64, source string, now time.Time) {
	// Only completed buckets are permanent; the current day is re-fetched.
	if bucket >= dayStartUnix(now) {
		return
	}
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO price_points(asset, interval, bucket_ts, usd, source) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(asset, interval, bucket_ts) DO UPDATE SET usd = excluded.usd, source = excluded.source
	`, asset, interval, bucket, usd, source); err != nil {
		logError(ctx, "price_point_store_failed", err, map[string]any{"asset": asset})
	}
}

type midgardPriceInterval struct {
	StartTime     string `json:"startTime"`
	AssetPriceUSD string `json:"assetPriceUSD"`
	RunePriceUSD  string `json:"runePriceUSD"`
}

func (a *App) fetchPoolPriceHistory(ctx context.Context, history *priceHistory, asset, pool string, days []int64, now time.Time) error {
	if a.mid == nil {
		return fmt.Errorf("midgard unavailable")
	}
	interval := priceIntervalDay
	if len(days) <= hourlyPriceDayLimit {
		interval = priceIntervalHour
	}
	var ranges [][2]int64
	if interval == priceIntervalHour {
		for _, day := range days {
			ranges = append(ranges, [2]int64{day, day + secondsPerDay})
		}
	} else {
		lo, hi := days[0], days[0]
		for _, day := range days {
			lo, hi = min(lo, day), max(hi, day)
		}
		for start := lo; start <= hi; start += priceHistoryMaxDays * secondsPerDay {
			ranges = append(ranges, [2]int64{start, min(hi+secondsPerDay, start+priceHistoryMaxDays*secondsPerDay)})
		}
	}
	source := priceSourcePoolHistory
	endpoint := "/history/depths/" + url.PathEscape(pool)
	if pool == "THOR.RUNE" {
		source = priceSourceRuneHistory
		endpoint = "/history/rune"
	}
	for _, r := range ranges {
		params := url.Values{}
		params.Set("interval", interval)
		params.Set("from", strconv.FormatInt(r[0], 10))
		params.Set("to", strconv.FormatInt(r[1], 10))
		var response struct {
			Intervals []midgardPriceInterval `json:"intervals"`
		}
		if err := a.mid.GetJSON(ctx, endpoint+"?"+params.Encode(), &response); err != nil {
			return err
		}
		for _, iv := range response.Intervals {
			bucket := parseInt64(iv.StartTime)
			usd := parseFloat64(iv.AssetPriceUSD)
			if pool == "THOR.RUNE" {
				usd = parseFloat64(iv.RunePriceUSD)
			}
			if bucket <= 0 || !validPrice(usd) {
				continue
			}
			history.set(asset, interval, bucket, usd, source)
			a.storePricePoint(ctx, asset, interval, bucket, usd, source, now)
		}
	}
	return nil
}

// fetchDefiLlamaPrices prices EVM tokens at the middle of day in one request.
func (a *App) fetchDefiLlamaPrices(ctx context.Context, history *priceHistory, assets []string, day int64, now time.Time) error {
	if strings.TrimSpace(a.cfg.DefiLlamaURL) == "" {
		return nil
	}
	coinByAsset := map[string]string{}
	coins := make([]string, 0, len(assets))
	sort.Strings(assets)
	for _, asset := range assets {
		if coin := defiLlamaCoin(asset); coin != "" {
			coinByAsset[asset] = coin
			coins = append(coins, coin)
		}
	}
	if len(coins) == 0 {
		return nil
	}
	at := day + secondsPerDay/2
	if at > now.Unix() {
		at = now.Unix()
	}
	var response struct {
		Coins map[string]struct {
			Price float64 `json:"price"`
		} `json:"coins"`
	}
	rawURL := fmt.Sprintf("%s/prices/historical/%d/%s", strings.TrimRight(a.cfg.DefiLlamaURL, "/"), at, strings.Join(coins, ","))
	if err := a.getJSONAbsolute(withTrackerRequestMeta(ctx, "defillama", "PRICES"), rawURL, nil, &response); err != nil {
		return err
	}
	for asset, coin := range coinByAsset {
		if entry, ok := response.Coins[coin]; ok && validPrice(entry.Price) {
			history.set(asset, priceIntervalDay, day, entry.Price, priceSourceDefiLlama)
			a.storePricePoint(ctx, asset, priceIntervalDay, day, entry.Price, priceSourceDefiLlama, now)
		}
	}
	return nil
}

// usdAtTime values amountRaw (1e8-scaled) of asset at t. It reports the price
// source and false when no price is known at all.
func usdAtTime(history *priceHistory, prices priceBook, asset, amountRaw string, t time.Time) (float64, string, bool) {
	asset = normalizeAsset(asset)
	if asset == "" || strings.TrimSpace(amountRaw) == "" {
		return 0, "", false
	}
	price, source, ok := history.priceAt(asset, t)
	if !ok {
		switch {
		case isStableAsset(asset):
			price, source, ok = 1, priceSourceStablePeg, true
		case prices.AssetUSD[asset] > 0:
			price, source, ok = prices.AssetUSD[asset], priceSourceSpot, true
		}
	}
	if !ok {
		return 0, "", false
	}
	amount, okAmount := new(big.Float).SetString(strings.TrimSpace(amountRaw))
	if !okAmount {
		return 0, source, true
	}
	units, _ := new(big.Float).Quo(amount, big.NewFloat(1e8)).Float64()
	usd := units * price
	if math.IsNaN(usd) || math.IsInf(usd, 0) {
		// Absurd raw amounts (spam tokens) overflow; they have no meaningful value.
		return 0, "", false
	}
	return usd, source, true
}

// validPrice rejects zero, negative, NaN, and infinite prices.
func validPrice(usd float64) bool {
	return usd > 0 && !math.IsInf(usd, 0)
}

// pricePointCount is used by tests and diagnostics.
func pricePointCount(ctx context.Context, db *sql.DB, asset string) int {
	var n int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM price_points WHERE asset = ?`, asset).Scan(&n)
	return n
}
