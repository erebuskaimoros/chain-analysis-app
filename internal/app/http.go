package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	a.RegisterLegacyStaticRoutes(mux)
	handle := func(pattern string, fn http.HandlerFunc) {
		mux.HandleFunc(pattern, a.withRequestLoggingFunc(fn))
	}
	handle("GET /{$}", a.handleIndex)
	a.RegisterLegacyAPIRoutes(mux)
}

func (a *App) RegisterLegacyStaticRoutes(mux *http.ServeMux) {
	if stat, err := os.Stat(a.cfg.StaticDir); err == nil && stat.IsDir() {
		static := http.StripPrefix("/static/", http.FileServer(http.Dir(a.cfg.StaticDir)))
		mux.Handle("/static/", a.withRequestLogging(static))
	}
}

func (a *App) RegisterLegacyAPIRoutes(mux *http.ServeMux) {
	handle := func(pattern string, fn http.HandlerFunc) {
		mux.HandleFunc(pattern, a.withRequestLoggingFunc(fn))
	}
	handle("GET /api/health", a.handleHealth)
	handle("GET /api/actions/{txid}", a.handleActionByTxID)
	handle("GET /api/wallets/{address}/liquidity", a.handleWalletLiquidity)
	handle("GET /api/wallets/{address}/bonds", a.handleWalletBonds)
	handle("GET /api/rebond/{address}", a.handleRebondTrace)
	handle("GET /api/actors", a.handleActors)
	handle("POST /api/actors", a.handleActors)
	handle("PUT /api/actors/{id}", a.handleActorByID)
	handle("DELETE /api/actors/{id}", a.handleActorByID)
	handle("POST /api/address-explorer/graph", a.handleAddressExplorerGraph)
	handle("GET /api/address-explorer/runs", a.handleAddressExplorerRuns)
	handle("DELETE /api/address-explorer/runs/{id}", a.handleAddressExplorerRunDelete)
	handle("POST /api/actor-tracker/graph", a.handleActorTrackerGraph)
	handle("POST /api/actor-tracker/expand", a.handleActorTrackerExpand)
	handle("POST /api/actor-tracker/live-holdings", a.handleActorTrackerLiveHoldings)
	handle("GET /api/actor-tracker/runs", a.handleGraphRuns)
	handle("DELETE /api/actor-tracker/runs/{id}", a.handleGraphRunDelete)
	handle("GET /api/address-annotations", a.handleAddressAnnotations)
	handle("PUT /api/address-annotations", a.handleAddressAnnotations)
	handle("DELETE /api/address-annotations", a.handleAddressAnnotations)
	handle("GET /api/address-blocklist", a.handleAddressBlocklist)
	handle("POST /api/address-blocklist", a.handleAddressBlocklist)
	handle("DELETE /api/address-blocklist/{address}", a.handleAddressBlocklistDelete)
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, a.cfg.StaticDir+"/index.html")
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().UTC().Format(time.RFC3339),
		"build": map[string]any{
			"version":    a.cfg.BuildVersion,
			"commit":     a.cfg.BuildCommit,
			"build_time": a.cfg.BuildTime,
		},
		"thornode_sources":      a.cfg.ThornodeEndpoints,
		"midgard_sources":       a.cfg.MidgardEndpoints,
		"legacy_action_sources": a.cfg.LegacyActionEndpoints,
		"tracker_providers":     a.cfg.ChainTrackerProviders,
		"tracker_overrides":     a.cfg.ChainTrackerOverrides,
		"tracker_candidates":    a.cfg.ChainTrackerCandidates,
		"tracker_health":        a.trackerHealth.snapshot(),
		"tracker_sources": map[string]any{
			"utxo":                  a.cfg.UtxoTrackerURLs,
			"utxo_expanded":         expandChainURLMap(a.cfg.UtxoTrackerURLs),
			"cosmos":                a.cfg.CosmosTrackerURLs,
			"cosmos_expanded":       expandChainURLMap(a.cfg.CosmosTrackerURLs),
			"etherscan":             a.cfg.EtherscanAPIURL,
			"etherscan_expanded":    a.cfg.etherscanAPIURLs(),
			"blockscout_urls":       a.cfg.BlockscoutAPIURLs,
			"blockscout_expanded":   expandChainURLMap(a.cfg.BlockscoutAPIURLs),
			"avacloud_base":         a.cfg.AvaCloudBaseURL,
			"avacloud_expanded":     a.cfg.avaCloudBaseURLs(),
			"nodereal_bsc":          a.cfg.NodeRealBSCURL,
			"nodereal_bsc_expanded": a.cfg.nodeRealBSCURLs(),
			"solana_rpc":            a.cfg.SolanaRPCURL,
			"solana_rpc_expanded":   a.cfg.solanaRPCURLs(),
			"trongrid":              a.cfg.TronGridURL,
			"trongrid_expanded":     a.cfg.tronGridURLs(),
			"xrp_rpc":               a.cfg.XRPRPCURL,
			"xrp_rpc_expanded":      a.cfg.xrplRPCURLs(),
		},
	})
}

