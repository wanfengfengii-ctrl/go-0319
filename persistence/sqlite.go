package persistence

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// SQLiteStore is the SQLite-backed Store implementation.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, enables WAL journal
// mode and runs the schema migration. A single connection is used to keep
// writes fully serialized while WAL still provides crash durability.
func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("persistence: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("persistence: pragma %q: %w", p, err)
		}
	}

	s := &SQLiteStore{db: db}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("persistence: migrate: %w", err)
	}
	if err := s.ensureVersion(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// ensureVersion stamps (or verifies) the schema version.
func (s *SQLiteStore) ensureVersion() error {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM schema_meta").Scan(&n); err != nil {
		return fmt.Errorf("persistence: version: %w", err)
	}
	if n == 0 {
		if _, err := s.db.Exec("INSERT INTO schema_meta(version) VALUES(?)", schemaVersion); err != nil {
			return fmt.Errorf("persistence: version stamp: %w", err)
		}
		return nil
	}
	var v int
	if err := s.db.QueryRow("SELECT version FROM schema_meta LIMIT 1").Scan(&v); err != nil {
		return fmt.Errorf("persistence: version read: %w", err)
	}
	if v != schemaVersion {
		return fmt.Errorf("persistence: schema version %d, want %d", v, schemaVersion)
	}
	return nil
}

// Close closes the underlying database.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// Recover verifies the snapshot digest and self-heals it if it mismatches. It
// returns true when a repair was required.
func (s *SQLiteStore) Recover() (bool, error) {
	computed, err := s.computeSnapshotDigest()
	if err != nil {
		return false, err
	}
	var stored string
	err = s.db.QueryRow("SELECT digest FROM snapshot_digest WHERE id = 0").Scan(&stored)
	if err == sql.ErrNoRows {
		if _, err := s.db.Exec("INSERT INTO snapshot_digest(id, digest) VALUES(0, ?)", computed); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if stored != computed {
		if _, err := s.db.Exec("UPDATE snapshot_digest SET digest = ? WHERE id = 0", computed); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// computeSnapshotDigest hashes the durable aggregate state so that a crash
// leaving an inconsistent snapshot is detectable at startup.
func (s *SQLiteStore) computeSnapshotDigest() (string, error) {
	var sb strings.Builder
	rows, err := s.db.Query(`
		SELECT voyage_id, lander_id, generation, phase, config_version, frozen_digest, terminal_state, current_epoch
		FROM tasks ORDER BY voyage_id, lander_id, generation`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var v, l string
		var g, phase, cv, ce int
		var fd, ts string
		if err := rows.Scan(&v, &l, &g, &phase, &cv, &fd, &ts, &ce); err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "task|%s|%s|%d|%d|%d|%s|%s|%d\n", v, l, g, phase, cv, fd, ts, ce)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	counts := []struct {
		name  string
		query string
	}{
		{"evidence", "SELECT COUNT(*) FROM evidence"},
		{"bindings", "SELECT COUNT(*) FROM bindings"},
		{"leases", "SELECT COUNT(*) FROM resource_leases"},
		{"reviews", "SELECT COUNT(*) FROM reviews"},
		{"recalibrations", "SELECT COUNT(*) FROM recalibrations"},
		{"terminals", "SELECT COUNT(*) FROM terminal_decisions"},
		{"credentials", "SELECT COUNT(*) FROM credentials"},
		{"events", "SELECT COALESCE(MAX(seq),0) FROM event_log"},
	}
	for _, c := range counts {
		var n int64
		if err := s.db.QueryRow(c.query).Scan(&n); err != nil {
			return "", err
		}
		fmt.Fprintf(&sb, "%s|%d\n", c.name, n)
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:]), nil
}
