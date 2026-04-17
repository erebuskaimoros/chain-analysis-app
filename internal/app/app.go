package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type App struct {
	cfg               Config
	db                *sql.DB
	thor              *ThorClient
	mid               *ThorClient
	mayaNode          *ThorClient
	mayaMid           *ThorClient
	legacyActions     *ThorClient
	httpClient        *http.Client
	trackerHealth     *trackerHealthStore
	trackerThrottle   *trackerThrottleStore
	trackerFeatures   *trackerFeatureStore
	trackerBlockNums  *trackerBlockNumberStore
	protocolDirectory cachedMetadataResult[protocolDirectory]
	priceBook         cachedMetadataResult[priceBook]
	trackerEndpointRR atomic.Uint64
}

func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(filepathDir(cfg.DBPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL`); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return nil, err
	}
	if err := initSchema(ctx, db); err != nil {
		return nil, err
	}

	a := &App{
		cfg:              cfg,
		db:               db,
		thor:             NewThorClient(cfg.ThornodeEndpoints, cfg.MidgardTimeout),
		mid:              NewThorClient(cfg.MidgardEndpoints, cfg.MidgardTimeout),
		mayaNode:         NewThorClient(cfg.MayanodeEndpoints, cfg.MidgardTimeout),
		mayaMid:          NewThorClient(cfg.MayaMidgardEndpoints, cfg.MidgardTimeout),
		legacyActions:    NewThorClient(cfg.LegacyActionEndpoints, cfg.MidgardTimeout),
		httpClient:       &http.Client{Timeout: cfg.RequestTimeout},
		trackerHealth:    newTrackerHealthStore(),
		trackerThrottle:  newTrackerThrottleStore(),
		trackerFeatures:  newTrackerFeatureStore(),
		trackerBlockNums: newTrackerBlockNumberStore(),
	}
	if a.httpClient.Timeout < 30*time.Second {
		a.httpClient.Timeout = 30 * time.Second
	}
	return a, nil
}

func (a *App) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func (a *App) saveLastRunLog(capture *runLogCapture) error {
	path := strings.TrimSpace(a.cfg.LastRunLogPath)
	if path == "" || capture == nil {
		return nil
	}

	lines := capture.snapshot()
	if len(lines) == 0 {
		lines = []string{
			fmt.Sprintf(`{"ts":"%s","level":"info","event":"run_log_capture_empty"}`, time.Now().UTC().Format(time.RFC3339Nano)),
		}
	}

	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
		return err
	}
	body := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

func (a *App) fetchPoolsForProtocol(ctx context.Context, protocol string) ([]MidgardPool, error) {
	engine, ok := a.liquidityEngine(protocol)
	if !ok || engine.MidgardClient == nil {
		return nil, fmt.Errorf("%s liquidity midgard unavailable", normalizeSourceProtocol(protocol))
	}
	var pools []MidgardPool
	if err := engine.MidgardClient.GetJSON(ctx, "/pools", &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

func getString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		case float64:
			return strconv.FormatInt(int64(t), 10)
		case int64:
			return strconv.FormatInt(t, 10)
		case int:
			return strconv.Itoa(t)
		case bool:
			if t {
				return "true"
			}
			return "false"
		default:
			raw, _ := json.Marshal(t)
			s := strings.Trim(string(raw), `"`)
			if strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func pickString(m map[string]any, keys ...string) string {
	return getString(m, keys...)
}

func parseInt64(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err == nil {
		return n
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return 0
	}
	n, _ = strconv.ParseInt(parts[0], 10, 64)
	return n
}

func filepathDir(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "."
	}
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return "."
	}
	if idx == 0 {
		return "/"
	}
	return path[:idx]
}

// extractRebondLinkFromMidgardBondAction parses Midgard rebond actions into
// rebond continuity links using the documented metadata.rebond payload.
func extractRebondLinkFromMidgardBondAction(action midgardAction) (RebondLink, bool) {
	if action.Metadata.Rebond == nil {
		return RebondLink{}, false
	}

	memo := strings.TrimSpace(action.Metadata.Rebond.Memo)
	nodeAddress := strings.TrimSpace(action.Metadata.Rebond.NodeAddress)
	newBondAddress := strings.TrimSpace(action.Metadata.Rebond.NewBondAddress)
	if memo == "" {
		return RebondLink{}, false
	}

	parts := strings.Split(memo, ":")
	if len(parts) < 3 {
		return RebondLink{}, false
	}
	memoAction := strings.ToUpper(strings.TrimSpace(parts[0]))
	if memoAction != "REBOND" {
		return RebondLink{}, false
	}
	actionType := strings.ToUpper(strings.TrimSpace(action.Type))
	if actionType != "" && actionType != "REBOND" {
		return RebondLink{}, false
	}

	if nodeAddress == "" {
		nodeAddress = strings.TrimSpace(parts[1])
	}
	if newBondAddress == "" {
		newBondAddress = strings.TrimSpace(parts[2])
	}
	if newBondAddress == "" {
		return RebondLink{}, false
	}

	oldBondAddress := ""
	if len(action.In) > 0 {
		oldBondAddress = strings.TrimSpace(action.In[0].Address)
	}
	if oldBondAddress == "" || strings.EqualFold(oldBondAddress, newBondAddress) {
		return RebondLink{}, false
	}

	height := parseInt64(action.Height)
	txID := ""
	if len(action.In) > 0 {
		txID = strings.ToUpper(strings.TrimSpace(action.In[0].TxID))
	}

	return RebondLink{
		Height:         height,
		TxID:           txID,
		NodeAddress:    nodeAddress,
		OldBondAddress: oldBondAddress,
		NewBondAddress: newBondAddress,
		Data:           map[string]any{"memo": memo, "source": "midgard"},
	}, true
}

// extractRebondLinksFromMidgardBondActions iterates bond-class Midgard actions,
// replaces cached rebond continuity for the fetched txids, and stores valid
// metadata.rebond links on-demand.
func (a *App) extractRebondLinksFromMidgardBondActions(ctx context.Context, actions []midgardAction) {
	var txIDs []string
	for _, action := range actions {
		if midgardActionClass(action) != "bonds" {
			continue
		}
		txIDs = append(txIDs, midgardActionTxIDs(action)...)
	}
	txIDs = uniqueStrings(txIDs)
	if err := deleteRebondLinksByTxIDs(ctx, a.db, txIDs); err != nil {
		logError(ctx, "rebond_link_delete_failed", err, map[string]any{
			"tx_count": len(txIDs),
		})
		return
	}

	for _, action := range actions {
		if midgardActionClass(action) != "bonds" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(action.Status), "failed") {
			continue
		}
		link, ok := extractRebondLinkFromMidgardBondAction(action)
		if !ok {
			continue
		}
		if err := insertRebondLink(ctx, a.db, link); err != nil {
			logError(ctx, "rebond_link_insert_failed", err, map[string]any{
				"old_bond_address": link.OldBondAddress,
				"new_bond_address": link.NewBondAddress,
			})
		}
	}
}
