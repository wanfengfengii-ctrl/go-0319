package persistence

import "github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"

// appendEvent records one append-only event in the replayable event log.
func (s *SQLiteStore) appendEvent(key domain.TaskKey, kind, payload string, lt domain.LogicalTime) error {
	_, err := s.db.Exec(`
		INSERT INTO event_log(voyage_id, lander_id, generation, kind, payload, logical_time)
		VALUES(?,?,?,?,?,?)`,
		key.VoyageID, key.LanderID, key.Generation, kind, payload, int64(lt))
	return err
}
