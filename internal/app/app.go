package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	jobs              *jobRunner
	bondIndexesTHOR   cachedMetadataResult[protocolBondIndexes]
	bondIndexesMAYA   cachedMetadataResult[protocolBondIndexes]
	trackerEndpointRR atomic.Uint64
	buildProgress     *progressRegistry
}

func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(filepathDir(cfg.DBPath), 0o755); err != nil {
		return nil, err
	}

	registerLogSecrets(cfg.EtherscanAPIKey, cfg.EthplorerAPIKey, cfg.AvaCloudAPIKey, cfg.NodeRealAPIKey, cfg.TronGridAPIKey)
	for _, key := range cfg.BlockscoutAPIKeys {
		registerLogSecrets(key)
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
	// One-time move of the retired query-window caches into the ledger. Large
	// databases need more than the schema timeout.
	backfillCtx, cancelBackfill := context.WithTimeout(context.Background(), 30*time.Minute)
	err = backfillLedgerFromQueryCaches(backfillCtx, db)
	cancelBackfill()
	if err != nil {
		return nil, fmt.Errorf("backfill ledger: %w", err)
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
		buildProgress:    newProgressRegistry(),
		jobs:             newJobRunner(jobLogDir(cfg.LastRunLogPath)),
	}
	if a.httpClient.Timeout < 30*time.Second {
		a.httpClient.Timeout = 30 * time.Second
	}
	if cfg.HTTPTransport != nil {
		a.httpClient.Transport = cfg.HTTPTransport
		for _, client := range []*ThorClient{a.thor, a.mid, a.mayaNode, a.mayaMid, a.legacyActions} {
			client.client.Transport = cfg.HTTPTransport
		}
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

// jobLogDir keeps per-job logs next to the last-run log, under runs/.
func jobLogDir(lastRunLogPath string) string {
	if strings.TrimSpace(lastRunLogPath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(lastRunLogPath), "runs")
}
