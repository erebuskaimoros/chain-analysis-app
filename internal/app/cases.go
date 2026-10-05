package app

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cases collect what an investigation found: addresses, transactions, saved
// traces, saved graph states and actors, with notes, so it can be exported
// as one report.

const (
	CaseItemAddress    = "address"
	CaseItemTx         = "tx"
	CaseItemTraceRun   = "trace_run"
	CaseItemGraphState = "graph_state"
	CaseItemActor      = "actor"
)

var caseItemKinds = map[string]bool{
	CaseItemAddress: true, CaseItemTx: true, CaseItemTraceRun: true, CaseItemGraphState: true, CaseItemActor: true,
}

type Case struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	NotesMD   string     `json:"notes_md"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ItemCount int        `json:"item_count"`
	Items     []CaseItem `json:"items,omitempty"`
}

type CaseItem struct {
	ID       int64     `json:"id"`
	CaseID   int64     `json:"case_id"`
	Kind     string    `json:"kind"`
	Ref      string    `json:"ref"`
	Note     string    `json:"note"`
	PinnedAt time.Time `json:"pinned_at"`
	// Title describes the item for listings: a trace or graph state's name,
	// an actor's name, or the address's best label.
	Title string `json:"title,omitempty"`
}

type CaseUpsertRequest struct {
	Title   string `json:"title"`
	NotesMD string `json:"notes_md"`
}

type CaseItemRequest struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
	Note string `json:"note"`
}

func (a *App) ListCases(ctx context.Context) ([]Case, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT c.id, c.title, c.notes_md, c.created_at, c.updated_at, COUNT(i.id)
		FROM cases c LEFT JOIN case_items i ON i.case_id = c.id
		GROUP BY c.id ORDER BY c.updated_at DESC, c.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cases := []Case{}
	for rows.Next() {
		var c Case
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &c.NotesMD, &created, &updated, &c.ItemCount); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

func (a *App) CreateCase(ctx context.Context, req CaseUpsertRequest) (Case, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return Case{}, fmt.Errorf("case title is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := a.db.ExecContext(ctx, `INSERT INTO cases(title, notes_md, created_at, updated_at) VALUES (?, ?, ?, ?)`, title, req.NotesMD, now, now)
	if err != nil {
		return Case{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Case{}, err
	}
	return a.GetCase(ctx, id)
}

func (a *App) UpdateCase(ctx context.Context, id int64, req CaseUpsertRequest) (Case, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return Case{}, fmt.Errorf("case title is required")
	}
	res, err := a.db.ExecContext(ctx, `UPDATE cases SET title = ?, notes_md = ?, updated_at = ? WHERE id = ?`,
		title, req.NotesMD, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return Case{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Case{}, sql.ErrNoRows
	}
	return a.GetCase(ctx, id)
}

func (a *App) DeleteCase(ctx context.Context, id int64) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM case_items WHERE case_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM cases WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (a *App) GetCase(ctx context.Context, id int64) (Case, error) {
	var c Case
	var created, updated string
	err := a.db.QueryRowContext(ctx, `SELECT id, title, notes_md, created_at, updated_at FROM cases WHERE id = ?`, id).
		Scan(&c.ID, &c.Title, &c.NotesMD, &created, &updated)
	if err != nil {
		return Case{}, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, case_id, kind, ref, note, pinned_at FROM case_items
		WHERE case_id = ? ORDER BY pinned_at ASC, id ASC
	`, id)
	if err != nil {
		return Case{}, err
	}
	defer rows.Close()
	c.Items = []CaseItem{}
	for rows.Next() {
		var item CaseItem
		var pinned string
		if err := rows.Scan(&item.ID, &item.CaseID, &item.Kind, &item.Ref, &item.Note, &pinned); err != nil {
			return Case{}, err
		}
		item.PinnedAt, _ = time.Parse(time.RFC3339Nano, pinned)
		c.Items = append(c.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Case{}, err
	}
	c.ItemCount = len(c.Items)
	a.titleCaseItems(ctx, c.Items)
	return c, nil
}

