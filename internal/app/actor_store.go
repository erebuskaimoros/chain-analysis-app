package app

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

func listActors(ctx context.Context, db *sql.DB) ([]Actor, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, color, notes, created_at, updated_at
		FROM actors
		ORDER BY lower(name) ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actors []Actor
	for rows.Next() {
		var actor Actor
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&actor.ID, &actor.Name, &actor.Color, &actor.Notes, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		actor.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		actor.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		actors = append(actors, actor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	addresses, err := loadActorAddresses(ctx, db, actorIDs(actors))
	if err != nil {
		return nil, err
	}
	attachActorAddresses(actors, addresses)
	return actors, nil
}

func getActorsByIDs(ctx context.Context, db *sql.DB, ids []int64) ([]Actor, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	where, args := buildInt64InClause(ids)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, name, color, notes, created_at, updated_at
		FROM actors
		WHERE id IN (%s)
		ORDER BY lower(name) ASC, id ASC
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actors []Actor
	for rows.Next() {
		var actor Actor
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&actor.ID, &actor.Name, &actor.Color, &actor.Notes, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		actor.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		actor.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		actors = append(actors, actor)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	addresses, err := loadActorAddresses(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	attachActorAddresses(actors, addresses)
	return actors, nil
}

func getActorByID(ctx context.Context, db *sql.DB, id int64) (Actor, error) {
	actors, err := getActorsByIDs(ctx, db, []int64{id})
	if err != nil {
		return Actor{}, err
	}
	if len(actors) == 0 {
		return Actor{}, sql.ErrNoRows
	}
	return actors[0], nil
}

func upsertActor(ctx context.Context, db *sql.DB, actorID int64, req ActorUpsertRequest) (Actor, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Actor{}, fmt.Errorf("actor name is required")
	}
	color := strings.TrimSpace(req.Color)
	if color == "" {
		color = "#4ca3ff"
	}

	prepared, err := prepareActorAddresses(req.Addresses)
	if err != nil {
		return Actor{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Actor{}, err
	}
	commit := false
	defer func() {
		if !commit {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if actorID == 0 {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO actors(name, color, notes, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?)
		`, name, color, strings.TrimSpace(req.Notes), now, now)
		if err != nil {
			return Actor{}, err
		}
		actorID, err = res.LastInsertId()
		if err != nil {
			return Actor{}, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `
			UPDATE actors
			SET name = ?, color = ?, notes = ?, updated_at = ?
			WHERE id = ?
		`, name, color, strings.TrimSpace(req.Notes), now, actorID)
		if err != nil {
			return Actor{}, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return Actor{}, sql.ErrNoRows
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM actor_addresses WHERE actor_id = ?`, actorID); err != nil {
			return Actor{}, err
		}
	}

	for _, addr := range prepared {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO actor_addresses(actor_id, address, normalized_address, chain_hint, label, created_at)
			VALUES(?, ?, ?, ?, ?, ?)
		`, actorID, addr.Address, addr.NormalizedAddress, addr.ChainHint, addr.Label, now); err != nil {
			return Actor{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Actor{}, err
	}
	commit = true
	return getActorByID(ctx, db, actorID)
}

func deleteActor(ctx context.Context, db *sql.DB, actorID int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM actors WHERE id = ?`, actorID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func prepareActorAddresses(in []ActorAddressInput) ([]ActorAddress, error) {
	seen := map[string]struct{}{}
	out := make([]ActorAddress, 0, len(in))
	for _, item := range in {
		address := strings.TrimSpace(item.Address)
		if address == "" {
			continue
		}
		normalized := normalizeAddress(address)
		if _, exists := seen[normalized]; exists {
			return nil, fmt.Errorf("duplicate address in actor: %s", address)
		}
		seen[normalized] = struct{}{}
		out = append(out, ActorAddress{
			Address:           address,
			NormalizedAddress: normalized,
			ChainHint:         strings.ToUpper(strings.TrimSpace(item.ChainHint)),
			Label:             strings.TrimSpace(item.Label),
		})
	}
	return out, nil
}

func loadActorAddresses(ctx context.Context, db *sql.DB, ids []int64) (map[int64][]ActorAddress, error) {
	result := map[int64][]ActorAddress{}
	if len(ids) == 0 {
		return result, nil
	}
	where, args := buildInt64InClause(ids)
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, actor_id, address, normalized_address, chain_hint, label, created_at
		FROM actor_addresses
		WHERE actor_id IN (%s)
		ORDER BY actor_id ASC, lower(address) ASC, id ASC
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var addr ActorAddress
		var createdAt string
		if err := rows.Scan(&addr.ID, &addr.ActorID, &addr.Address, &addr.NormalizedAddress, &addr.ChainHint, &addr.Label, &createdAt); err != nil {
			return nil, err
		}
		addr.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		result[addr.ActorID] = append(result[addr.ActorID], addr)
	}
	return result, rows.Err()
}

func attachActorAddresses(actors []Actor, addresses map[int64][]ActorAddress) {
	for i := range actors {
		actors[i].Addresses = addresses[actors[i].ID]
	}
}

func actorIDs(actors []Actor) []int64 {
	out := make([]int64, 0, len(actors))
	for _, actor := range actors {
		out = append(out, actor.ID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func isLikelyEVMAddress(address string) bool {
	address = strings.TrimSpace(address)
	if len(address) != 42 || !strings.HasPrefix(strings.ToLower(address), "0x") {
		return false
	}
	for _, r := range address[2:] {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func normalizeAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	lower := strings.ToLower(address)
	switch {
	case isLikelyEVMAddress(address):
		return "0x" + strings.ToLower(address[2:])
	case strings.HasPrefix(lower, "thor"),
		strings.HasPrefix(lower, "maya"),
		strings.HasPrefix(lower, "bc1"),
		strings.HasPrefix(lower, "ltc1"),
		strings.HasPrefix(lower, "cosmos1"),
		strings.HasPrefix(lower, "account_rdx"),
		strings.HasPrefix(lower, "component_rdx"),
		strings.HasPrefix(lower, "resource_rdx"):
		return lower
	case strings.HasPrefix(lower, "bitcoincash:"):
		return strings.TrimPrefix(lower, "bitcoincash:")
	default:
		return address
	}
}

func buildInt64InClause(values []int64) (string, []any) {
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, v := range values {
		placeholders = append(placeholders, "?")
		args = append(args, v)
	}
	return strings.Join(placeholders, ","), args
}
