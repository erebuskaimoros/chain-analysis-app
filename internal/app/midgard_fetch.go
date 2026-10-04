package app

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (a *App) ensureActorTrackerCoverage(ctx context.Context, addresses []string, start, end time.Time) (int64, bool, []string, map[string][]midgardAction, map[string]bool, error) {
	_, prefilterSatisfied, prefilterWarnings, _, actionCache, truncatedCache, prefilterErr := a.ensureMidgardAddressCoverage(ctx, addresses, start, end)
	if prefilterErr != nil {
		return 0, false, nil, nil, nil, prefilterErr
	}
	return 0, prefilterSatisfied, prefilterWarnings, actionCache, truncatedCache, nil
}

func (a *App) ensureMidgardAddressCoverage(ctx context.Context, addresses []string, start, end time.Time) (int64, bool, []string, bool, map[string][]midgardAction, map[string]bool, error) {
	normalized := normalizeAddressList(addresses)
	if len(normalized) == 0 {
		return 0, false, nil, false, nil, nil, nil
	}

	logInfo(ctx, "midgard_prefilter_started", map[string]any{
		"address_count": len(normalized),
		"start_time":    start.Format(time.RFC3339),
		"end_time":      end.Format(time.RFC3339),
	})

	var warnings []string
	coverageSatisfied := true
	usableCalls := 0
	actionCache := map[string][]midgardAction{}
	truncatedCache := map[string]bool{}
	progress := buildProgressFromContext(ctx)
	progress.set("fetching seed history", 0, len(normalized), "")

	for index, seed := range normalized {
		address := seed.Address
		progress.set("fetching seed history", index, len(normalized), shortAddress(address))
		actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, address, start, end, midgardMaxPagesPerAddress)
		if err != nil {
			coverageSatisfied = false
			warnings = append(warnings, fmt.Sprintf("midgard fetch failed for %s", shortAddress(address)))
			logError(ctx, "midgard_prefilter_address_failed", err, map[string]any{
				"address": address,
			})
			continue
		}
		actionCache[address] = actions
		truncatedCache[address] = truncated
		usableCalls++
		if truncated {
			coverageSatisfied = false
			warnings = append(warnings, fmt.Sprintf("midgard actions truncated for %s after %d pages", shortAddress(address), midgardMaxPagesPerAddress))
		}
	}

	if usableCalls == 0 {
		logInfo(ctx, "midgard_prefilter_unavailable", map[string]any{
			"address_count": len(normalized),
		})
		return 0, false, warnings, false, nil, nil, nil
	}

	logInfo(ctx, "midgard_prefilter_completed", map[string]any{
		"address_count":      len(normalized),
		"coverage_satisfied": coverageSatisfied,
		"warnings":           len(warnings),
	})
	return 0, coverageSatisfied, warnings, true, actionCache, truncatedCache, nil
}

func (a *App) prefetchMidgardBatch(
	ctx context.Context,
	wave []queueItem,
	start, end time.Time,
	hop int,
	cache map[string][]midgardAction,
	truncCache map[string]bool,
	fetchCount *int,
	rateLimited *bool,
	budget int,
	builder *graphBuilder,
) {
	var toFetch []queueItem
	for _, item := range wave {
		if _, ok := cache[item.Address]; ok {
			continue
		}
		if *fetchCount >= budget {
			break
		}
		toFetch = append(toFetch, item)
	}
	if len(toFetch) == 0 {
		return
	}

	progress := buildProgressFromContext(ctx)

	handleResult := func(address string, actions []midgardAction, truncated bool, err error) {
		if err != nil {
			builder.warnings = append(builder.warnings, fmt.Sprintf("midgard action flow fetch failed for %s", shortAddress(address)))
			logError(ctx, "midgard_graph_action_fetch_failed", err, map[string]any{
				"address": address,
				"hop":     hop,
			})
			if hop > 0 && isMidgardRateLimitError(err) {
				*rateLimited = true
			}
			cache[address] = nil
			truncCache[address] = false
		} else {
			cache[address] = actions
			truncCache[address] = truncated
		}
	}

	// Serialize hop>0 fetches to avoid Midgard 429 rate limiting.
	if hop > 0 {
		for _, item := range toFetch {
			if *fetchCount >= budget {
				break
			}
			*fetchCount++
			progress.set(fmt.Sprintf("expanding hop %d", hop), *fetchCount, budget, shortAddress(item.Address))
			maxPages := midgardGraphPagesForHop(item.Hop)
			actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, item.Address, start, end, maxPages)
			handleResult(item.Address, actions, truncated, err)
			if *rateLimited {
				break
			}
			time.Sleep(midgardActionPageDelay)
		}
		return
	}

	progress.set("fetching actor addresses", *fetchCount, budget, "")

	type fetchResult struct {
		address   string
		actions   []midgardAction
		truncated bool
		err       error
	}

	sem := make(chan struct{}, midgardConcurrentFetches)
	results := make(chan fetchResult, len(toFetch))
	var wg sync.WaitGroup
	for _, item := range toFetch {
		if *fetchCount >= budget {
			break
		}
		*fetchCount++
		wg.Add(1)
		go func(addr string, hopLevel int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			maxPages := midgardGraphPagesForHop(hopLevel)
			actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, addr, start, end, maxPages)
			results <- fetchResult{address: addr, actions: actions, truncated: truncated, err: err}
		}(item.Address, item.Hop)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		handleResult(r.address, r.actions, r.truncated, r.err)
	}
}

