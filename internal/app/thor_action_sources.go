package app

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	legacyActionPageDelay = 250 * time.Millisecond
)

func (a *App) lookupActionByTxID(ctx context.Context, txID string) (ActionLookupResult, error) {
	if a == nil {
		return ActionLookupResult{}, fmt.Errorf("app is required")
	}
	txID = strings.ToUpper(strings.TrimSpace(txID))
	if txID == "" {
		return ActionLookupResult{}, fmt.Errorf("txid is required")
	}

	var (
		rows     []ActionLookupAction
		firstErr error
	)
	for _, engine := range a.availableLiquidityEngines() {
		actions, err := a.lookupActionByTxIDFromProtocol(ctx, engine.Protocol, txID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, action := range actions {
			rows = append(rows, ActionLookupAction{
				midgardAction:  action,
				SourceProtocol: sourceProtocolFromAction(action),
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left := parseMidgardActionTime(rows[i].Date)
		right := parseMidgardActionTime(rows[j].Date)
		if !left.Equal(right) {
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.After(right)
		}
		return parseInt64(rows[i].Height) > parseInt64(rows[j].Height)
	})
	if len(rows) == 0 && firstErr != nil {
		return ActionLookupResult{}, firstErr
	}
	return ActionLookupResult{
		TxID:    txID,
		Actions: rows,
	}, nil
}

func (a *App) fetchActionHistoryForAddress(ctx context.Context, address string, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	protocols := a.actionSourceProtocolsForSeed(seed)
	if len(protocols) == 0 {
		return nil, false, nil
	}
	var (
		groups    [][]midgardAction
		truncated bool
		firstErr  error
		hadError  bool
	)
	for _, protocol := range protocols {
		actions, protocolTruncated, err := a.fetchActionHistoryForAddressFromProtocol(ctx, protocol, seed, start, end, maxPages)
		if err != nil {
			hadError = true
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		groups = append(groups, actions)
		truncated = truncated || protocolTruncated
	}
	merged := mergeSourcedMidgardActions(groups...)
	if len(merged) > 0 {
		if hadError {
			truncated = true
		}
		return merged, truncated, nil
	}
	if firstErr != nil {
		return nil, false, firstErr
	}
	return nil, false, nil
}

func (a *App) fetchActionHistoryForAddressPaged(ctx context.Context, address string, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	protocols := a.actionSourceProtocolsForSeed(seed)
	if len(protocols) == 0 {
		return nil, false, nil
	}
	var (
		groups    [][]midgardAction
		truncated bool
		firstErr  error
		hadError  bool
	)
	for _, protocol := range protocols {
		actions, protocolTruncated, err := a.fetchActionHistoryForAddressPagedFromProtocol(ctx, protocol, seed, start, end, startPage, pageCount)
		if err != nil {
			hadError = true
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		groups = append(groups, actions)
		truncated = truncated || protocolTruncated
	}
	merged := mergeSourcedMidgardActions(groups...)
	if len(merged) > 0 {
		if hadError {
			truncated = true
		}
		return merged, truncated, nil
	}
	if firstErr != nil {
		return nil, false, firstErr
	}
	return nil, false, nil
}

func (a *App) lookupActionByTxIDFromProtocol(ctx context.Context, protocol, txID string) ([]midgardAction, error) {
	engine, ok := a.liquidityEngine(protocol)
	if !ok || engine.MidgardClient == nil {
		return nil, fmt.Errorf("%s liquidity engine unavailable", normalizeSourceProtocol(protocol))
	}

	var midgardResp midgardActionsResponse
	midgardPath := "/actions?txid=" + txID
	midgardErr := engine.MidgardClient.GetJSON(ctx, midgardPath, &midgardResp)
	midgardActions := annotateMidgardActions(canonicalizeMidgardLookupActions(midgardResp.Actions), protocol)
	if normalizeSourceProtocol(protocol) != sourceProtocolTHOR || engine.LegacyActionClient == nil {
		if midgardErr != nil {
			return nil, midgardErr
		}
		return midgardActions, nil
	}

	var legacyResp midgardActionsResponse
	legacyPath := "/actions?txid=" + txID
	legacyErr := engine.LegacyActionClient.GetJSON(ctx, legacyPath, &legacyResp)
	legacyActions := annotateMidgardActions(canonicalizeMidgardLookupActions(legacyResp.Actions), protocol)
	switch {
	case midgardErr == nil && legacyErr == nil:
		return annotateMidgardActions(canonicalizeMidgardLookupActions(mergeMidgardActions(midgardResp.Actions, legacyResp.Actions)), protocol), nil
	case midgardErr == nil:
		return midgardActions, nil
	case legacyErr == nil:
		return legacyActions, nil
	default:
		return nil, midgardErr
	}
}

func (a *App) fetchActionHistoryForAddressFromProtocol(ctx context.Context, protocol string, seed frontierAddress, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	if a.shouldUseLegacyTHORActionHistory(protocol, seed) {
		if maxPages < 1 {
			maxPages = 1
		}
		if maxPages > midgardMaxPagesPerAddress {
			maxPages = midgardMaxPagesPerAddress
		}

		fromTimestamp, endTimestamp := actionHistoryQueryBounds(start, end)
		cacheKey := mergedTHORActionCacheKey(seed.Address)
		if cached, truncated, found, err := lookupMidgardActionCache(ctx, a.db, cacheKey, fromTimestamp, endTimestamp, maxPages); err == nil && found {
			cached = annotateMidgardActions(canonicalizeMidgardLookupActions(cached), protocol)
			logInfo(ctx, "thor_action_cache_hit", map[string]any{
				"address":   seed.Address,
				"actions":   len(cached),
				"truncated": truncated,
				"max_pages": maxPages,
			})
			return cached, truncated, nil
		}

		midgardActions, midgardTruncated, err := a.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, protocol, seed.Address, start, end, maxPages)
		midgardErr := err
		legacyActions, legacyTruncated, err := a.fetchLegacyActionsForAddress(ctx, seed.Address, start, end, maxPages)
		legacyErr := err

		merged, truncated, cacheable, err := resolveTHORActionHistorySources(
			midgardActions,
			midgardTruncated,
			midgardErr,
			legacyActions,
			legacyTruncated,
			legacyErr,
		)
		if err != nil {
			return merged, false, err
		}
		merged = annotateMidgardActions(merged, protocol)
		if cacheable {
			if err := insertMidgardActionCache(ctx, a.db, cacheKey, fromTimestamp, endTimestamp, maxPages, truncated, merged); err != nil {
				logError(ctx, "thor_action_cache_write_failed", err, map[string]any{"address": seed.Address})
			}
		}
		return merged, truncated, nil
	}
	return a.fetchMidgardActionsForAddressOnlyFromProtocol(ctx, protocol, seed.Address, start, end, maxPages)
}

func (a *App) fetchActionHistoryForAddressPagedFromProtocol(ctx context.Context, protocol string, seed frontierAddress, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	if !a.shouldUseLegacyTHORActionHistory(protocol, seed) {
		return a.fetchMidgardActionsForAddressPagedOnlyFromProtocol(ctx, protocol, seed.Address, start, end, startPage, pageCount)
	}

	if startPage < 0 {
		startPage = 0
	}
	if pageCount < 1 {
		pageCount = 1
	}

	totalPages := startPage + pageCount
	if totalPages < 1 {
		totalPages = 1
	}

	midgardActions, midgardTruncated, err := a.fetchMidgardActionsForAddressPagedOnlyFromProtocol(ctx, protocol, seed.Address, start, end, 0, totalPages)
	midgardErr := err
	legacyActions, legacyTruncated, err := a.fetchLegacyActionsForAddress(ctx, seed.Address, start, end, totalPages)
	legacyErr := err

	merged, truncated, _, err := resolveTHORActionHistorySources(
		midgardActions,
		midgardTruncated,
		midgardErr,
		legacyActions,
		legacyTruncated,
		legacyErr,
	)
	if err != nil {
		return merged, false, err
	}
	merged = annotateMidgardActions(merged, protocol)

	startIndex := startPage * midgardActionsPageLimit
	if startIndex >= len(merged) {
		return nil, truncated, nil
	}
	endIndex := (startPage + pageCount) * midgardActionsPageLimit
	if endIndex > len(merged) {
		endIndex = len(merged)
	}
	return append([]midgardAction(nil), merged[startIndex:endIndex]...), truncated || endIndex < len(merged), nil
}

func resolveTHORActionHistorySources(
	midgardActions []midgardAction,
	midgardTruncated bool,
	midgardErr error,
	legacyActions []midgardAction,
	legacyTruncated bool,
	legacyErr error,
) ([]midgardAction, bool, bool, error) {
	midgardActions = canonicalizePartialMidgardActions(midgardActions)
	legacyActions = canonicalizePartialMidgardActions(legacyActions)

	merged := mergeMidgardActions(midgardActions, legacyActions)
	if midgardErr == nil && legacyErr == nil {
		return merged, midgardTruncated || legacyTruncated, true, nil
	}

	// THOR history merge is best-effort. If either source succeeded or yielded
	// partial data, keep the usable subset and mark the result truncated so
	// callers can warn without dropping the entire expansion/load flow.
	if midgardErr == nil || legacyErr == nil || len(merged) > 0 {
		return merged, true, false, nil
	}

	if midgardErr != nil {
		return nil, false, false, midgardErr
	}
	return nil, false, false, legacyErr
}

func canonicalizePartialMidgardActions(actions []midgardAction) []midgardAction {
	if len(actions) == 0 {
		return nil
	}
	return canonicalizeMidgardLookupActions(actions)
}

func (a *App) fetchLegacyActionsForAddress(ctx context.Context, address string, start, end time.Time, pageCount int) ([]midgardAction, bool, error) {
	if a == nil || !a.hasLegacyActionSource() {
		return nil, false, nil
	}
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	address = seed.Address
	if pageCount < 1 {
		pageCount = 1
	}

	fromTimestamp, endTimestamp := actionHistoryQueryBounds(start, end)
	actions := make([]midgardAction, 0, pageCount*midgardActionsPageLimit)

	for page := 0; page < pageCount; page++ {
		params := legacyActionQueryParams(address, fromTimestamp, endTimestamp, page)

		var response midgardActionsResponse
		path := "/actions?" + params.Encode()
		if err := a.legacyActions.GetJSONObserved(ctx, path, &response, func(meta RequestAttemptMeta) {
			fields := map[string]any{
				"address":               address,
				"page":                  page,
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
				logInfo(ctx, "legacy_thor_action_call", fields)
				return
			}
			callErr := fmt.Errorf("legacy THOR action result=%s", meta.Result)
			if strings.TrimSpace(meta.Error) != "" {
				callErr = fmt.Errorf("%s", strings.TrimSpace(meta.Error))
			}
			logError(ctx, "legacy_thor_action_call_failed", callErr, fields)
		}); err != nil {
			if isMidgardRateLimitError(err) {
				sleepWithContext(ctx, midgard429Cooldown)
			}
			return actions, false, err
		}

		actions = append(actions, response.Actions...)
		if len(response.Actions) < midgardActionsPageLimit {
			return canonicalizeMidgardLookupActions(actions), false, nil
		}
		if page+1 < pageCount && !sleepWithContext(ctx, legacyActionPageDelay) {
			return actions, false, ctx.Err()
		}
	}

	return canonicalizeMidgardLookupActions(actions), true, nil
}

func legacyActionQueryParams(address string, fromTimestamp, endTimestamp int64, page int) url.Values {
	params := url.Values{}
	params.Set("address", address)
	params.Set("fromTimestamp", strconv.FormatInt(fromTimestamp, 10))
	params.Set("timestamp", strconv.FormatInt(endTimestamp, 10))
	params.Set("limit", strconv.Itoa(midgardActionsPageLimit))
	// Vanaheim cursor pagination is unstable for follow-up pages; offset paging
	// remains fast for the same bounded queries.
	params.Set("offset", strconv.Itoa(page*midgardActionsPageLimit))
	return params
}

func (a *App) shouldUseLegacyTHORActionHistory(protocol string, seed frontierAddress) bool {
	if a == nil || !a.hasLegacyActionSource() || normalizeSourceProtocol(protocol) != sourceProtocolTHOR {
		return false
	}
	return normalizeChain(seed.Chain, seed.Address) == "THOR"
}

func (a *App) hasLegacyActionSource() bool {
	return a != nil && a.legacyActions != nil && len(a.cfg.LegacyActionEndpoints) > 0
}

func actionHistoryQueryBounds(start, end time.Time) (int64, int64) {
	fromTimestamp := start.Unix()
	if fromTimestamp < 0 {
		fromTimestamp = 0
	}
	endTimestamp := end.Unix()
	if endTimestamp < fromTimestamp {
		endTimestamp = fromTimestamp
	}
	return fromTimestamp, endTimestamp
}

func mergedTHORActionCacheKey(address string) string {
	address = normalizeAddress(address)
	if address == "" {
		return ""
	}
	return mergedTHORActionCachePref + address
}

func mergeMidgardActions(primary, secondary []midgardAction) []midgardAction {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}

	out := make([]midgardAction, 0, len(primary)+len(secondary))
	seen := map[string]struct{}{}
	appendUnique := func(actions []midgardAction) {
		for _, action := range actions {
			key := midgardActionKey(action)
			if key == "" {
				key = midgardSyntheticTxID(action)
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, action)
		}
	}

	appendUnique(primary)
	appendUnique(secondary)
	sortMidgardActionsNewestFirst(out)
	return out
}

func sortMidgardActionsNewestFirst(actions []midgardAction) {
	sort.SliceStable(actions, func(i, j int) bool {
		leftTime := parseMidgardActionTime(actions[i].Date)
		rightTime := parseMidgardActionTime(actions[j].Date)
		if !leftTime.Equal(rightTime) {
			if leftTime.IsZero() {
				return false
			}
			if rightTime.IsZero() {
				return true
			}
			return leftTime.After(rightTime)
		}

		leftHeight := parseInt64(actions[i].Height)
		rightHeight := parseInt64(actions[j].Height)
		if leftHeight != rightHeight {
			return leftHeight > rightHeight
		}

		return midgardActionKey(actions[i]) > midgardActionKey(actions[j])
	})
}
