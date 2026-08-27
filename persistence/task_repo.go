package persistence

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// CreateTask inserts a new task aggregate. The (voyage, lander, generation)
// key is unique, so a duplicate returns an error.
func (s *SQLiteStore) CreateTask(t domain.MissionTask) error {
	_, err := s.db.Exec(`
		INSERT INTO tasks(voyage_id, lander_id, generation, phase, config_version, frozen_digest, terminal_state, created_at, current_epoch)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		t.Key.VoyageID, t.Key.LanderID, t.Key.Generation, int(t.Phase), t.ConfigVersion, t.FrozenDigest,
		string(t.TerminalState), int64(t.CreatedLogicalTime), t.CurrentEpoch)
	if err != nil {
		return fmt.Errorf("persistence: create task: %w", err)
	}
	return s.appendEvent(t.Key, "task_created", "", t.CreatedLogicalTime)
}

// GetTask loads a task aggregate.
func (s *SQLiteStore) GetTask(key domain.TaskKey) (domain.MissionTask, error) {
	var t domain.MissionTask
	var phase int
	var created, epoch int64
	err := s.db.QueryRow(`
		SELECT phase, config_version, frozen_digest, terminal_state, created_at, current_epoch
		FROM tasks WHERE voyage_id=? AND lander_id=? AND generation=?`,
		key.VoyageID, key.LanderID, key.Generation).
		Scan(&phase, &t.ConfigVersion, &t.FrozenDigest, &t.TerminalState, &created, &epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	t.Key = key
	t.Phase = domain.Phase(phase)
	t.CreatedLogicalTime = domain.LogicalTime(created)
	t.CurrentEpoch = epoch
	return t, nil
}

// AdvancePhase atomically moves a task from one phase to the next. It fails
// when the current phase does not match the expected "from" phase, enforcing
// the strict stage-prefix invariant.
func (s *SQLiteStore) AdvancePhase(key domain.TaskKey, from, to domain.Phase) error {
	res, err := s.db.Exec(`
		UPDATE tasks SET phase=? WHERE voyage_id=? AND lander_id=? AND generation=? AND phase=?`,
		int(to), key.VoyageID, key.LanderID, key.Generation, int(from))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.NewError(domain.CodeStageOutOfOrder, "stage prefix violated")
	}
	return s.appendEvent(key, "phase_advanced", fmt.Sprintf("%d->%d", from, to), 0)
}

// FreezeTask stores the immutable configuration and advances the task to the
// frozen phase in one transaction.
func (s *SQLiteStore) FreezeTask(key domain.TaskKey, cfg catalog.FrozenConfiguration, digest string, version int64) error {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		UPDATE tasks SET phase=?, frozen_digest=?, config_version=?
		WHERE voyage_id=? AND lander_id=? AND generation=? AND phase=?`,
		int(domain.PhaseConfigFrozen), digest, version, key.VoyageID, key.LanderID, key.Generation, int(domain.PhaseCreated))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewError(domain.CodeStageOutOfOrder, "freeze requires created phase")
	}
	if _, err := tx.Exec(`
		INSERT INTO configurations(voyage_id, lander_id, generation, version, payload)
		VALUES(?,?,?,?,?)
		ON CONFLICT(voyage_id, lander_id, generation) DO UPDATE SET version=excluded.version, payload=excluded.payload`,
		key.VoyageID, key.LanderID, key.Generation, version, string(payload)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.appendEvent(key, "config_frozen", digest, 0)
}

// GetConfig loads the frozen configuration payload.
func (s *SQLiteStore) GetConfig(key domain.TaskKey) (catalog.FrozenConfiguration, error) {
	var payload string
	err := s.db.QueryRow(`
		SELECT payload FROM configurations WHERE voyage_id=? AND lander_id=? AND generation=?`,
		key.VoyageID, key.LanderID, key.Generation).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.FrozenConfiguration{}, ErrNotFound
	}
	if err != nil {
		return catalog.FrozenConfiguration{}, err
	}
	var cfg catalog.FrozenConfiguration
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SetCurrentEpoch updates the task's current epoch pointer.
func (s *SQLiteStore) SetCurrentEpoch(key domain.TaskKey, epoch int64) error {
	_, err := s.db.Exec(`
		UPDATE tasks SET current_epoch=? WHERE voyage_id=? AND lander_id=? AND generation=?`,
		epoch, key.VoyageID, key.LanderID, key.Generation)
	return err
}

// CreateEpoch appends a new clock epoch. Epochs are strictly increasing and
// identified by (key, epoch).
func (s *SQLiteStore) CreateEpoch(e domain.ClockEpoch) error {
	_, err := s.db.Exec(`
		INSERT INTO epochs(voyage_id, lander_id, generation, epoch, reason, clock_source, drift_us, start_time, end_time)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		e.Key.VoyageID, e.Key.LanderID, e.Key.Generation, e.Epoch, e.Reason, e.ClockSource,
		e.DriftUS, int64(e.StartTime), int64(e.EndTime))
	if err != nil {
		return err
	}
	return s.appendEvent(e.Key, "epoch_created", e.Reason, e.StartTime)
}

// CurrentEpoch returns the highest epoch for a task.
func (s *SQLiteStore) CurrentEpoch(key domain.TaskKey) (domain.ClockEpoch, error) {
	var e domain.ClockEpoch
	var start, end int64
	err := s.db.QueryRow(`
		SELECT epoch, reason, clock_source, drift_us, start_time, end_time
		FROM epochs WHERE voyage_id=? AND lander_id=? AND generation=?
		ORDER BY epoch DESC LIMIT 1`,
		key.VoyageID, key.LanderID, key.Generation).
		Scan(&e.Epoch, &e.Reason, &e.ClockSource, &e.DriftUS, &start, &end)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	e.Key = key
	e.StartTime = domain.LogicalTime(start)
	e.EndTime = domain.LogicalTime(end)
	return e, nil
}