func (a *App) fetchMidgardActionsForAddress(ctx context.Context, address string, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	return a.fetchActionHistoryForAddress(ctx, address, start, end, maxPages)
}

func (a *App) fetchMidgardActionsForAddressOnly(ctx context.Context, address string, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	return a.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, sourceProtocolTHOR, address, start, end, maxPages)
}

func (a *App) fetchMidgardActionsForAddressOnlyFromProtocol(ctx context.Context, protocol, address string, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	engine, ok := a.liquidityEngine(protocol)
	if !ok || engine.MidgardClient == nil {
		return nil, false, fmt.Errorf("%s liquidity midgard unavailable", normalizeSourceProtocol(protocol))
	}
	address = seed.Address
	if maxPages < 1 {
		maxPages = 1
	}
	if maxPages > midgardMaxPagesPerAddress {
		maxPages = midgardMaxPagesPerAddress
	}

	fromTimestamp := start.Unix()
	if fromTimestamp < 0 {
		fromTimestamp = 0
	}
	endTimestamp := end.Unix()
	if endTimestamp < fromTimestamp {
		endTimestamp = fromTimestamp
	}
	source := ledgerActionSource(protocol)
	actions, truncated, err := a.fetchLedgerActions(ctx, source, address, fromTimestamp, endTimestamp, maxPages,
		func(from, to int64, pageBudget int) ([]midgardAction, bool, int, error) {
			return a.fetchMidgardActionWindow(ctx, engine.MidgardClient, protocol, address, from, to, pageBudget)
		})
	return annotateMidgardActions(canonicalizeMidgardLookupActions(actions), protocol), truncated, err
}

// fetchLedgerActions serves [from, to] for address from the ledger, fetching
// only the uncovered ranges (newest first, sharing maxPages across them).
func (a *App) fetchLedgerActions(
	ctx context.Context,
	source, address string,
	fromTimestamp, endTimestamp int64,
	maxPages int,
	fetchWindow func(from, to int64, pageBudget int) ([]midgardAction, bool, int, error),
) ([]midgardAction, bool, error) {
	covered, err := loadLedgerCoverage(ctx, a.db, source, address)
	if err != nil {
		return nil, false, err
	}
	gaps := ledgerGaps(covered, fromTimestamp, endTimestamp)
	if len(gaps) == 0 {
		actions, err := queryLedgerActions(ctx, a.db, source, address, fromTimestamp, endTimestamp)
		if err == nil {
			logInfo(ctx, "midgard_action_cache_hit", map[string]any{
				"source":  source,
				"address": address,
				"actions": len(actions),
			})
		}
		return actions, false, err
	}

	truncated := false
	budget := maxPages
	var fetchErr error
	for _, gap := range gaps {
		if budget <= 0 {
			truncated = true
			break
		}
		fetchedAt := time.Now().UTC()
		fetched, gapTruncated, pagesUsed, err := fetchWindow(gap.From, gap.To, budget)
		budget -= max(pagesUsed, 1)
		if storeErr := upsertLedgerActions(ctx, a.db, source, address, fetched); storeErr != nil {
			logError(ctx, "midgard_action_cache_write_failed", storeErr, map[string]any{"source": source, "address": address})
		}
		if err != nil {
			fetchErr = err
			break
		}
		coveredFrom := gap.From
		if gapTruncated {
			truncated = true
			coveredFrom = oldestMidgardActionUnix(fetched)
			if coveredFrom == 0 {
				continue
			}
		}
		if storeErr := markLedgerCovered(ctx, a.db, source, address, coveredFrom, ledgerCoverageEnd(gap.To, fetchedAt), fetchedAt); storeErr != nil {
			logError(ctx, "midgard_action_cache_write_failed", storeErr, map[string]any{"source": source, "address": address})
		}
	}
	actions, err := queryLedgerActions(ctx, a.db, source, address, fromTimestamp, endTimestamp)
	if err != nil {
		return nil, false, err
	}
	if fetchErr != nil {
		return actions, false, fetchErr
	}
	return actions, truncated, nil
}

