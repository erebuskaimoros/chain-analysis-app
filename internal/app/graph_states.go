package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	GraphStateKindActorGraph      = "actor-graph"
	GraphStateKindAddressExplorer = "address-explorer"
)

// GraphStateSummary is the list form of a saved graph state (no payload).
type GraphStateSummary struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	NodeCount int       `json:"node_count"`
	EdgeCount int       `json:"edge_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GraphState is a saved graph state including its full JSON payload.
type GraphState struct {
	GraphStateSummary
	State json.RawMessage `json:"state"`
}

func normalizeGraphStateKind(kind string) (string, error) {
	kind = strings.TrimSpace(strings.ToLower(kind))
	switch kind {
	case GraphStateKindActorGraph, GraphStateKindAddressExplorer:
		return kind, nil
	default:
		return "", fmt.Errorf("invalid graph state kind %q", kind)
	}
}

func insertGraphState(ctx context.Context, db *sql.DB, kind, name string, state json.RawMessage, nodeCount, edgeCount int) (GraphStateSummary, error) {
	normalizedKind, err := normalizeGraphStateKind(kind)
	if err != nil {
		return GraphStateSummary{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return GraphStateSummary{}, fmt.Errorf("graph state name is required")
	}
	if len(state) == 0 || !json.Valid(state) {
		return GraphStateSummary{}, fmt.Errorf("graph state payload is required")
	}

	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	result, err := db.ExecContext(ctx, `
		INSERT INTO graph_states(kind, name, state_json, node_count, edge_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, normalizedKind, name, string(state), nodeCount, edgeCount, stamp, stamp)
	if err != nil {
		return GraphStateSummary{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return GraphStateSummary{}, err
	}
	return GraphStateSummary{
		ID:        id,
		Kind:      normalizedKind,
		Name:      name,
		NodeCount: nodeCount,
		EdgeCount: edgeCount,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func listGraphStates(ctx context.Context, db *sql.DB, kind string) ([]GraphStateSummary, error) {
	normalizedKind, err := normalizeGraphStateKind(kind)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, name, node_count, edge_count, created_at, updated_at
		FROM graph_states
		WHERE kind = ?
		ORDER BY updated_at DESC
	`, normalizedKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GraphStateSummary
	for rows.Next() {
		var item GraphStateSummary
		var createdAt, updatedAt string
		if err := rows.Scan(&item.ID, &item.Kind, &item.Name, &item.NodeCount, &item.EdgeCount, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		out = append(out, item)
	}
	return out, rows.Err()
}

func getGraphState(ctx context.Context, db *sql.DB, id int64) (GraphState, error) {
	var item GraphState
	var createdAt, updatedAt, stateJSON string
	err := db.QueryRowContext(ctx, `
		SELECT id, kind, name, state_json, node_count, edge_count, created_at, updated_at
		FROM graph_states
		WHERE id = ?
	`, id).Scan(&item.ID, &item.Kind, &item.Name, &stateJSON, &item.NodeCount, &item.EdgeCount, &createdAt, &updatedAt)
	if err != nil {
		return GraphState{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	item.State = json.RawMessage(stateJSON)
	return item, nil
}

func deleteGraphState(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM graph_states WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
