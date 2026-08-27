package persistence

import (
	"database/sql"
	"errors"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// AcquireBindingsAndLeases atomically binds transponder identities and leases
// resources for one task, honouring idempotency, active-identity uniqueness and
// unexpired-resource exclusivity. All-or-nothing: any conflict rolls the whole
// transaction back.
func (s *SQLiteStore) AcquireBindingsAndLeases(key domain.TaskKey, bindings []domain.TransponderBinding, leases []domain.ResourceLease, idem domain.IdempotencyRecord, now domain.LogicalTime) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Idempotency short-circuit.
	var existingDigest string
	err = tx.QueryRow("SELECT content_digest FROM idempotency WHERE idem_key=?", idem.Key).Scan(&existingDigest)
	if err == nil {
		if existingDigest != idem.ContentDigest {
			return domain.NewError(domain.CodeIdempotencyConflict, "idempotency key reused with different content")
		}
		return nil // identical retry: original result stands.
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// Active identity uniqueness.
	for _, b := range bindings {
		var v, l string
		var g int
		err := tx.QueryRow("SELECT voyage_id, lander_id, generation FROM bindings WHERE serial=?", b.Serial).
			Scan(&v, &l, &g)
		if err == nil {
			if v != key.VoyageID || l != key.LanderID || g != key.Generation {
				return domain.NewError(domain.CodeDuplicateIdentity, "transponder serial already bound: "+b.Serial)
			}
			continue // already bound by this task (idempotent path).
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO bindings(serial, voyage_id, lander_id, generation, mount_point, binding_generation, bound_at)
			VALUES(?,?,?,?,?,?,?)`,
			b.Serial, key.VoyageID, key.LanderID, key.Generation, b.MountPoint, b.BindingGeneration, int64(b.BoundAt)); err != nil {
			return err
		}
	}

	// Unexpired resource exclusivity.
	for _, lease := range leases {
		var v, l string
		var g int
		var end int64
		err := tx.QueryRow(`
			SELECT voyage_id, lander_id, generation, end_time
			FROM resource_leases WHERE resource_type=? AND resource_id=?`,
			lease.ResourceType, lease.ResourceID).Scan(&v, &l, &g, &end)
		if err == nil {
			if domain.LogicalTime(end) > now {
				if v != key.VoyageID || l != key.LanderID || g != key.Generation {
					return domain.NewError(domain.CodeResourceBusy, "resource busy: "+string(lease.ResourceType)+"/"+lease.ResourceID)
				}
				continue // already leased by this task.
			}
			// Expired: fall through and overwrite.
			if _, err := tx.Exec(`
				UPDATE resource_leases SET voyage_id=?, lander_id=?, generation=?, lease_token=?, start_time=?, end_time=?, version=?
				WHERE resource_type=? AND resource_id=?`,
				key.VoyageID, key.LanderID, key.Generation, lease.LeaseToken, int64(lease.StartTime), int64(lease.EndTime), lease.Version,
				lease.ResourceType, lease.ResourceID); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO resource_leases(resource_type, resource_id, voyage_id, lander_id, generation, lease_token, start_time, end_time, version)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			lease.ResourceType, lease.ResourceID, key.VoyageID, key.LanderID, key.Generation, lease.LeaseToken,
			int64(lease.StartTime), int64(lease.EndTime), lease.Version); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO idempotency(idem_key, content_digest, response_digest, recorded_at)
		VALUES(?,?,?,?)`,
		idem.Key, idem.ContentDigest, idem.ResponseDigest, int64(idem.RecordedAt)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return s.appendEvent(key, "bindings_acquired", idem.Key, now)
}

// RenewLease extends a lease's expiry under its token, returning the updated
// lease. An expired or unknown lease fails.
func (s *SQLiteStore) RenewLease(token string, until domain.LogicalTime) (domain.ResourceLease, error) {
	res, err := s.db.Exec("UPDATE resource_leases SET end_time=?, version=version+1 WHERE lease_token=? AND end_time > ?",
		int64(until), token, int64(until))
	if err != nil {
		return domain.ResourceLease{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ResourceLease{}, domain.NewError(domain.CodeLeaseExpired, "lease expired or unknown")
	}
	var l domain.ResourceLease
	var v, l2 string
	var g int
	var start, end int64
	err = s.db.QueryRow(`
		SELECT resource_type, resource_id, voyage_id, lander_id, generation, start_time, end_time, version
		FROM resource_leases WHERE lease_token=?`, token).
		Scan(&l.ResourceType, &l.ResourceID, &v, &l2, &g, &start, &end, &l.Version)
	if err != nil {
		return l, err
	}
	l.Key = domain.TaskKey{VoyageID: v, LanderID: l2, Generation: g}
	l.LeaseToken = token
	l.StartTime = domain.LogicalTime(start)
	l.EndTime = domain.LogicalTime(end)
	return l, nil
}

// GetIdempotency loads an idempotency record.
func (s *SQLiteStore) GetIdempotency(idemKey string) (domain.IdempotencyRecord, error) {
	var r domain.IdempotencyRecord
	var rec int64
	err := s.db.QueryRow("SELECT idem_key, content_digest, response_digest, recorded_at FROM idempotency WHERE idem_key=?", idemKey).
		Scan(&r.Key, &r.ContentDigest, &r.ResponseDigest, &rec)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.RecordedAt = domain.LogicalTime(rec)
	return r, nil
}