func oldestMidgardActionUnix(actions []midgardAction) int64 {
	var oldest int64
	for _, action := range actions {
		ts := parseMidgardActionTime(action.Date).Unix()
		if ts <= 0 {
			continue
		}
		if oldest == 0 || ts < oldest {
			oldest = ts
		}
	}
	return oldest
}

// fetchMidgardActionWindow pages /actions for address within [from, to],
// newest first, up to maxPages. It reports whether the window was truncated
// and how many pages it used.
func (a *App) fetchMidgardActionWindow(ctx context.Context, client *ThorClient, protocol, address string, fromTimestamp, endTimestamp int64, maxPages int) ([]midgardAction, bool, int, error) {
	pagesUsed := 0
	actions := make([]midgardAction, 0, maxPages*midgardActionsPageLimit)
	for page := 0; page < maxPages; page++ {
		params := url.Values{}
		params.Set("address", address)
		params.Set("fromTimestamp", strconv.FormatInt(fromTimestamp, 10))
		params.Set("timestamp", strconv.FormatInt(endTimestamp, 10))
		params.Set("limit", strconv.Itoa(midgardActionsPageLimit))
		params.Set("offset", strconv.Itoa(page*midgardActionsPageLimit))

		var response midgardActionsResponse
		path := "/actions?" + params.Encode()
		offset := page * midgardActionsPageLimit
		if err := client.GetJSONObserved(ctx, path, &response, func(meta RequestAttemptMeta) {
			fields := map[string]any{
				"protocol":              normalizeSourceProtocol(protocol),
				"address":               address,
				"page":                  page,
				"offset":                offset,
				"limit":                 midgardActionsPageLimit,
				"path":                  meta.Path,
				"endpoint":              meta.Endpoint,
				"url":                   meta.URL,
				"attempt":               meta.Attempt,
				"status":                meta.StatusCode,
				"result":                meta.Result,
				"duration_ms":           meta.Duration.Milliseconds(),
				"will_retry":            meta.WillRetry,
				"retryable_status":      meta.RetryableStatus,
				"retry_after":           meta.RetryAfter,
				"x_ratelimit_limit":     meta.XRateLimitLimit,
				"x_ratelimit_remaining": meta.XRateLimitRemaining,
				"x_ratelimit_reset":     meta.XRateLimitReset,
				"ratelimit_limit":       meta.RateLimitLimit,
				"ratelimit_remaining":   meta.RateLimitRemaining,
				"ratelimit_reset":       meta.RateLimitReset,
				"cf_ray":                meta.CFRay,
				"cf_mitigated":          meta.CFMitigated,
			}
			if meta.Result == "success" {
				logInfo(ctx, "midgard_graph_action_call", fields)
				return
			}
			callErr := fmt.Errorf("midgard graph action result=%s", meta.Result)
			if strings.TrimSpace(meta.Error) != "" {
				callErr = fmt.Errorf("%s", strings.TrimSpace(meta.Error))
			}
			logError(ctx, "midgard_graph_action_call_failed", callErr, fields)
		}); err != nil {
			if isMidgardRateLimitError(err) {
				sleepWithContext(ctx, midgard429Cooldown)
			}
			return actions, false, pagesUsed, err
		}

		actions = append(actions, response.Actions...)
		pagesUsed = page + 1
		if len(response.Actions) < midgardActionsPageLimit {
			return actions, false, pagesUsed, nil
		}
		if page+1 < maxPages && !sleepWithContext(ctx, midgardActionPageDelay) {
			return actions, false, pagesUsed, ctx.Err()
		}
	}
	return actions, true, pagesUsed, nil
}

// fetchMidgardActionsForAddressPaged is like fetchMidgardActionsForAddress but
// starts at an arbitrary page offset instead of 0. It skips the disk cache
// because paged requests are not cache-aligned. It does not enforce the
// midgardMaxPagesPerAddress cap so callers must bound pageCount themselves.
func (a *App) fetchMidgardActionsForAddressPaged(ctx context.Context, address string, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	return a.fetchActionHistoryForAddressPaged(ctx, address, start, end, startPage, pageCount)
}

