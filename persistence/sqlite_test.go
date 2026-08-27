package persistence

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

func openTest(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecoverSnapshotDigest(t *testing.T) {
	s := openTest(t)
	if repaired, err := s.Recover(); err != nil || !repaired {
		t.Fatalf("first Recover should repair: repaired=%v err=%v", repaired, err)
	}
	if repaired, err := s.Recover(); err != nil || repaired {
		t.Fatalf("second Recover should not repair: repaired=%v err=%v", repaired, err)
	}
}

func TestTaskRoundTripAndRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	key := domain.TaskKey{VoyageID: "v1", LanderID: "L1", Generation: 1}
	if err := s.CreateTask(domain.MissionTask{Key: key, Phase: domain.PhaseCreated, CreatedLogicalTime: 5}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := s.AdvancePhase(key, domain.PhaseCreated, domain.PhaseConfigFrozen); err != nil {
		t.Fatalf("AdvancePhase: %v", err)
	}
	if err := s.AppendEvidence(domain.TimestampEvidence{
		Key: key, Transponder: "tp0", Epoch: 1, Sequence: 0, Line: "l0",
		Kind: domain.EvidenceReceive, Valid: true, RecordedAt: 6,
	}); err != nil {
		t.Fatalf("AppendEvidence: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify state is intact.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	task, err := s2.GetTask(key)
	if err != nil {
		t.Fatalf("GetTask after restart: %v", err)
	}
	if task.Phase != domain.PhaseConfigFrozen {
		t.Fatalf("phase after restart = %v", task.Phase)
	}
	ev, err := s2.ListEvidence(key)
	if err != nil || len(ev) != 1 {
		t.Fatalf("evidence after restart: %d %v", len(ev), err)
	}
	if !ev[0].Valid {
		t.Fatalf("evidence validity lost after restart")
	}
}

func TestAcquireRollbackOnDuplicateIdentity(t *testing.T) {
	s := openTest(t)
	keyA := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}
	keyB := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 2}
	for _, k := range []domain.TaskKey{keyA, keyB} {
		if err := s.CreateTask(domain.MissionTask{Key: k, Phase: domain.PhaseConfigFrozen}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}

	now := domain.LogicalTime(100)
	acquire := func(key domain.TaskKey, serial string, idem string) error {
		return s.AcquireBindingsAndLeases(key,
			[]domain.TransponderBinding{{Key: key, Serial: serial, MountPoint: "m", BindingGeneration: 1, BoundAt: now}},
			[]domain.ResourceLease{{ResourceType: domain.ResourceSink, ResourceID: "sink1", Key: key, LeaseToken: "t", StartTime: now, EndTime: now + 100, Version: 1}},
			domain.IdempotencyRecord{Key: idem, ContentDigest: idem, ResponseDigest: idem, RecordedAt: now},
			now)
	}

	if err := acquire(keyA, "serial-1", "idem-a"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	err := acquire(keyB, "serial-1", "idem-b")
	var de *domain.DomainError
	if !errors.As(err, &de) || de.Code != domain.CodeDuplicateIdentity {
		t.Fatalf("want DUPLICATE_IDENTITY, got %v", err)
	}

	// The failed second acquire must not have leaked the sink lease.
	lease, err := s.RenewLease("t", now+200)
	if err == nil {
		// The sink should still belong to task A.
		if lease.Key.Generation != 1 {
			t.Fatalf("sink lease leaked to generation %d", lease.Key.Generation)
		}
	}
}

func TestIdempotencyRetryAndConflict(t *testing.T) {
	s := openTest(t)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}
	if err := s.CreateTask(domain.MissionTask{Key: key, Phase: domain.PhaseConfigFrozen}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	now := domain.LogicalTime(100)
	acq := func(idemKey, digest string) error {
		return s.AcquireBindingsAndLeases(key,
			[]domain.TransponderBinding{{Key: key, Serial: "s1", MountPoint: "m", BindingGeneration: 1, BoundAt: now}},
			nil,
			domain.IdempotencyRecord{Key: idemKey, ContentDigest: digest, ResponseDigest: digest, RecordedAt: now},
			now)
	}
	if err := acq("k1", "digest-1"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Identical retry is a no-op success.
	if err := acq("k1", "digest-1"); err != nil {
		t.Fatalf("identical retry should succeed, got %v", err)
	}
	// Different content under the same key conflicts.
	err := acq("k1", "digest-2")
	var de *domain.DomainError
	if !errors.As(err, &de) || de.Code != domain.CodeIdempotencyConflict {
		t.Fatalf("want IDEMPOTENCY_CONFLICT, got %v", err)
	}
}
