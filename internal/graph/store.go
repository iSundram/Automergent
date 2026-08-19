package graph

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var (
	ErrNodeNotFound = errors.New("node not found")
	ErrEdgeNotFound = errors.New("edge not found")
	ErrInvalidInput = errors.New("invalid input")
)

type Store struct {
	db     *sql.DB
	mu     sync.RWMutex
	path   string
}

type Tx struct {
	tx *sql.Tx
	store *Store
}

func NewStore(path string) (*Store, error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return &Store{
		db:   db,
		path: path,
	}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

func (s *Store) BeginTx(ctx context.Context) (*Tx, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &Tx{tx: tx, store: s}, nil
}

func (t *Tx) Commit() error {
	return t.tx.Commit()
}

func (t *Tx) Rollback() error {
	return t.tx.Rollback()
}

func (s *Store) CreateNode(ctx context.Context, node *Node) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	data, err := json.Marshal(node.Data)
	if err != nil {
		return fmt.Errorf("marshal node data: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO nodes (id, type, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		node.ID.String(), node.Type, string(data), node.CreatedAt, node.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert node: %w", err)
	}
	return nil
}

func (t *Tx) CreateNode(ctx context.Context, node *Node) error {
	data, err := json.Marshal(node.Data)
	if err != nil {
		return fmt.Errorf("marshal node data: %w", err)
	}

	_, err = t.tx.ExecContext(ctx, `
		INSERT INTO nodes (id, type, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		node.ID.String(), node.Type, string(data), node.CreatedAt, node.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert node: %w", err)
	}
	return nil
}

func (s *Store) GetNode(ctx context.Context, id uuid.UUID) (*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	var node Node
	var typeStr string
	var data []byte
	var createdAt, updatedAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT id, type, data, created_at, updated_at
		FROM nodes WHERE id = ?`, id.String()).Scan(
		&node.ID, &typeStr, &data, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query node: %w", err)
	}

	node.Type = NodeType(typeStr)
	node.Data = data
	node.CreatedAt = createdAt
	node.UpdatedAt = updatedAt
	return &node, nil
}

func (t *Tx) GetNode(ctx context.Context, id uuid.UUID) (*Node, error) {
	var node Node
	var typeStr string
	var data []byte
	var createdAt, updatedAt time.Time

	err := t.tx.QueryRowContext(ctx, `
		SELECT id, type, data, created_at, updated_at
		FROM nodes WHERE id = ?`, id.String()).Scan(
		&node.ID, &typeStr, &data, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query node: %w", err)
	}

	node.Type = NodeType(typeStr)
	node.Data = data
	node.CreatedAt = createdAt
	node.UpdatedAt = updatedAt
	return &node, nil
}

func (s *Store) UpdateNode(ctx context.Context, node *Node) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	data, err := json.Marshal(node.Data)
	if err != nil {
		return fmt.Errorf("marshal node data: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE nodes SET type = ?, data = ?, updated_at = ?
		WHERE id = ?`,
		node.Type, string(data), time.Now(), node.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNodeNotFound
	}
	return nil
}

func (t *Tx) UpdateNode(ctx context.Context, node *Node) error {
	data, err := json.Marshal(node.Data)
	if err != nil {
		return fmt.Errorf("marshal node data: %w", err)
	}

	result, err := t.tx.ExecContext(ctx, `
		UPDATE nodes SET type = ?, data = ?, updated_at = ?
		WHERE id = ?`,
		node.Type, string(data), time.Now(), node.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNodeNotFound
	}
	return nil
}

func (s *Store) DeleteNode(ctx context.Context, id uuid.UUID) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNodeNotFound
	}
	return nil
}

func (t *Tx) DeleteNode(ctx context.Context, id uuid.UUID) error {
	result, err := t.tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNodeNotFound
	}
	return nil
}

func (s *Store) ListNodes(ctx context.Context, nodeType NodeType, limit, offset int) ([]*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	query := `SELECT id, type, data, created_at, updated_at FROM nodes`
	args := []interface{}{}
	if nodeType != "" {
		query += ` WHERE type = ?`
		args = append(args, nodeType)
	}
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	if offset > 0 {
		query += ` OFFSET ?`
		args = append(args, offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	return scanNodes(rows)
}

func (t *Tx) ListNodes(ctx context.Context, nodeType NodeType, limit, offset int) ([]*Node, error) {
	query := `SELECT id, type, data, created_at, updated_at FROM nodes`
	args := []interface{}{}
	if nodeType != "" {
		query += ` WHERE type = ?`
		args = append(args, nodeType)
	}
	query += ` ORDER BY created_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	if offset > 0 {
		query += ` OFFSET ?`
		args = append(args, offset)
	}

	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	return scanNodes(rows)
}

func (s *Store) CountNodes(ctx context.Context, nodeType NodeType) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return 0, errors.New("store closed")
	}

	query := `SELECT COUNT(*) FROM nodes`
	args := []interface{}{}
	if nodeType != "" {
		query += ` WHERE type = ?`
		args = append(args, nodeType)
	}

	var count int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count nodes: %w", err)
	}
	return count, nil
}