func (a *App) fetchMidgardActionsForAddressPagedOnly(ctx context.Context, address string, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	return a.fetchMidgardActionsForAddressPagedOnlyFromProtocol(ctx, sourceProtocolTHOR, address, start, end, startPage, pageCount)
}

func (a *App) fetchMidgardActionsForAddressPagedOnlyFromProtocol(ctx context.Context, protocol, address string, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	engine, ok := a.liquidityEngine(protocol)
	if !ok || engine.MidgardClient == nil {
		return nil, false, fmt.Errorf("%s liquidity midgard unavailable", normalizeSourceProtocol(protocol))
	}
	address = seed.Address
	if pageCount < 1 {
		pageCount = 1
	}

	fromTimestamp := start.Unix()
	if fromTimestamp < 0 {
		fromTimestamp = 0
	}
	endTimestamp := end.Unix()
	if endTimestamp < fromTimestamp {
		endTimestamp = fromTimestamp
	}

	actions := make([]midgardAction, 0, pageCount*midgardActionsPageLimit)
	for i := 0; i < pageCount; i++ {
		page := startPage + i
		params := url.Values{}
		params.Set("address", address)
		params.Set("fromTimestamp", strconv.FormatInt(fromTimestamp, 10))
		params.Set("timestamp", strconv.FormatInt(endTimestamp, 10))
		params.Set("limit", strconv.Itoa(midgardActionsPageLimit))
		params.Set("offset", strconv.Itoa(page*midgardActionsPageLimit))

		var response midgardActionsResponse
		path := "/actions?" + params.Encode()
		offset := page * midgardActionsPageLimit
		if err := engine.MidgardClient.GetJSONObserved(ctx, path, &response, func(meta RequestAttemptMeta) {
			fields := map[string]any{
				"protocol":              normalizeSourceProtocol(protocol),
				"address":               address,
				"page":                  page,
				"offset":                offset,
				"limit":                 midgardActionsPageLimit,
				"path":                  meta.Path,
				"endpoint":              meta.Endpoint,
				"url":                   meta.URL,
				"attempt":               meta.Attempt,
				"status":                meta.StatusCode,
				"result":                meta.Result,
				"duration_ms":           meta.Duration.Milliseconds(),
				"will_retry":            meta.WillRetry,
				"retryable_status":      meta.RetryableStatus,
				"retry_after":           meta.RetryAfter,
				"x_ratelimit_limit":     meta.XRateLimitLimit,
				"x_ratelimit_remaining": meta.XRateLimitRemaining,
				"x_ratelimit_reset":     meta.XRateLimitReset,
				"ratelimit_limit":       meta.RateLimitLimit,
				"ratelimit_remaining":   meta.RateLimitRemaining,
				"ratelimit_reset":       meta.RateLimitReset,
				"cf_ray":                meta.CFRay,
				"cf_mitigated":          meta.CFMitigated,
			}
			if meta.Result == "success" {
				logInfo(ctx, "midgard_explorer_paged_call", fields)
				return
			}
			callErr := fmt.Errorf("midgard explorer paged result=%s", meta.Result)
			if strings.TrimSpace(meta.Error) != "" {
				callErr = fmt.Errorf("%s", strings.TrimSpace(meta.Error))
			}
			logError(ctx, "midgard_explorer_paged_call_failed", callErr, fields)
		}); err != nil {
			if isMidgardRateLimitError(err) {
				sleepWithContext(ctx, midgard429Cooldown)
			}
			return actions, false, err
		}

		actions = append(actions, response.Actions...)
		if len(response.Actions) < midgardActionsPageLimit {
			return annotateMidgardActions(canonicalizeMidgardLookupActions(actions), protocol), false, nil
		}
		if i+1 < pageCount && !sleepWithContext(ctx, midgardActionPageDelay) {
			return actions, false, ctx.Err()
		}
	}

	return annotateMidgardActions(canonicalizeMidgardLookupActions(actions), protocol), true, nil
}

// probeMidgardTotalPages does a binary search to find the last page with data
// for an address. Returns the total number of pages (0-indexed last page + 1).
func (a *App) probeMidgardTotalPages(ctx context.Context, address string, start, end time.Time) (int, error) {
	seed := normalizeFrontierAddress(address)
	protocols := a.actionSourceProtocolsForSeed(seed)
	if len(protocols) == 0 {
		return 0, nil
	}
	return a.probeMidgardTotalPagesForProtocol(ctx, protocols[0], seed.Address, start, end)
}

