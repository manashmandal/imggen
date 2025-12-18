package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    name TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    current_iteration_id TEXT,
    model TEXT NOT NULL DEFAULT 'gpt-image-1'
);

CREATE TABLE IF NOT EXISTS iterations (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    parent_id TEXT,
    operation TEXT NOT NULL,
    prompt TEXT NOT NULL,
    revised_prompt TEXT,
    model TEXT NOT NULL,
    image_path TEXT NOT NULL,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    metadata_json TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS cost_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    iteration_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    cost REAL NOT NULL,
    image_count INTEGER NOT NULL DEFAULT 1,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (iteration_id) REFERENCES iterations(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_iterations_session_id ON iterations(session_id);
CREATE INDEX IF NOT EXISTS idx_iterations_parent_id ON iterations(parent_id);
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at);
CREATE INDEX IF NOT EXISTS idx_cost_log_timestamp ON cost_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_cost_log_provider ON cost_log(provider);
CREATE INDEX IF NOT EXISTS idx_cost_log_session_id ON cost_log(session_id);
`

type Store struct {
	db *sql.DB
}

func NewStore() (*Store, error) {
	dbPath, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	return NewStoreWithPath(dbPath)
}

func NewStoreWithPath(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &Store{db: db}, nil
}

func defaultDBPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".imggen", "sessions.db"), nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, name, created_at, updated_at, current_iteration_id, model)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Name, sess.CreatedAt, sess.UpdatedAt, sess.CurrentIterationID, sess.Model)
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, created_at, updated_at, current_iteration_id, model
		 FROM sessions WHERE id = ?`, id)

	sess := &Session{}
	var currentIterID sql.NullString
	var name sql.NullString
	err := row.Scan(&sess.ID, &name, &sess.CreatedAt, &sess.UpdatedAt, &currentIterID, &sess.Model)
	if err != nil {
		return nil, err
	}
	sess.Name = name.String
	sess.CurrentIterationID = currentIterID.String
	return sess, nil
}

func (s *Store) UpdateSession(ctx context.Context, sess *Session) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET name = ?, updated_at = ?, current_iteration_id = ?, model = ?
		 WHERE id = ?`,
		sess.Name, sess.UpdatedAt, sess.CurrentIterationID, sess.Model, sess.ID)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) ListSessions(ctx context.Context) ([]*Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at, updated_at, current_iteration_id, model
		 FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess := &Session{}
		var currentIterID sql.NullString
		var name sql.NullString
		if err := rows.Scan(&sess.ID, &name, &sess.CreatedAt, &sess.UpdatedAt, &currentIterID, &sess.Model); err != nil {
			return nil, err
		}
		sess.Name = name.String
		sess.CurrentIterationID = currentIterID.String
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *Store) CreateIteration(ctx context.Context, iter *Iteration) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO iterations (id, session_id, parent_id, operation, prompt, revised_prompt, model, image_path, timestamp, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iter.ID, iter.SessionID, nullString(iter.ParentID), iter.Operation, iter.Prompt,
		nullString(iter.RevisedPrompt), iter.Model, iter.ImagePath, iter.Timestamp, iter.Metadata.ToJSON())
	return err
}

func (s *Store) GetIteration(ctx context.Context, id string) (*Iteration, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, parent_id, operation, prompt, revised_prompt, model, image_path, timestamp, metadata_json
		 FROM iterations WHERE id = ?`, id)

	iter := &Iteration{}
	var parentID, revisedPrompt, metadataJSON sql.NullString
	err := row.Scan(&iter.ID, &iter.SessionID, &parentID, &iter.Operation, &iter.Prompt,
		&revisedPrompt, &iter.Model, &iter.ImagePath, &iter.Timestamp, &metadataJSON)
	if err != nil {
		return nil, err
	}
	iter.ParentID = parentID.String
	iter.RevisedPrompt = revisedPrompt.String
	iter.Metadata = ParseIterationMetadata(metadataJSON.String)
	return iter, nil
}

func (s *Store) ListIterations(ctx context.Context, sessionID string) ([]*Iteration, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, parent_id, operation, prompt, revised_prompt, model, image_path, timestamp, metadata_json
		 FROM iterations WHERE session_id = ? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var iterations []*Iteration
	for rows.Next() {
		iter := &Iteration{}
		var parentID, revisedPrompt, metadataJSON sql.NullString
		if err := rows.Scan(&iter.ID, &iter.SessionID, &parentID, &iter.Operation, &iter.Prompt,
			&revisedPrompt, &iter.Model, &iter.ImagePath, &iter.Timestamp, &metadataJSON); err != nil {
			return nil, err
		}
		iter.ParentID = parentID.String
		iter.RevisedPrompt = revisedPrompt.String
		iter.Metadata = ParseIterationMetadata(metadataJSON.String)
		iterations = append(iterations, iter)
	}
	return iterations, rows.Err()
}

func (s *Store) CountIterations(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM iterations WHERE session_id = ?`, sessionID).Scan(&count)
	return count, err
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func DefaultImageDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".imggen", "images"), nil
}

func ImageDir(sessionID string) (string, error) {
	baseDir, err := DefaultImageDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, sessionID), nil
}

func EnsureImageDir(sessionID string) (string, error) {
	dir, err := ImageDir(sessionID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

type CostEntry struct {
	IterationID string
	SessionID   string
	Provider    string
	Model       string
	Cost        float64
	ImageCount  int
	Timestamp   time.Time
}

type CostSummary struct {
	TotalCost   float64
	ImageCount  int
	EntryCount  int
}

type ProviderCostSummary struct {
	Provider   string
	TotalCost  float64
	ImageCount int
}

func (s *Store) LogCost(ctx context.Context, entry *CostEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_log (iteration_id, session_id, provider, model, cost, image_count, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.IterationID, entry.SessionID, entry.Provider, entry.Model,
		entry.Cost, entry.ImageCount, entry.Timestamp)
	return err
}

func (s *Store) GetCostByDateRange(ctx context.Context, start, end time.Time) (*CostSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost), 0), COALESCE(SUM(image_count), 0), COUNT(*)
		 FROM cost_log WHERE timestamp >= ? AND timestamp < ?`,
		start, end)

	var summary CostSummary
	if err := row.Scan(&summary.TotalCost, &summary.ImageCount, &summary.EntryCount); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Store) GetCostByProvider(ctx context.Context) ([]ProviderCostSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT provider, COALESCE(SUM(cost), 0), COALESCE(SUM(image_count), 0)
		 FROM cost_log GROUP BY provider ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []ProviderCostSummary
	for rows.Next() {
		var ps ProviderCostSummary
		if err := rows.Scan(&ps.Provider, &ps.TotalCost, &ps.ImageCount); err != nil {
			return nil, err
		}
		summaries = append(summaries, ps)
	}
	return summaries, rows.Err()
}

func (s *Store) GetTotalCost(ctx context.Context) (*CostSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost), 0), COALESCE(SUM(image_count), 0), COUNT(*)
		 FROM cost_log`)

	var summary CostSummary
	if err := row.Scan(&summary.TotalCost, &summary.ImageCount, &summary.EntryCount); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *Store) GetSessionCost(ctx context.Context, sessionID string) (*CostSummary, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost), 0), COALESCE(SUM(image_count), 0), COUNT(*)
		 FROM cost_log WHERE session_id = ?`,
		sessionID)

	var summary CostSummary
	if err := row.Scan(&summary.TotalCost, &summary.ImageCount, &summary.EntryCount); err != nil {
		return nil, err
	}
	return &summary, nil
}
