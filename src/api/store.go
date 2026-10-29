package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store is the Run Store boundary (Step 3 ADR-003): the single place run
// state, stage outputs, evidence, results, and feedback are kept. The
// Application Service is the only caller. Two implementations share this
// interface: PostgreSQL (the Run Store in deployment) and an in-memory
// store used only when DATABASE_URL is unset, for local use and tests.
type Store interface {
	CreateRun(ctx context.Context, run *Run) error
	GetRun(ctx context.Context, id string) (*Run, error)
	UpdateRun(ctx context.Context, run *Run) error
	LatestRun(ctx context.Context) (*Run, error)
	SaveFeedback(ctx context.Context, runID string, fb Feedback) error
	// ResetStaleRuns marks runs left "running" by an earlier process as
	// failed, so a restarted server never shows a run as running forever.
	ResetStaleRuns(ctx context.Context) error
}

var errRunNotFound = errors.New("run not found")

func cloneRun(r *Run) *Run {
	if r == nil {
		return nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		// Run contains only JSON-marshalable fields; this cannot happen
		// for a run this package built. Copy by value as a fallback.
		cp := *r
		return &cp
	}
	var out Run
	_ = json.Unmarshal(b, &out)
	return &out
}

// --- In-memory store (local use and tests only) ---

type memoryStore struct {
	mu   sync.Mutex
	runs map[string]*Run
}

func newMemoryStore() *memoryStore { return &memoryStore{runs: map[string]*Run{}} }

func (s *memoryStore) CreateRun(_ context.Context, run *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[run.ID]; exists {
		return fmt.Errorf("run %s already exists", run.ID)
	}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func (s *memoryStore) GetRun(_ context.Context, id string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, errRunNotFound
	}
	return cloneRun(r), nil
}

func (s *memoryStore) UpdateRun(_ context.Context, run *Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.ID]; !ok {
		return errRunNotFound
	}
	s.runs[run.ID] = cloneRun(run)
	return nil
}

func (s *memoryStore) LatestRun(_ context.Context) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *Run
	for _, r := range s.runs {
		if latest == nil || r.CreatedAt.After(latest.CreatedAt) {
			latest = r
		}
	}
	if latest == nil {
		return nil, errRunNotFound
	}
	return cloneRun(latest), nil
}

func (s *memoryStore) SaveFeedback(_ context.Context, runID string, fb Feedback) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errRunNotFound
	}
	r.Feedback = append(r.Feedback, fb)
	r.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *memoryStore) ResetStaleRuns(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.Status == RunStatusRunning {
			r.Status = RunStatusFailed
			r.Error = "run interrupted by server restart"
			r.UpdatedAt = time.Now().UTC()
		}
	}
	return nil
}

// --- PostgreSQL store (the Run Store in deployment) ---

const runStoreSchema = `
CREATE TABLE IF NOT EXISTS runs (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	stage TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	payload JSONB NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS feedback (
	id BIGSERIAL PRIMARY KEY,
	run_id TEXT NOT NULL REFERENCES runs(id),
	opportunity_index INT NOT NULL,
	worked BOOLEAN NOT NULL,
	outcome TEXT NOT NULL DEFAULT '',
	remind_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL
);`

type pgStore struct{ db *sql.DB }

// openRunStore returns the PostgreSQL store when DATABASE_URL is set, and
// fails startup if the database cannot be reached or migrated: a
// configured-but-unreachable Run Store must fail visibly, not silently
// fall back to volatile storage. When DATABASE_URL is unset it returns
// the in-memory store and reports false for postgresInUse.
func openRunStore(ctx context.Context, databaseURL string) (Store, bool, error) {
	if databaseURL == "" {
		log.Print("DATABASE_URL is not set; using in-memory run store (local use only, not durable)")
		return newMemoryStore(), false, nil
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, false, fmt.Errorf("database driver setup failed: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, false, fmt.Errorf("database ping failed: %w", err)
	}
	if _, err := db.ExecContext(ctx, runStoreSchema); err != nil {
		_ = db.Close()
		return nil, false, fmt.Errorf("run store schema migration failed: %w", err)
	}
	return &pgStore{db: db}, true, nil
}

func (s *pgStore) CreateRun(ctx context.Context, run *Run) error {
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO runs (id, status, stage, error, payload, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		run.ID, run.Status, run.Stage, run.Error, payload, run.CreatedAt, run.UpdatedAt)
	return err
}

func (s *pgStore) GetRun(ctx context.Context, id string) (*Run, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT payload FROM runs WHERE id = $1`, id).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, errRunNotFound
	}
	if err != nil {
		return nil, err
	}
	var run Run
	if err := json.Unmarshal(payload, &run); err != nil {
		return nil, err
	}
	// Feedback lives in its own table; attach it so callers see one run.
	fb, err := s.loadFeedback(ctx, id)
	if err != nil {
		return nil, err
	}
	run.Feedback = fb
	return &run, nil
}

func (s *pgStore) loadFeedback(ctx context.Context, runID string) ([]Feedback, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT opportunity_index, worked, outcome, remind_at, created_at
		 FROM feedback WHERE run_id = $1 ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Feedback
	for rows.Next() {
		var fb Feedback
		if err := rows.Scan(&fb.OpportunityIndex, &fb.Worked, &fb.Outcome, &fb.RemindAt, &fb.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, fb)
	}
	return out, rows.Err()
}

func (s *pgStore) UpdateRun(ctx context.Context, run *Run) error {
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = $2, stage = $3, error = $4, payload = $5, updated_at = $6
		 WHERE id = $1`,
		run.ID, run.Status, run.Stage, run.Error, payload, run.UpdatedAt)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errRunNotFound
	}
	return nil
}

func (s *pgStore) LatestRun(ctx context.Context) (*Run, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM runs ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, errRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetRun(ctx, id)
}

func (s *pgStore) SaveFeedback(ctx context.Context, runID string, fb Feedback) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feedback (run_id, opportunity_index, worked, outcome, remind_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		runID, fb.OpportunityIndex, fb.Worked, fb.Outcome, fb.RemindAt, fb.CreatedAt)
	return err
}

func (s *pgStore) ResetStaleRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE runs SET status = $1, error = $2, updated_at = now()
		 WHERE status = $3`, RunStatusFailed, "run interrupted by server restart", RunStatusRunning)
	return err
}