func (a *App) handleActionByTxID(w http.ResponseWriter, r *http.Request) {
	txID := strings.ToUpper(strings.TrimSpace(r.PathValue("txid")))
	if txID == "" {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("txid is required"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	result, err := a.lookupActionByTxID(ctx, txID)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func canonicalizeMidgardLookupActions(actions []midgardAction) []midgardAction {
	if len(actions) < 2 {
		return actions
	}
	out := make([]midgardAction, 0, len(actions))
	for i, action := range actions {
		if isMidgardShadowSendActionForLookup(i, actions) {
			continue
		}
		out = append(out, action)
	}
	return out
}

func isMidgardShadowSendActionForLookup(index int, actions []midgardAction) bool {
	if index < 0 || index >= len(actions) {
		return false
	}
	candidate := actions[index]
	if strings.ToLower(strings.TrimSpace(candidate.Type)) != "send" {
		return false
	}
	candidateOut := midgardLookupLegSignatures(candidate.Out)
	if len(candidateOut) == 0 {
		return false
	}
	candidateIn := midgardLookupLegSignatures(candidate.In)
	for i, other := range actions {
		if i == index {
			continue
		}
		if midgardActionClass(other) == "transfers" {
			continue
		}
		if strings.TrimSpace(candidate.Height) != "" && strings.TrimSpace(other.Height) != "" && candidate.Height != other.Height {
			continue
		}
		if strings.TrimSpace(candidate.Date) != "" && strings.TrimSpace(other.Date) != "" && candidate.Date != other.Date {
			continue
		}
		if !hasMidgardLookupLegIntersection(candidateOut, midgardLookupLegSignatures(other.Out)) {
			continue
		}
		otherIn := midgardLookupLegSignatures(other.In)
		if len(candidateIn) > 0 && len(otherIn) > 0 && !hasMidgardLookupLegIntersection(candidateIn, otherIn) {
			continue
		}
		return true
	}
	return false
}

func midgardLookupLegSignatures(legs []midgardActionLeg) map[string]struct{} {
	if len(legs) == 0 {
		return nil
	}
	out := map[string]struct{}{}
	for _, leg := range legs {
		sig := midgardLookupLegSignature(leg)
		if sig == "" {
			continue
		}
		out[sig] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func midgardLookupLegSignature(leg midgardActionLeg) string {
	address := normalizeAddress(leg.Address)
	txID := cleanTxID(leg.TxID)
	coinSigs := make([]string, 0, len(leg.Coins))
	for _, coin := range leg.Coins {
		asset := normalizeAsset(coin.Asset)
		amount := strings.TrimSpace(coin.Amount)
		if asset == "" && amount == "" {
			continue
		}
		coinSigs = append(coinSigs, asset+":"+amount)
	}
	sort.Strings(coinSigs)
	if address == "" && txID == "" && len(coinSigs) == 0 {
		return ""
	}
	return strings.Join([]string{address, txID, strings.Join(coinSigs, ",")}, "|")
}

func hasMidgardLookupLegIntersection(left, right map[string]struct{}) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	for key := range left {
		if _, ok := right[key]; ok {
			return true
		}
	}
	return false
}

func (a *App) handleWalletLiquidity(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.PathValue("address"))
	if address == "" {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("address is required"))
		return
	}

	limit := intQuery(r, "limit", 300)
	if limit > 2000 {
		limit = 2000
	}
	maxPages := intQuery(r, "max_pages", midgardMaxPagesPerAddress)
	if maxPages > midgardMaxPagesPerAddress {
		maxPages = midgardMaxPagesPerAddress
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.MidgardTimeout*time.Duration(max(3, maxPages)))
	defer cancel()

	end := time.Now().UTC()
	start := end.Add(-365 * 24 * time.Hour)
	actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, address, start, end, maxPages)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err)
		return
	}

	liquidityTypes := map[string]bool{
		"addLiquidity": true, "withdraw": true, "swap": true, "refund": true,
		"runePoolDeposit": true, "runePoolWithdraw": true,
	}
	var filtered []midgardAction
	for _, action := range actions {
		if liquidityTypes[action.Type] {
			filtered = append(filtered, action)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	summary := summarizeMidgardLiquidity(address, filtered)
	writeJSON(w, http.StatusOK, map[string]any{
		"address":   address,
		"truncated": truncated,
		"summary":   summary,
		"actions":   filtered,
	})
}

func (a *App) handleWalletBonds(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.PathValue("address"))
	if address == "" {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("address is required"))
		return
	}
	limit := intQuery(r, "limit", 300)
	if limit > 2000 {
		limit = 2000
	}
	maxPages := intQuery(r, "max_pages", midgardMaxPagesPerAddress)
	if maxPages > midgardMaxPagesPerAddress {
		maxPages = midgardMaxPagesPerAddress
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.MidgardTimeout*time.Duration(max(3, maxPages)))
	defer cancel()

	end := time.Now().UTC()
	start := end.Add(-365 * 24 * time.Hour)
	actions, truncated, err := a.fetchMidgardActionsForAddress(ctx, address, start, end, maxPages)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err)
		return
	}

	// Extract rebond links from bond/rebond Midgard metadata.
	a.extractRebondLinksFromMidgardBondActions(ctx, actions)

	bondTypes := map[string]bool{
		"bond": true, "rebond": true, "unbond": true, "refund": true,
	}
	var filtered []midgardAction
	for _, action := range actions {
		if bondTypes[strings.ToLower(strings.TrimSpace(action.Type))] {
			filtered = append(filtered, action)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	links, err := queryRebondLinksByAddress(ctx, a.db, address, 300)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, err)
		return
	}

	continuity := buildRebondContinuity(address, links)

	writeJSON(w, http.StatusOK, map[string]any{
		"address":      address,
		"truncated":    truncated,
		"actions":      filtered,
		"rebond_links": links,
		"continuity":   continuity,
	})
}

