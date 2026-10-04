package app

import (
	"context"
	"database/sql"
	"sort"
	"time"
)

// ledgerInterval is an inclusive range of unix seconds.
type ledgerInterval struct {
	From int64
	To   int64
}

func loadLedgerCoverage(ctx context.Context, db *sql.DB, source, address string) ([]ledgerInterval, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT from_ts, to_ts FROM ledger_coverage
		WHERE source = ? AND address = ?
		ORDER BY from_ts ASC
	`, source, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ledgerInterval
	for rows.Next() {
		var iv ledgerInterval
		if err := rows.Scan(&iv.From, &iv.To); err != nil {
			return nil, err
		}
		out = append(out, iv)
	}
	return out, rows.Err()
}

// markLedgerCovered records [from, to] as complete for (source, address),
// merging it with any overlapping or adjacent intervals.
func markLedgerCovered(ctx context.Context, db *sql.DB, source, address string, from, to int64, fetchedAt time.Time) error {
	if to < from {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		SELECT from_ts, to_ts FROM ledger_coverage
		WHERE source = ? AND address = ? AND from_ts <= ? AND to_ts >= ?
	`, source, address, to+1, from-1)
	if err != nil {
		return err
	}
	merged := ledgerInterval{From: from, To: to}
	var overlapping []int64
	for rows.Next() {
		var iv ledgerInterval
		if err := rows.Scan(&iv.From, &iv.To); err != nil {
			rows.Close()
			return err
		}
		overlapping = append(overlapping, iv.From)
		merged.From = min(merged.From, iv.From)
		merged.To = max(merged.To, iv.To)
	}
	rows.Close()
	for _, start := range overlapping {
		if _, err := tx.ExecContext(ctx, `DELETE FROM ledger_coverage WHERE source = ? AND address = ? AND from_ts = ?`, source, address, start); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_coverage(source, address, from_ts, to_ts, fetched_at) VALUES (?, ?, ?, ?, ?)
	`, source, address, merged.From, merged.To, fetchedAt.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// ledgerGaps returns the parts of [from, to] not covered, newest first so a
// shared page budget is spent on recent history before older history.
func ledgerGaps(covered []ledgerInterval, from, to int64) []ledgerInterval {
	if to < from {
		return nil
	}
	sorted := append([]ledgerInterval(nil), covered...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From < sorted[j].From })
	var gaps []ledgerInterval
	cursor := from
	for _, iv := range sorted {
		if iv.To < cursor {
			continue
		}
		if iv.From > to {
			break
		}
		if iv.From > cursor {
			gaps = append(gaps, ledgerInterval{From: cursor, To: min(iv.From-1, to)})
		}
		cursor = max(cursor, iv.To+1)
		if cursor > to {
			break
		}
	}
	if cursor <= to {
		gaps = append(gaps, ledgerInterval{From: cursor, To: to})
	}
	for i, j := 0, len(gaps)-1; i < j; i, j = i+1, j-1 {
		gaps[i], gaps[j] = gaps[j], gaps[i]
	}
	return gaps
}

// ledgerCoverageEnd clamps a fetched range's end so the most recent
// ledgerTailLag stays uncovered and is fetched again next time; upstream
// indexers can still be catching up on it.
func ledgerCoverageEnd(to int64, fetchedAt time.Time) int64 {
	return min(to, fetchedAt.Add(-ledgerTailLag).Unix())
}