// AddCaseItem pins an item to a case; pinning the same item again updates
// its note.
func (a *App) AddCaseItem(ctx context.Context, caseID int64, req CaseItemRequest) (CaseItem, error) {
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	ref := strings.TrimSpace(req.Ref)
	if !caseItemKinds[kind] {
		return CaseItem{}, fmt.Errorf("invalid case item kind %q: want address, tx, trace_run, graph_state or actor", req.Kind)
	}
	if ref == "" {
		return CaseItem{}, fmt.Errorf("case item ref is required")
	}
	switch kind {
	case CaseItemTraceRun, CaseItemGraphState, CaseItemActor:
		id, err := strconv.ParseInt(ref, 10, 64)
		if err != nil || id <= 0 {
			return CaseItem{}, fmt.Errorf("invalid %s ref %q: want a numeric id", kind, ref)
		}
		if err := a.caseRefExists(ctx, kind, id); err != nil {
			return CaseItem{}, fmt.Errorf("invalid %s ref %d: %w", kind, id, err)
		}
		ref = strconv.FormatInt(id, 10)
	case CaseItemTx:
		ref = cleanTxID(ref)
	case CaseItemAddress:
		address := normalizeFrontierAddress(ref)
		if address.Address == "" {
			return CaseItem{}, fmt.Errorf("invalid address ref %q", req.Ref)
		}
		ref = encodeFrontierAddress(address)
	}
	if _, err := a.GetCase(ctx, caseID); err != nil {
		return CaseItem{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO case_items(case_id, kind, ref, note, pinned_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(case_id, kind, ref) DO UPDATE SET note = excluded.note
	`, caseID, kind, ref, strings.TrimSpace(req.Note), now); err != nil {
		return CaseItem{}, err
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE cases SET updated_at = ? WHERE id = ?`, now, caseID); err != nil {
		return CaseItem{}, err
	}
	var item CaseItem
	var pinned string
	err := a.db.QueryRowContext(ctx, `SELECT id, case_id, kind, ref, note, pinned_at FROM case_items WHERE case_id = ? AND kind = ? AND ref = ?`, caseID, kind, ref).
		Scan(&item.ID, &item.CaseID, &item.Kind, &item.Ref, &item.Note, &pinned)
	if err != nil {
		return CaseItem{}, err
	}
	item.PinnedAt, _ = time.Parse(time.RFC3339Nano, pinned)
	items := []CaseItem{item}
	a.titleCaseItems(ctx, items)
	return items[0], nil
}

func (a *App) DeleteCaseItem(ctx context.Context, caseID, itemID int64) error {
	res, err := a.db.ExecContext(ctx, `DELETE FROM case_items WHERE case_id = ? AND id = ?`, caseID, itemID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	_, err = a.db.ExecContext(ctx, `UPDATE cases SET updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), caseID)
	return err
}

func (a *App) caseRefExists(ctx context.Context, kind string, id int64) error {
	table := map[string]string{CaseItemTraceRun: "trace_runs", CaseItemGraphState: "graph_states", CaseItemActor: "actors"}[kind]
	var found int64
	if err := a.db.QueryRowContext(ctx, `SELECT id FROM `+table+` WHERE id = ?`, id).Scan(&found); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("not found")
		}
		return err
	}
	return nil
}

// titleCaseItems fills each item's Title from what it refers to.
func (a *App) titleCaseItems(ctx context.Context, items []CaseItem) {
	var addresses []string
	for _, item := range items {
		if item.Kind == CaseItemAddress {
			addresses = append(addresses, caseItemAddress(item.Ref))
		}
	}
	labels := map[string][]AddressLabel{}
	if len(addresses) > 0 {
		labels, _ = lookupAddressLabels(ctx, a.db, addresses)
	}
	for i := range items {
		item := &items[i]
		var title string
		switch item.Kind {
		case CaseItemTraceRun:
			_ = a.db.QueryRowContext(ctx, `SELECT title FROM trace_runs WHERE id = ?`, item.Ref).Scan(&title)
		case CaseItemGraphState:
			_ = a.db.QueryRowContext(ctx, `SELECT name FROM graph_states WHERE id = ?`, item.Ref).Scan(&title)
		case CaseItemActor:
			_ = a.db.QueryRowContext(ctx, `SELECT name FROM actors WHERE id = ?`, item.Ref).Scan(&title)
		case CaseItemAddress:
			if found := labels[normalizeAddress(caseItemAddress(item.Ref))]; len(found) > 0 {
				title = found[0].Label
			}
		}
		item.Title = title
	}
}

// caseItemAddress returns the address of a CHAIN|address ref.
func caseItemAddress(ref string) string {
	if _, address, ok := strings.Cut(ref, "|"); ok {
		return address
	}
	return ref
}

// caseItemChain returns the chain of a CHAIN|address ref.
func caseItemChain(ref string) string {
	if chain, _, ok := strings.Cut(ref, "|"); ok {
		return chain
	}
	return normalizeChain("", ref)
}