func (a *App) handleRebondTrace(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.PathValue("address"))
	if address == "" {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("address is required"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.MidgardTimeout*time.Duration(max(3, midgardMaxPagesPerAddress)))
	defer cancel()

	// Fetch bond actions from Midgard and populate rebond_links on-demand.
	end := time.Now().UTC()
	start := end.Add(-365 * 24 * time.Hour)
	actions, _, err := a.fetchMidgardActionsForAddress(ctx, address, start, end, midgardMaxPagesPerAddress)
	if err != nil {
		writeError(r, w, http.StatusBadGateway, err)
		return
	}
	a.extractRebondLinksFromMidgardBondActions(ctx, actions)

	links, err := queryRebondLinksByAddress(ctx, a.db, address, 500)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, err)
		return
	}
	if len(links) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"start_address": address,
			"addresses":     []string{address},
			"links":         []RebondLink{},
		})
		return
	}

	connectedAddrs := map[string]struct{}{address: {}}
	for _, l := range links {
		connectedAddrs[l.OldBondAddress] = struct{}{}
		connectedAddrs[l.NewBondAddress] = struct{}{}
	}
	addrList := mapKeys(connectedAddrs)

	expanded, err := queryRebondLinksForAddressSet(ctx, a.db, addrList, 1000)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, buildRebondContinuity(address, expanded))
}

