package persistence

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/solver"
)

// PublishSolve stores the current solve result and advances the task from the
// ranging phase to the solved phase in a single transaction.
func (s *SQLiteStore) PublishSolve(key domain.TaskKey, sr solver.SolveResult) error {
	payload, err := json.Marshal(sr)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`
		UPDATE tasks SET phase=? WHERE voyage_id=? AND lander_id=? AND generation=? AND phase=?`,
		int(domain.PhaseSolved), key.VoyageID, key.LanderID, key.Generation, int(domain.PhaseRanging))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.NewError(domain.CodeStageOutOfOrder, "solve requires ranging phase")
	}
	if _, err := tx.Exec(`
		INSERT INTO solve_results(voyage_id, lander_id, generation, payload) VALUES(?,?,?,?)
		ON CONFLICT(voyage_id, lander_id, generation) DO UPDATE SET payload=excluded.payload`,
		key.VoyageID, key.LanderID, key.Generation, string(payload)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.appendEvent(key, "solve_published", sr.InputDigest, 0)
}

// GetSolve loads the published solve result.
func (s *SQLiteStore) GetSolve(key domain.TaskKey) (solver.SolveResult, error) {
	var payload string
	err := s.db.QueryRow("SELECT payload FROM solve_results WHERE voyage_id=? AND lander_id=? AND generation=?",
		key.VoyageID, key.LanderID, key.Generation).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return solver.SolveResult{}, ErrNotFound
	}
	if err != nil {
		return solver.SolveResult{}, err
	}
	var sr solver.SolveResult
	if err := json.Unmarshal([]byte(payload), &sr); err != nil {
		return sr, err
	}
	return sr, nil
}

// PutRetryCall records a scheduled device retry.
func (s *SQLiteStore) PutRetryCall(rc domain.RetryCall) error {
	_, err := s.db.Exec(`
		INSERT INTO retry_calls(voyage_id, lander_id, generation, device, call_seq, attempt, next_time, last_error)
		VALUES(?,?,?,?,?,?,?,?)`,
		rc.Key.VoyageID, rc.Key.LanderID, rc.Key.Generation, string(rc.Device), rc.CallSeq, rc.Attempt, int64(rc.NextTime), rc.LastError)
	return err
}

