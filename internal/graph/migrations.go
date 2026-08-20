package graph

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	schemaVersion = 10
)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`,

	`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		data TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`,

	`CREATE TABLE IF NOT EXISTS edges (
		id TEXT PRIMARY KEY,
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		type TEXT NOT NULL,
		data TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (from_id) REFERENCES nodes(id) ON DELETE CASCADE,
		FOREIGN KEY (to_id) REFERENCES nodes(id) ON DELETE CASCADE
	);`,

	`CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type);`,

	`CREATE INDEX IF NOT EXISTS idx_edges_from_id ON edges(from_id);`,

	`CREATE INDEX IF NOT EXISTS idx_edges_to_id ON edges(to_id);`,

	`CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(type);`,

	`CREATE INDEX IF NOT EXISTS idx_edges_from_to ON edges(from_id, to_id);`,

	`CREATE TABLE IF NOT EXISTS node_labels (
		node_id TEXT NOT NULL,
		label TEXT NOT NULL,
		PRIMARY KEY (node_id, label),
		FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
	);`,

	`CREATE INDEX IF NOT EXISTS idx_node_labels_label ON node_labels(label);`,

	`CREATE TRIGGER IF NOT EXISTS trigger_nodes_updated_at
		AFTER UPDATE ON nodes
		BEGIN
			UPDATE nodes SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
		END;`,
}

func Migrate(db *sql.DB) error {
	if err := ensureSchemaVersionTable(db); err != nil {
		return fmt.Errorf("ensure schema version table: %w", err)
	}

	currentVersion, err := getSchemaVersion(db)
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if currentVersion >= schemaVersion {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for i := currentVersion; i < schemaVersion; i++ {
		stmt := migrations[i]
		if _, err := tx.ExecContext(context.Background(), stmt); err != nil {
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
	}

	if _, err := tx.ExecContext(context.Background(),
		"INSERT OR REPLACE INTO schema_version (version) VALUES (?)", schemaVersion); err != nil {
		return fmt.Errorf("update schema version: %w", err)
	}

	return tx.Commit()
}

func ensureSchemaVersionTable(db *sql.DB) error {
	_, err := db.ExecContext(context.Background(), migrations[0])
	return err
}

func getSchemaVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

func OpenDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is empty")
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}
