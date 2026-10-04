package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// The ledger stores fetched chain history row by row, keyed by the queried
// address, instead of caching whole query windows. Each (source, address)
// listing keeps the order the upstream returned rows in (batch, position), so
// reads reproduce what a fresh fetch would return. ledger_coverage records
// which time ranges of each listing are complete.

const ledgerTailLag = 10 * time.Minute

func ledgerActionSource(protocol string) string {
	return "midgard:" + normalizeSourceProtocol(protocol)
}

func ledgerMergedTHORSource() string {
	return "merged:" + sourceProtocolTHOR
}

func ledgerTransferSource(provider, chain string) string {
	return "transfers:" + provider + ":" + chain
}

func newLedgerBatch(ctx context.Context, tx *sql.Tx) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO ledger_batches DEFAULT VALUES`)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// upsertLedgerActions stores actions fetched for address under source.
// Actions sharing a key within one fetch get an occurrence suffix so distinct
// rows never collapse; re-fetching updates rows in place, which keeps status
// changes (pending to success) without reordering the listing.
func upsertLedgerActions(ctx context.Context, db *sql.DB, source, address string, actions []midgardAction) error {
	if len(actions) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	batch, err := newLedgerBatch(ctx, tx)
	if err != nil {
		return err
	}
	occurrences := map[string]int{}
	for position, action := range actions {
		key := midgardActionKey(action)
		if key == "" {
			key = "date:" + action.Date + "|height:" + action.Height
		}
		occurrences[key]++
		if n := occurrences[key]; n > 1 {
			key += "#" + strconv.Itoa(n)
		}
		raw, err := json.Marshal(action)
		if err != nil {
			return err
		}
		blockTime := parseMidgardActionTime(action.Date).Unix()
		if action.Date == "" {
			blockTime = 0
		}
		height := parseInt64(action.Height)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_actions(source, action_key, block_time, height, action_json)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(source, action_key) DO UPDATE SET action_json = excluded.action_json
		`, source, key, blockTime, height, string(raw)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_action_addresses(source, address, action_key, block_time, height, batch, position)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(source, address, action_key) DO NOTHING
		`, source, address, key, blockTime, height, batch, position); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// queryLedgerActions returns the stored actions for address within
// [fromTS, toTS], newest first, in upstream order within a block.
func queryLedgerActions(ctx context.Context, db *sql.DB, source, address string, fromTS, toTS int64) ([]midgardAction, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.action_json
		FROM ledger_action_addresses la
		JOIN ledger_actions a ON a.source = la.source AND a.action_key = la.action_key
		WHERE la.source = ? AND la.address = ? AND la.block_time >= ? AND la.block_time <= ?
		ORDER BY la.height DESC, la.batch ASC, la.position ASC
	`, source, address, fromTS, toTS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []midgardAction
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var action midgardAction
		if err := json.Unmarshal([]byte(raw), &action); err != nil {
			return nil, fmt.Errorf("decode ledger action: %w", err)
		}
		out = append(out, action)
	}
	return out, rows.Err()
}

// upsertLedgerTransfers stores external transfers fetched for address. The
// transfer key already identifies a transfer (callers dedupe by it).
func upsertLedgerTransfers(ctx context.Context, db *sql.DB, source, address string, transfers []externalTransfer) error {
	if len(transfers) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	batch, err := newLedgerBatch(ctx, tx)
	if err != nil {
		return err
	}
	for position, transfer := range transfers {
		key := externalTransferKey(transfer)
		raw, err := json.Marshal(transfer)
		if err != nil {
			return err
		}
		blockTime := transfer.Time.Unix()
		if transfer.Time.IsZero() {
			blockTime = 0
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_transfers(source, transfer_key, block_time, transfer_json)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(source, transfer_key) DO UPDATE SET transfer_json = excluded.transfer_json
		`, source, key, blockTime, string(raw)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ledger_transfer_addresses(source, address, transfer_key, block_time, batch, position)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(source, address, transfer_key) DO NOTHING
		`, source, address, key, blockTime, batch, position); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// queryLedgerTransfers returns stored transfers for address within
// [fromTS, toTS] in the order providers returned them.
func queryLedgerTransfers(ctx context.Context, db *sql.DB, source, address string, fromTS, toTS int64) ([]externalTransfer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.transfer_json
		FROM ledger_transfer_addresses lt
		JOIN ledger_transfers t ON t.source = lt.source AND t.transfer_key = lt.transfer_key
		WHERE lt.source = ? AND lt.address = ? AND lt.block_time >= ? AND lt.block_time <= ?
		ORDER BY lt.batch ASC, lt.position ASC
	`, source, address, fromTS, toTS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []externalTransfer
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var transfer externalTransfer
		if err := json.Unmarshal([]byte(raw), &transfer); err != nil {
			return nil, fmt.Errorf("decode ledger transfer: %w", err)
		}
		out = append(out, transfer)
	}
	return out, rows.Err()
}
