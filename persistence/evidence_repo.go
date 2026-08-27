package persistence

import (
	"database/sql"
	"errors"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// AppendEvidence appends one evidence record. The unique index on valid
// receive keys is enforced here at the database level as a backstop to the
// service-layer uniqueness checks.
func (s *SQLiteStore) AppendEvidence(e domain.TimestampEvidence) error {
	valid := 0
	if e.Valid {
		valid = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO evidence(voyage_id, lander_id, generation, transponder, epoch, sequence, line, kind, transmit_us, receive_us, valid, content_digest, recorded_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.Key.VoyageID, e.Key.LanderID, e.Key.Generation, e.Transponder, e.Epoch, e.Sequence, e.Line,
		string(e.Kind), int64(e.TransmitUS), int64(e.ReceiveUS), valid, e.ContentDigest, int64(e.RecordedAt))
	if err != nil {
		return err
	}
	return s.appendEvent(e.Key, "evidence_"+string(e.Kind), e.Line, e.RecordedAt)
}

// ListEvidence returns all evidence for a task in stable
// (epoch, transponder, sequence) order.
func (s *SQLiteStore) ListEvidence(key domain.TaskKey) ([]domain.TimestampEvidence, error) {
	rows, err := s.db.Query(`
		SELECT transponder, epoch, sequence, line, kind, transmit_us, receive_us, valid, content_digest, recorded_at
		FROM evidence WHERE voyage_id=? AND lander_id=? AND generation=?
		ORDER BY epoch, transponder, sequence, kind`, key.VoyageID, key.LanderID, key.Generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TimestampEvidence
	for rows.Next() {
		var e domain.TimestampEvidence
		var valid int
		var tx, rx, rec int64
		if err := rows.Scan(&e.Transponder, &e.Epoch, &e.Sequence, &e.Line, &e.Kind, &tx, &rx, &valid, &e.ContentDigest, &rec); err != nil {
			return nil, err
		}
		e.Key = key
		e.TransmitUS = domain.LogicalTime(tx)
		e.ReceiveUS = domain.LogicalTime(rx)
		e.Valid = valid == 1
		e.RecordedAt = domain.LogicalTime(rec)
		out = append(out, e)
	}
	return out, rows.Err()
}

// HasValidReceive reports whether a valid receive already exists for the given
// transponder, epoch and sequence.
func (s *SQLiteStore) HasValidReceive(key domain.TaskKey, epoch int64, transponder string, seq uint64) (bool, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM evidence
		WHERE voyage_id=? AND lander_id=? AND generation=? AND transponder=? AND epoch=? AND sequence=? AND kind='receive' AND valid=1`,
		key.VoyageID, key.LanderID, key.Generation, transponder, epoch, seq).Scan(&n)
	return n > 0, err
}

// ValidReceiveDigest returns the content digest of an existing valid receive,
// if one exists.
func (s *SQLiteStore) ValidReceiveDigest(key domain.TaskKey, epoch int64, transponder string, seq uint64) (string, bool, error) {
	var digest string
	err := s.db.QueryRow(`
		SELECT content_digest FROM evidence
		WHERE voyage_id=? AND lander_id=? AND generation=? AND transponder=? AND epoch=? AND sequence=? AND kind='receive' AND valid=1`,
		key.VoyageID, key.LanderID, key.Generation, transponder, epoch, seq).Scan(&digest)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return digest, true, nil
}

// NextSequence returns the next transmit sequence number for a transponder in
// an epoch (one greater than the highest already recorded).
func (s *SQLiteStore) NextSequence(key domain.TaskKey, epoch int64, transponder string) (uint64, error) {
	var max sql.NullInt64
	err := s.db.QueryRow(`
		SELECT MAX(sequence) FROM evidence
		WHERE voyage_id=? AND lander_id=? AND generation=? AND transponder=? AND epoch=?`,
		key.VoyageID, key.LanderID, key.Generation, transponder, epoch).Scan(&max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return uint64(max.Int64) + 1, nil
}
