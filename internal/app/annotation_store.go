package app

import (
	"context"
	"database/sql"
	"time"
)

type AddressAnnotation struct {
	ID                int64     `json:"id"`
	Address           string    `json:"address"`
	NormalizedAddress string    `json:"normalized_address"`
	Kind              string    `json:"kind"`
	Value             string    `json:"value"`
	CreatedAt         time.Time `json:"created_at"`
}

type BlocklistedAddress struct {
	ID                int64     `json:"id"`
	Address           string    `json:"address"`
	NormalizedAddress string    `json:"normalized_address"`
	Reason            string    `json:"reason"`
	CreatedAt         time.Time `json:"created_at"`
}

func listAddressAnnotations(ctx context.Context, db *sql.DB) ([]AddressAnnotation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, address, normalized_address, kind, value, created_at
		FROM address_annotations
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AddressAnnotation
	for rows.Next() {
		var a AddressAnnotation
		var createdAt string
		if err := rows.Scan(&a.ID, &a.Address, &a.NormalizedAddress, &a.Kind, &a.Value, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, a)
	}
	return out, rows.Err()
}

func upsertAddressAnnotation(ctx context.Context, db *sql.DB, address, kind, value string) error {
	norm := normalizeAddress(address)
	_, err := db.ExecContext(ctx, `
		INSERT INTO address_annotations(address, normalized_address, kind, value, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(normalized_address, kind) DO UPDATE SET value = excluded.value
	`, address, norm, kind, value, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func deleteAddressAnnotation(ctx context.Context, db *sql.DB, address, kind string) error {
	norm := normalizeAddress(address)
	_, err := db.ExecContext(ctx, `
		DELETE FROM address_annotations WHERE normalized_address = ? AND kind = ?
	`, norm, kind)
	return err
}

func listBlocklistedAddresses(ctx context.Context, db *sql.DB) ([]BlocklistedAddress, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, address, normalized_address, reason, created_at
		FROM address_blocklist
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BlocklistedAddress
	for rows.Next() {
		var b BlocklistedAddress
		var createdAt string
		if err := rows.Scan(&b.ID, &b.Address, &b.NormalizedAddress, &b.Reason, &createdAt); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, b)
	}
	return out, rows.Err()
}

func addToBlocklist(ctx context.Context, db *sql.DB, address, reason string) error {
	norm := normalizeAddress(address)
	_, err := db.ExecContext(ctx, `
		INSERT INTO address_blocklist(address, normalized_address, reason, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(normalized_address) DO NOTHING
	`, address, norm, reason, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func removeFromBlocklist(ctx context.Context, db *sql.DB, address string) error {
	norm := normalizeAddress(address)
	_, err := db.ExecContext(ctx, `
		DELETE FROM address_blocklist WHERE normalized_address = ?
	`, norm)
	return err
}