func (a *App) probeMidgardTotalPagesForProtocol(ctx context.Context, protocol, address string, start, end time.Time) (int, error) {
	// Exponential probe to find upper bound.
	probe := 10 // start at page 10 (offset 500)
	for {
		seed := normalizeFrontierAddress(address)
		actions, _, err := a.fetchActionHistoryForAddressPagedFromProtocol(ctx, protocol, seed, start, end, probe, 1)
		if err != nil {
			return 0, err
		}
		if len(actions) == 0 {
			break
		}
		probe *= 2
		if probe > 20000 {
			// Safety cap at 1M actions.
			break
		}
	}

	// Binary search between probe/2 and probe.
	lo, hi := probe/2, probe
	for lo < hi {
		mid := (lo + hi) / 2
		seed := normalizeFrontierAddress(address)
		actions, _, err := a.fetchActionHistoryForAddressPagedFromProtocol(ctx, protocol, seed, start, end, mid, 1)
		if err != nil {
			return 0, err
		}
		if len(actions) > 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

func filterMidgardActionsByTimeRange(actions []midgardAction, startTS, endTS int64) []midgardAction {
	out := make([]midgardAction, 0, len(actions))
	for _, a := range actions {
		ts := parseInt64(a.Date)
		if ts <= 0 {
			out = append(out, a)
			continue
		}
		sec := ts / 1_000_000_000
		if sec >= startTS && sec <= endTS {
			out = append(out, a)
		}
	}
	return out
}

func filterExternalTransfersByTimeRange(transfers []externalTransfer, startTS, endTS int64) []externalTransfer {
	out := make([]externalTransfer, 0, len(transfers))
	for _, transfer := range transfers {
		if transfer.Time.IsZero() {
			out = append(out, transfer)
			continue
		}
		sec := transfer.Time.Unix()
		if sec >= startTS && sec <= endTS {
			out = append(out, transfer)
		}
	}
	return out
}

func (a *App) hydrateBondMemoNodeCache(_ context.Context, actions []midgardAction, cache map[string]string) []string {
	if len(actions) == 0 || cache == nil {
		return nil
	}
	for _, action := range actions {
		if midgardActionClass(action) != "bonds" {
			continue
		}
		// Read node address directly from Midgard metadata.
		nodeAddress := ""
		switch {
		case action.Metadata.Rebond != nil:
			nodeAddress = normalizeAddress(action.Metadata.Rebond.NodeAddress)
			if nodeAddress == "" {
				nodeAddress = parseBondMemoNodeAddress(action.Metadata.Rebond.Memo)
			}
		case action.Metadata.Bond != nil:
			nodeAddress = normalizeAddress(action.Metadata.Bond.NodeAddress)
			if nodeAddress == "" {
				nodeAddress = parseBondMemoNodeAddress(action.Metadata.Bond.Memo)
			}
		}
		for _, txID := range midgardActionTxIDs(action) {
			txID = cleanTxID(txID)
			if txID == "" {
				continue
			}
			if _, ok := cache[txID]; ok {
				continue
			}
			cache[txID] = normalizeAddress(nodeAddress)
		}
	}
	return nil
}

func compactHeightRanges(heights []int64, padding, mergeGap int64) []heightRange {
	if len(heights) == 0 {
		return nil
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })

	makeRange := func(height int64) heightRange {
		start := height - padding
		if start < 1 {
			start = 1
		}
		end := height + padding
		return heightRange{Start: start, End: end}
	}

	current := makeRange(heights[0])
	out := make([]heightRange, 0, len(heights))
	for _, height := range heights[1:] {
		next := makeRange(height)
		if next.Start <= current.End+mergeGap {
			if next.End > current.End {
				current.End = next.End
			}
			continue
		}
		out = append(out, current)
		current = next
	}
	out = append(out, current)
	return out
}

func midgardGraphPagesForHop(hop int) int {
	if hop <= 0 {
		return midgardGraphPagesPerSeed
	}
	if hop == 1 {
		return midgardGraphPagesPerFirstHop
	}
	return midgardGraphPagesPerHop
}

func isMidgardRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	if text == "" {
		return false
	}
	return strings.Contains(text, "status=429") ||
		strings.Contains(text, "too many requests") ||
		strings.Contains(text, "slow down cowboy")
}