// ListRetryCalls returns all pending retry records for a task in stable order.
func (s *SQLiteStore) ListRetryCalls(key domain.TaskKey) ([]domain.RetryCall, error) {
	rows, err := s.db.Query(`
		SELECT device, call_seq, attempt, next_time, last_error FROM retry_calls
		WHERE voyage_id=? AND lander_id=? AND generation=? ORDER BY device, call_seq`,
		key.VoyageID, key.LanderID, key.Generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RetryCall
	for rows.Next() {
		var rc domain.RetryCall
		var next int64
		if err := rows.Scan(&rc.Device, &rc.CallSeq, &rc.Attempt, &next, &rc.LastError); err != nil {
			return nil, err
		}
		rc.Key = key
		rc.NextTime = domain.LogicalTime(next)
		out = append(out, rc)
	}
	return out, rows.Err()
}

// PutRecalibration stores a recalibration batch and its new epoch pointer.
func (s *SQLiteStore) PutRecalibration(b domain.RecalibrationBatch) error {
	tpJSON, _ := json.Marshal(b.AffectedTransponders)
	lineJSON, _ := json.Marshal(b.AffectedLines)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT INTO recalibrations(voyage_id, lander_id, generation, batch_seq, reason, affected_transponders, affected_lines, new_epoch, created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		b.Key.VoyageID, b.Key.LanderID, b.Key.Generation, b.BatchSeq, b.Reason, string(tpJSON), string(lineJSON), b.NewEpoch, int64(b.CreatedAt)); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE tasks SET current_epoch=? WHERE voyage_id=? AND lander_id=? AND generation=?`,
		b.NewEpoch, b.Key.VoyageID, b.Key.LanderID, b.Key.Generation); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.appendEvent(b.Key, "recalibration_created", b.Reason, b.CreatedAt)
}

// LatestRecalibration returns the most recent recalibration batch, if any.
func (s *SQLiteStore) LatestRecalibration(key domain.TaskKey) (domain.RecalibrationBatch, bool, error) {
	var b domain.RecalibrationBatch
	var tpJSON, lineJSON string
	var created int64
	err := s.db.QueryRow(`
		SELECT batch_seq, reason, affected_transponders, affected_lines, new_epoch, created_at
		FROM recalibrations WHERE voyage_id=? AND lander_id=? AND generation=? ORDER BY batch_seq DESC LIMIT 1`,
		key.VoyageID, key.LanderID, key.Generation).
		Scan(&b.BatchSeq, &b.Reason, &tpJSON, &lineJSON, &b.NewEpoch, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return b, false, nil
	}
	if err != nil {
		return b, false, err
	}
	b.Key = key
	b.CreatedAt = domain.LogicalTime(created)
	_ = json.Unmarshal([]byte(tpJSON), &b.AffectedTransponders)
	_ = json.Unmarshal([]byte(lineJSON), &b.AffectedLines)
	return b, true, nil
}

// PutReview records a reviewer's sign-off. The (task, reviewer) key is unique,
// so a repeated review by the same person is rejected.
func (s *SQLiteStore) PutReview(r domain.Review) error {
	_, err := s.db.Exec(`
		INSERT INTO reviews(voyage_id, lander_id, generation, reviewer_id, config_digest, solve_digest, reviewed_at)
		VALUES(?,?,?,?,?,?,?)`,
		r.Key.VoyageID, r.Key.LanderID, r.Key.Generation, r.ReviewerID, r.ConfigDigest, r.SolveDigest, int64(r.ReviewedAt))
	if err != nil {
		return err
	}
	return s.appendEvent(r.Key, "review_submitted", r.ReviewerID, r.ReviewedAt)
}

// ListReviews returns all reviews for a task in stable reviewer order.
func (s *SQLiteStore) ListReviews(key domain.TaskKey) ([]domain.Review, error) {
	rows, err := s.db.Query(`
		SELECT reviewer_id, config_digest, solve_digest, reviewed_at FROM reviews
		WHERE voyage_id=? AND lander_id=? AND generation=? ORDER BY reviewer_id`,
		key.VoyageID, key.LanderID, key.Generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Review
	for rows.Next() {
		var r domain.Review
		var at int64
		if err := rows.Scan(&r.ReviewerID, &r.ConfigDigest, &r.SolveDigest, &at); err != nil {
			return nil, err
		}
		r.Key = key
		r.ReviewedAt = domain.LogicalTime(at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DecideTerminal implements the single-writer terminal barrier. Exactly one
// decision commits per task; later attempts observe the existing decision and
// fail with TERMINAL_ALREADY_DECIDED. An admitted decision also issues the
// unique credential in the same transaction.
func (s *SQLiteStore) DecideTerminal(key domain.TaskKey, barrierSeq int64, d domain.TerminalDecision, cred domain.Credential) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existing string
	err = tx.QueryRow("SELECT state FROM terminal_decisions WHERE voyage_id=? AND lander_id=? AND generation=?",
		key.VoyageID, key.LanderID, key.Generation).Scan(&existing)
	if err == nil {
		return domain.NewError(domain.CodeTerminalAlreadyDecided, "terminal already decided: "+existing)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO terminal_decisions(voyage_id, lander_id, generation, barrier_seq, state, credential_digest, decided_at)
		VALUES(?,?,?,?,?,?,?)`,
		key.VoyageID, key.LanderID, key.Generation, barrierSeq, string(d.State), d.CredentialDigest, int64(d.DecidedAt)); err != nil {
		return err
	}
	if d.State == domain.TerminalAdmitted {
		if _, err := tx.Exec(`
			INSERT INTO credentials(voyage_id, lander_id, generation, digest, issued_at)
			VALUES(?,?,?,?,?)`,
			key.VoyageID, key.LanderID, key.Generation, cred.Digest, int64(cred.IssuedAt)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE tasks SET terminal_state=?, phase=? WHERE voyage_id=? AND lander_id=? AND generation=?`,
		string(d.State), int(domain.PhaseTerminal), key.VoyageID, key.LanderID, key.Generation); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.appendEvent(key, "terminal_decided", string(d.State), d.DecidedAt)
}

// GetTerminal loads the committed terminal decision.
func (s *SQLiteStore) GetTerminal(key domain.TaskKey) (domain.TerminalDecision, error) {
	var d domain.TerminalDecision
	var at int64
	err := s.db.QueryRow(`
		SELECT barrier_seq, state, credential_digest, decided_at FROM terminal_decisions
		WHERE voyage_id=? AND lander_id=? AND generation=?`,
		key.VoyageID, key.LanderID, key.Generation).Scan(&d.BarrierSeq, &d.State, &d.CredentialDigest, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	d.Key = key
	d.DecidedAt = domain.LogicalTime(at)
	return d, nil
}

// GetCredential loads the issued admission credential.
func (s *SQLiteStore) GetCredential(key domain.TaskKey) (domain.Credential, error) {
	var c domain.Credential
	var at int64
	err := s.db.QueryRow("SELECT digest, issued_at FROM credentials WHERE voyage_id=? AND lander_id=? AND generation=?",
		key.VoyageID, key.LanderID, key.Generation).Scan(&c.Digest, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.Key = key
	c.IssuedAt = domain.LogicalTime(at)
	return c, nil
}
