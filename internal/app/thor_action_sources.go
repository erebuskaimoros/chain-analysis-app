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
	legacyActionPageDelay     = 250 * time.Millisecond
	mergedTHORActionCachePref = "__thor_merged__|"
)

func (a *App) lookupActionByTxID(ctx context.Context, txID string) (ActionLookupResult, error) {
	if a == nil {
		return ActionLookupResult{}, fmt.Errorf("app is required")
	}
	txID = strings.ToUpper(strings.TrimSpace(txID))
	if txID == "" {
		return ActionLookupResult{}, fmt.Errorf("txid is required")
	}

	var midgardResp midgardActionsResponse
	midgardPath := "/actions?txid=" + txID
	midgardErr := a.mid.GetJSON(ctx, midgardPath, &midgardResp)
	if midgardErr == nil && len(midgardResp.Actions) > 0 {
		return ActionLookupResult{
			TxID:    txID,
			Actions: canonicalizeMidgardLookupActions(midgardResp.Actions),
		}, nil
	}

	if !a.hasLegacyActionSource() {
		if midgardErr != nil {
			return ActionLookupResult{}, midgardErr
		}
		return ActionLookupResult{
			TxID:    txID,
			Actions: canonicalizeMidgardLookupActions(midgardResp.Actions),
		}, nil
	}

	var legacyResp midgardActionsResponse
	legacyPath := "/actions?txid=" + txID
	legacyErr := a.legacyActions.GetJSON(ctx, legacyPath, &legacyResp)
	if legacyErr != nil {
		if midgardErr != nil {
			return ActionLookupResult{}, midgardErr
		}
		return ActionLookupResult{}, legacyErr
	}

	return ActionLookupResult{
		TxID:    txID,
		Actions: canonicalizeMidgardLookupActions(mergeMidgardActions(midgardResp.Actions, legacyResp.Actions)),
	}, nil
}

func (a *App) fetchActionHistoryForAddress(ctx context.Context, address string, start, end time.Time, maxPages int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	if !a.shouldUseLegacyTHORActionHistory(seed) {
		return a.fetchMidgardActionsForAddressOnly(ctx, seed.Address, start, end, maxPages)
	}

	if maxPages < 1 {
		maxPages = 1
	}
	if maxPages > midgardMaxPagesPerAddress {
		maxPages = midgardMaxPagesPerAddress
	}

	fromTimestamp, endTimestamp := actionHistoryQueryBounds(start, end)
	cacheKey := mergedTHORActionCacheKey(seed.Address)
	if cached, truncated, found, err := lookupMidgardActionCache(ctx, a.db, cacheKey, fromTimestamp, endTimestamp, maxPages); err == nil && found {
		cached = canonicalizeMidgardLookupActions(cached)
		logInfo(ctx, "thor_action_cache_hit", map[string]any{
			"address":   seed.Address,
			"actions":   len(cached),
			"truncated": truncated,
			"max_pages": maxPages,
		})
		return cached, truncated, nil
	}

	midgardActions, midgardTruncated, err := a.fetchMidgardActionsForAddressOnly(ctx, seed.Address, start, end, maxPages)
	if err != nil {
		return midgardActions, false, err
	}
	legacyActions, legacyTruncated, err := a.fetchLegacyActionsForAddress(ctx, seed.Address, start, end, maxPages)
	if err != nil {
		return midgardActions, false, err
	}

	merged := mergeMidgardActions(midgardActions, legacyActions)
	truncated := midgardTruncated || legacyTruncated
	if err := insertMidgardActionCache(ctx, a.db, cacheKey, fromTimestamp, endTimestamp, maxPages, truncated, merged); err != nil {
		logError(ctx, "thor_action_cache_write_failed", err, map[string]any{"address": seed.Address})
	}
	return merged, truncated, nil
}

func (a *App) fetchActionHistoryForAddressPaged(ctx context.Context, address string, start, end time.Time, startPage, pageCount int) ([]midgardAction, bool, error) {
	seed := normalizeFrontierAddress(address)
	if seed.Address == "" {
		return nil, false, nil
	}
	if !a.shouldUseLegacyTHORActionHistory(seed) {
		return a.fetchMidgardActionsForAddressPagedOnly(ctx, seed.Address, start, end, startPage, pageCount)
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

	midgardActions, midgardTruncated, err := a.fetchMidgardActionsForAddressPagedOnly(ctx, seed.Address, start, end, 0, totalPages)
	if err != nil {
		return midgardActions, false, err
	}
	legacyActions, legacyTruncated, err := a.fetchLegacyActionsForAddress(ctx, seed.Address, start, end, totalPages)
	if err != nil {
		return midgardActions, false, err
	}

	merged := mergeMidgardActions(midgardActions, legacyActions)
	startIndex := startPage * midgardActionsPageLimit
	if startIndex >= len(merged) {
		return nil, midgardTruncated || legacyTruncated, nil
	}
	endIndex := (startPage + pageCount) * midgardActionsPageLimit
	if endIndex > len(merged) {
		endIndex = len(merged)
	}
	return append([]midgardAction(nil), merged[startIndex:endIndex]...), midgardTruncated || legacyTruncated || endIndex < len(merged), nil
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
	nextPageToken := ""

	for page := 0; page < pageCount; page++ {
		params := url.Values{}
		params.Set("address", address)
		params.Set("fromTimestamp", strconv.FormatInt(fromTimestamp, 10))
		params.Set("timestamp", strconv.FormatInt(endTimestamp, 10))
		params.Set("limit", strconv.Itoa(midgardActionsPageLimit))
		if nextPageToken != "" {
			params.Set("nextPageToken", nextPageToken)
		}

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
		nextPageToken = strings.TrimSpace(response.Meta.NextPageToken)
		if len(response.Actions) < midgardActionsPageLimit || nextPageToken == "" {
			return canonicalizeMidgardLookupActions(actions), false, nil
		}
		if page+1 < pageCount && !sleepWithContext(ctx, legacyActionPageDelay) {
			return actions, false, ctx.Err()
		}
	}

	return canonicalizeMidgardLookupActions(actions), true, nil
}

func (a *App) shouldUseLegacyTHORActionHistory(seed frontierAddress) bool {
	if a == nil || !a.hasLegacyActionSource() {
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