func (a *App) handleActors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
		defer cancel()

		actors, err := listActors(ctx, a.db)
		if err != nil {
			writeError(r, w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"actors": actors})
	case http.MethodPost:
		var req ActorUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(r, w, http.StatusBadRequest, err)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
		defer cancel()

		actor, err := upsertActor(ctx, a.db, 0, req)
		if err != nil {
			writeError(r, w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, actor)
	default:
		writeError(r, w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (a *App) handleActorByID(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(strings.TrimSpace(r.PathValue("id")))
	if id <= 0 {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("actor id is required"))
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req ActorUpsertRequest
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(r, w, http.StatusBadRequest, err)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
		defer cancel()

		actor, err := upsertActor(ctx, a.db, id, req)
		if err != nil {
			status := http.StatusBadRequest
			if err == sql.ErrNoRows {
				status = http.StatusNotFound
			}
			writeError(r, w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, actor)
	case http.MethodDelete:
		ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
		defer cancel()
		if err := deleteActor(ctx, a.db, id); err != nil {
			status := http.StatusInternalServerError
			if err == sql.ErrNoRows {
				status = http.StatusNotFound
			}
			writeError(r, w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
	default:
		writeError(r, w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (a *App) handleAddressExplorerGraph(w http.ResponseWriter, r *http.Request) {
	var req AddressExplorerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout*4)
	defer cancel()
	r = r.WithContext(ctx)

	resp, err := a.buildAddressExplorer(ctx, req)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}

	if strings.EqualFold(strings.TrimSpace(resp.Mode), "graph") && req.Offset <= 0 {
		storedReq := req
		storedReq.Mode = resp.Mode
		storedReq.Direction = resp.Query.Direction
		storedReq.BatchSize = resp.Query.BatchSize
		storedReq.Offset = 0
		if strings.TrimSpace(resp.RawAddress) != "" {
			storedReq.Address = resp.RawAddress
		}
		if _, err := insertAddressExplorerRun(ctx, a.db, storedReq, firstNonEmpty(resp.RunLabel, shortAddress(resp.Address)), len(resp.Nodes), len(resp.Edges)); err != nil {
			logError(ctx, "address_explorer_run_save_failed", err, nil)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleActorTrackerGraph(w http.ResponseWriter, r *http.Request) {
	var req ActorTrackerRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}

	capturedCtx, capture := withRunLogCapture(r.Context())
	ctx, cancel := context.WithTimeout(capturedCtx, a.cfg.RequestTimeout*4)
	defer cancel()
	r = r.WithContext(ctx)
	defer func() {
		if err := a.saveLastRunLog(capture); err != nil {
			logError(ctx, "actor_tracker_last_run_log_save_failed", err, map[string]any{
				"path": a.cfg.LastRunLogPath,
			})
		}
	}()

	resp, err := a.buildActorTracker(ctx, req)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}

	var actorNames []string
	for _, actor := range resp.Actors {
		actorNames = append(actorNames, actor.Name)
	}
	if _, err := insertGraphRun(ctx, a.db, req, strings.Join(actorNames, ", "), len(resp.Nodes), len(resp.Edges)); err != nil {
		logError(ctx, "graph_run_save_failed", err, nil)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleActorTrackerExpand(w http.ResponseWriter, r *http.Request) {
	var req ActorTrackerExpandRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}

	capturedCtx, capture := withRunLogCapture(r.Context())
	ctx, cancel := context.WithTimeout(capturedCtx, a.cfg.RequestTimeout*3)
	defer cancel()
	r = r.WithContext(ctx)
	defer func() {
		if err := a.saveLastRunLog(capture); err != nil {
			logError(ctx, "actor_tracker_last_run_log_save_failed", err, map[string]any{
				"path": a.cfg.LastRunLogPath,
			})
		}
	}()

	resp, err := a.expandActorTrackerOneHop(ctx, req)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleActorTrackerLiveHoldings(w http.ResponseWriter, r *http.Request) {
	var req ActorTrackerLiveHoldingsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}
	if len(req.Nodes) == 0 {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("at least one node is required"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout*2)
	defer cancel()

	warnings, err := a.refreshActorTrackerLiveHoldings(ctx, req.Nodes)
	if err != nil {
		writeError(r, w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, ActorTrackerLiveHoldingsResponse{
		Nodes:       req.Nodes,
		Warnings:    warnings,
		RefreshedAt: time.Now().UTC(),
	})
}

func (a *App) handleGraphRuns(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	runs, err := listGraphRuns(ctx, a.db)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, err)
		return
	}
	if runs == nil {
		runs = []GraphRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (a *App) handleAddressExplorerRuns(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	runs, err := listAddressExplorerRuns(ctx, a.db)
	if err != nil {
		writeError(r, w, http.StatusInternalServerError, err)
		return
	}
	if runs == nil {
		runs = []AddressExplorerRun{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (a *App) handleGraphRunDelete(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(strings.TrimSpace(r.PathValue("id")))
	if id <= 0 {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("run id is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	if err := deleteGraphRun(ctx, a.db, id); err != nil {
		status := http.StatusInternalServerError
		if err == sql.ErrNoRows {
			status = http.StatusNotFound
		}
		writeError(r, w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (a *App) handleAddressExplorerRunDelete(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(strings.TrimSpace(r.PathValue("id")))
	if id <= 0 {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("run id is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	if err := deleteAddressExplorerRun(ctx, a.db, id); err != nil {
		status := http.StatusInternalServerError
		if err == sql.ErrNoRows {
			status = http.StatusNotFound
		}
		writeError(r, w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func summarizeMidgardLiquidity(address string, actions []midgardAction) map[string]any {
	typeCounts := map[string]int64{}
	for _, action := range actions {
		typeCounts[action.Type]++
	}
	return map[string]any{
		"action_count": len(actions),
		"type_counts":  typeCounts,
	}
}

func buildRebondContinuity(startAddress string, links []RebondLink) map[string]any {
	adj := map[string][]RebondLink{}
	for _, link := range links {
		if link.OldBondAddress == "" || link.NewBondAddress == "" {
			continue
		}
		adj[link.OldBondAddress] = append(adj[link.OldBondAddress], link)
		adj[link.NewBondAddress] = append(adj[link.NewBondAddress], link)
	}

	visitedAddr := map[string]bool{startAddress: true}
	queue := []string{startAddress}
	seenLink := map[string]bool{}
	var outLinks []RebondLink

	for len(queue) > 0 {
		addr := queue[0]
		queue = queue[1:]
		for _, link := range adj[addr] {
			key := fmt.Sprintf("%d|%s|%s|%s", link.Height, link.TxID, link.OldBondAddress, link.NewBondAddress)
			if !seenLink[key] {
				seenLink[key] = true
				outLinks = append(outLinks, link)
			}
			if !visitedAddr[link.OldBondAddress] {
				visitedAddr[link.OldBondAddress] = true
				queue = append(queue, link.OldBondAddress)
			}
			if !visitedAddr[link.NewBondAddress] {
				visitedAddr[link.NewBondAddress] = true
				queue = append(queue, link.NewBondAddress)
			}
		}
	}

	sort.Slice(outLinks, func(i, j int) bool {
		if outLinks[i].Height == outLinks[j].Height {
			return outLinks[i].TxID < outLinks[j].TxID
		}
		return outLinks[i].Height < outLinks[j].Height
	})

	addresses := make([]string, 0, len(visitedAddr))
	for addr := range visitedAddr {
		addresses = append(addresses, addr)
	}
	sort.Strings(addresses)

	return map[string]any{
		"start_address": startAddress,
		"addresses":     addresses,
		"links":         outLinks,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func writeError(r *http.Request, w http.ResponseWriter, status int, err error) {
	logError(r.Context(), "http_handler_error", err, map[string]any{
		"status": status,
		"method": r.Method,
		"path":   r.URL.Path,
	})
	resp := map[string]any{
		"error": err.Error(),
	}
	if requestID := requestIDFromContext(r.Context()); requestID != "" {
		resp["request_id"] = requestID
	}
	writeJSON(w, status, resp)
}

func intQuery(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if v < 1 {
		return fallback
	}
	return v
}

func mapKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

func decodeJSONBody(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func (a *App) handleAddressAnnotations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		annotations, err := listAddressAnnotations(ctx, a.db)
		if err != nil {
			writeError(r, w, http.StatusInternalServerError, err)
			return
		}
		if annotations == nil {
			annotations = []AddressAnnotation{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"annotations": annotations})

	case http.MethodPut:
		var req struct {
			Address string `json:"address"`
			Kind    string `json:"kind"`
			Value   string `json:"value"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(r, w, http.StatusBadRequest, err)
			return
		}
		if req.Address == "" || req.Kind == "" {
			writeError(r, w, http.StatusBadRequest, fmt.Errorf("address and kind are required"))
			return
		}
		if err := upsertAddressAnnotation(ctx, a.db, req.Address, req.Kind, req.Value); err != nil {
			writeError(r, w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case http.MethodDelete:
		var req struct {
			Address string `json:"address"`
			Kind    string `json:"kind"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(r, w, http.StatusBadRequest, err)
			return
		}
		if req.Address == "" || req.Kind == "" {
			writeError(r, w, http.StatusBadRequest, fmt.Errorf("address and kind are required"))
			return
		}
		if err := deleteAddressAnnotation(ctx, a.db, req.Address, req.Kind); err != nil {
			writeError(r, w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleAddressBlocklist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		addresses, err := listBlocklistedAddresses(ctx, a.db)
		if err != nil {
			writeError(r, w, http.StatusInternalServerError, err)
			return
		}
		if addresses == nil {
			addresses = []BlocklistedAddress{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"addresses": addresses})

	case http.MethodPost:
		var req struct {
			Address string `json:"address"`
			Reason  string `json:"reason"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(r, w, http.StatusBadRequest, err)
			return
		}
		if req.Address == "" {
			writeError(r, w, http.StatusBadRequest, fmt.Errorf("address is required"))
			return
		}
		if err := addToBlocklist(ctx, a.db, req.Address, req.Reason); err != nil {
			writeError(r, w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleAddressBlocklistDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	address := r.PathValue("address")
	if address == "" {
		writeError(r, w, http.StatusBadRequest, fmt.Errorf("address is required"))
		return
	}
	if err := removeFromBlocklist(r.Context(), a.db, address); err != nil {
		writeError(r, w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