func (s *Store) CreateEdge(ctx context.Context, edge *Edge) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	data, err := json.Marshal(edge.Data)
	if err != nil {
		return fmt.Errorf("marshal edge data: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO edges (id, from_id, to_id, type, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		edge.ID.String(), edge.FromID.String(), edge.ToID.String(),
		edge.Type, string(data), edge.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

func (t *Tx) CreateEdge(ctx context.Context, edge *Edge) error {
	data, err := json.Marshal(edge.Data)
	if err != nil {
		return fmt.Errorf("marshal edge data: %w", err)
	}

	_, err = t.tx.ExecContext(ctx, `
		INSERT INTO edges (id, from_id, to_id, type, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		edge.ID.String(), edge.FromID.String(), edge.ToID.String(),
		edge.Type, string(data), edge.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert edge: %w", err)
	}
	return nil
}

func (s *Store) GetEdge(ctx context.Context, id uuid.UUID) (*Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	var edge Edge
	var fromID, toID, typeStr string
	var data []byte
	var createdAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT id, from_id, to_id, type, data, created_at
		FROM edges WHERE id = ?`, id.String()).Scan(
		&edge.ID, &fromID, &toID, &typeStr, &data, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query edge: %w", err)
	}

	edge.FromID, _ = uuid.Parse(fromID)
	edge.ToID, _ = uuid.Parse(toID)
	edge.Type = EdgeType(typeStr)
	edge.Data = data
	edge.CreatedAt = createdAt
	return &edge, nil
}

func (t *Tx) GetEdge(ctx context.Context, id uuid.UUID) (*Edge, error) {
	var edge Edge
	var fromID, toID, typeStr string
	var data []byte
	var createdAt time.Time

	err := t.tx.QueryRowContext(ctx, `
		SELECT id, from_id, to_id, type, data, created_at
		FROM edges WHERE id = ?`, id.String()).Scan(
		&edge.ID, &fromID, &toID, &typeStr, &data, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query edge: %w", err)
	}

	edge.FromID, _ = uuid.Parse(fromID)
	edge.ToID, _ = uuid.Parse(toID)
	edge.Type = EdgeType(typeStr)
	edge.Data = data
	edge.CreatedAt = createdAt
	return &edge, nil
}

func (s *Store) UpdateEdge(ctx context.Context, edge *Edge) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	data, err := json.Marshal(edge.Data)
	if err != nil {
		return fmt.Errorf("marshal edge data: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE edges SET from_id = ?, to_id = ?, type = ?, data = ?
		WHERE id = ?`,
		edge.FromID.String(), edge.ToID.String(), edge.Type, string(data), edge.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

func (t *Tx) UpdateEdge(ctx context.Context, edge *Edge) error {
	data, err := json.Marshal(edge.Data)
	if err != nil {
		return fmt.Errorf("marshal edge data: %w", err)
	}

	result, err := t.tx.ExecContext(ctx, `
		UPDATE edges SET from_id = ?, to_id = ?, type = ?, data = ?
		WHERE id = ?`,
		edge.FromID.String(), edge.ToID.String(), edge.Type, string(data), edge.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("update edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

func (s *Store) DeleteEdge(ctx context.Context, id uuid.UUID) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM edges WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

func (t *Tx) DeleteEdge(ctx context.Context, id uuid.UUID) error {
	result, err := t.tx.ExecContext(ctx, `DELETE FROM edges WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("delete edge: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return ErrEdgeNotFound
	}
	return nil
}

func (s *Store) GetEdgesFrom(ctx context.Context, fromID uuid.UUID, edgeType EdgeType) ([]*Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	query := `SELECT id, from_id, to_id, type, data, created_at FROM edges WHERE from_id = ?`
	args := []interface{}{fromID.String()}
	if edgeType != "" {
		query += ` AND type = ?`
		args = append(args, edgeType)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func (t *Tx) GetEdgesFrom(ctx context.Context, fromID uuid.UUID, edgeType EdgeType) ([]*Edge, error) {
	query := `SELECT id, from_id, to_id, type, data, created_at FROM edges WHERE from_id = ?`
	args := []interface{}{fromID.String()}
	if edgeType != "" {
		query += ` AND type = ?`
		args = append(args, edgeType)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func (s *Store) GetEdgesTo(ctx context.Context, toID uuid.UUID, edgeType EdgeType) ([]*Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	query := `SELECT id, from_id, to_id, type, data, created_at FROM edges WHERE to_id = ?`
	args := []interface{}{toID.String()}
	if edgeType != "" {
		query += ` AND type = ?`
		args = append(args, edgeType)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func (t *Tx) GetEdgesTo(ctx context.Context, toID uuid.UUID, edgeType EdgeType) ([]*Edge, error) {
	query := `SELECT id, from_id, to_id, type, data, created_at FROM edges WHERE to_id = ?`
	args := []interface{}{toID.String()}
	if edgeType != "" {
		query += ` AND type = ?`
		args = append(args, edgeType)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func (s *Store) GetEdgesBetween(ctx context.Context, fromID, toID uuid.UUID) ([]*Edge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, from_id, to_id, type, data, created_at
		FROM edges WHERE from_id = ? AND to_id = ?
		ORDER BY created_at DESC`, fromID.String(), toID.String())
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func (t *Tx) GetEdgesBetween(ctx context.Context, fromID, toID uuid.UUID) ([]*Edge, error) {
	rows, err := t.tx.QueryContext(ctx, `
		SELECT id, from_id, to_id, type, data, created_at
		FROM edges WHERE from_id = ? AND to_id = ?
		ORDER BY created_at DESC`, fromID.String(), toID.String())
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer rows.Close()

	return scanEdges(rows)
}

func (s *Store) AddNodeLabel(ctx context.Context, nodeID uuid.UUID, label string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return errors.New("store closed")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO node_labels (node_id, label) VALUES (?, ?)`,
		nodeID.String(), label,
	)
	return err
}

func (t *Tx) AddNodeLabel(ctx context.Context, nodeID uuid.UUID, label string) error {
	_, err := t.tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO node_labels (node_id, label) VALUES (?, ?)`,
		nodeID.String(), label,
	)
	return err
}

func (s *Store) GetNodeLabels(ctx context.Context, nodeID uuid.UUID) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	rows, err := s.db.QueryContext(ctx, `SELECT label FROM node_labels WHERE node_id = ?`, nodeID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

func (s *Store) GetNodesByLabel(ctx context.Context, label string) ([]*Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, errors.New("store closed")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.type, n.data, n.created_at, n.updated_at
		FROM nodes n
		JOIN node_labels nl ON n.id = nl.node_id
		WHERE nl.label = ?
		ORDER BY n.created_at DESC`, label)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanNodes(rows)
}

func scanNodes(rows *sql.Rows) ([]*Node, error) {
	var nodes []*Node
	for rows.Next() {
		var node Node
		var typeStr string
		var data []byte
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&node.ID, &typeStr, &data, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		node.Type = NodeType(typeStr)
		node.Data = data
		node.CreatedAt = createdAt
		node.UpdatedAt = updatedAt
		nodes = append(nodes, &node)
	}
	return nodes, rows.Err()
}

func scanEdges(rows *sql.Rows) ([]*Edge, error) {
	var edges []*Edge
	for rows.Next() {
		var edge Edge
		var fromID, toID, typeStr string
		var data []byte
		var createdAt time.Time

		if err := rows.Scan(&edge.ID, &fromID, &toID, &typeStr, &data, &createdAt); err != nil {
			return nil, err
		}

		edge.FromID, _ = uuid.Parse(fromID)
		edge.ToID, _ = uuid.Parse(toID)
		edge.Type = EdgeType(typeStr)
		edge.Data = data
		edge.CreatedAt = createdAt
		edges = append(edges, &edge)
	}
	return edges, rows.Err()
}