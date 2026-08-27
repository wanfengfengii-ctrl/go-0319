package calibration

import (
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/resources"
)

func TestPhasePrefixRejected(t *testing.T) {
	clock := &fakeClock{now: 100}
	dev := &fakeDevice{valid: true}
	s, _ := setupService(t, clock, dev)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}

	if _, err := s.CreateTask(key); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	// Binding before freeze must be rejected.
	if _, err := s.AcquireBindings(key, "k", resources.AcquireRequest{}); err == nil {
		t.Fatalf("expected STAGE_OUT_OF_ORDER for bind-before-freeze")
	}
	// Echo before ranging must be rejected.
	if _, _, err := s.RecordEcho(key, 1, "t0", 0, "l0", 0, 10); err == nil {
		t.Fatalf("expected STAGE_OUT_OF_ORDER for echo-before-ranging")
	}
	// Solve before ranging must be rejected.
	if _, err := s.Solve(key); err == nil {
		t.Fatalf("expected STAGE_OUT_OF_ORDER for solve-before-ranging")
	}
}

func TestEpochIsolation(t *testing.T) {
	clock := &fakeClock{now: 100}
	dev := &fakeDevice{valid: true}
	s, store := setupService(t, clock, dev)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}

	mustFreezeAndAcquire(t, s, key)

	if _, err := s.DisciplineClock(key); err != nil {
		t.Fatalf("DisciplineClock: %v", err)
	}
	if err := s.ConfirmLoopback(key); err != nil {
		t.Fatalf("ConfirmLoopback: %v", err)
	}
	// Record a transmission in epoch 1.
	if _, err := s.RecordTransmission(key, "t0", "l0", 0); err != nil {
		t.Fatalf("RecordTransmission: %v", err)
	}
	// Resync opens epoch 2.
	if _, err := s.ResyncClock(key); err != nil {
		t.Fatalf("ResyncClock: %v", err)
	}
	task, _ := store.GetTask(key)
	if task.CurrentEpoch != 2 {
		t.Fatalf("current epoch = %d, want 2", task.CurrentEpoch)
	}
	// An echo for the old epoch 1 must be recorded only as late audit evidence.
	outcome, ev, err := s.RecordEcho(key, 1, "t0", 0, "l0", 0, 10)
	if err != nil {
		t.Fatalf("RecordEcho late: %v", err)
	}
	if outcome != EchoLate || ev.Valid {
		t.Fatalf("want late invalid evidence, got outcome=%d valid=%v", outcome, ev.Valid)
	}
}

func TestSequenceDuplicateAndConflict(t *testing.T) {
	clock := &fakeClock{now: 100}
	dev := &fakeDevice{valid: true}
	s, _ := setupService(t, clock, dev)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}

	mustFreezeAndAcquire(t, s, key)
	if _, err := s.DisciplineClock(key); err != nil {
		t.Fatalf("DisciplineClock: %v", err)
	}
	if err := s.ConfirmLoopback(key); err != nil {
		t.Fatalf("ConfirmLoopback: %v", err)
	}
	if _, err := s.RecordTransmission(key, "t0", "l0", 0); err != nil {
		t.Fatalf("RecordTransmission: %v", err)
	}

	// First valid receive.
	if outcome, _, err := s.RecordEcho(key, 1, "t0", 0, "l0", 0, 100); err != nil || outcome != EchoAccepted {
		t.Fatalf("first echo: outcome=%d err=%v", outcome, err)
	}
	// Identical duplicate.
	if outcome, _, err := s.RecordEcho(key, 1, "t0", 0, "l0", 0, 100); err != nil || outcome != EchoDuplicate {
		t.Fatalf("duplicate echo: outcome=%d err=%v", outcome, err)
	}
	// Different content conflict.
	if _, _, err := s.RecordEcho(key, 1, "t0", 0, "l0", 5, 200); err == nil {
		t.Fatalf("expected SEQUENCE_CONFLICT")
	}
}

func TestDeviceFailureRecordsRetriesAndStays(t *testing.T) {
	clock := &fakeClock{now: 100}
	dev := &fakeDevice{valid: false, err: domain.CodeDeviceTimeout}
	s, store := setupService(t, clock, dev)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}

	mustFreezeAndAcquire(t, s, key)
	if _, err := s.DisciplineClock(key); err == nil {
		t.Fatalf("expected device failure")
	}
	task, _ := store.GetTask(key)
	if task.Phase != domain.PhaseBindingsAcquired {
		t.Fatalf("phase advanced on device failure: %v", task.Phase)
	}
	calls, _ := store.ListRetryCalls(key)
	if len(calls) != 1 {
		t.Fatalf("expected 1 retry call, got %d", len(calls))
	}
}

func TestFullWorkflowToCredential(t *testing.T) {
	clock := &fakeClock{now: 100}
	dev := &fakeDevice{valid: true}
	s, store := setupService(t, clock, dev)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}

	if _, err := s.CreateTask(key); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	cfg := testConfig()
	digest, err := s.Freeze(key, cfg, 1)
	if err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if _, err := s.AcquireBindings(key, "idem-1", resources.AcquireRequest{
		Bindings: []resources.BindingRequest{{Serial: "S1", MountPoint: "m0"}},
		Leases: []resources.LeaseRequest{
			{ResourceType: domain.ResourceSink, ResourceID: "sink1", Duration: 1000},
			{ResourceType: domain.ResourceReferenceClockPort, ResourceID: "clk1", Duration: 1000},
		},
	}); err != nil {
		t.Fatalf("AcquireBindings: %v", err)
	}
	if _, err := s.DisciplineClock(key); err != nil {
		t.Fatalf("DisciplineClock: %v", err)
	}
	if err := s.ConfirmLoopback(key); err != nil {
		t.Fatalf("ConfirmLoopback: %v", err)
	}

	// Transmit + echo for all four lines.
	lines := []struct {
		id string
		d  int64
	}{
		{"l0", targetDistance(cfg.ReferencePoints[0].Coord)},
		{"l1", targetDistance(cfg.ReferencePoints[1].Coord)},
		{"l2", targetDistance(cfg.ReferencePoints[2].Coord)},
		{"l3", targetDistance(cfg.ReferencePoints[3].Coord)},
	}
	for i, ln := range lines {
		if _, err := s.RecordTransmission(key, "t0", ln.id, 0); err != nil {
			t.Fatalf("transmit %s: %v", ln.id, err)
		}
		tx, rx := echoTimes(ln.d)
		if outcome, _, err := s.RecordEcho(key, 1, "t0", uint64(i), ln.id, tx, rx); err != nil || outcome != EchoAccepted {
			t.Fatalf("echo %s: outcome=%d err=%v", ln.id, outcome, err)
		}
	}

	res, err := s.Solve(key)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !res.ResidualPassed {
		t.Fatalf("residual should pass, residuals=%+v", res.Residuals)
	}
	task, _ := store.GetTask(key)
	if task.Phase != domain.PhaseRecalibrationDone {
		t.Fatalf("phase = %v, want recalibration_done", task.Phase)
	}
	if task.FrozenDigest != digest {
		t.Fatalf("frozen digest mismatch")
	}
}

// mustFreezeAndAcquire drives a task through freeze and bind for tests that
// only exercise later phases.
func mustFreezeAndAcquire(t *testing.T, s *Service, key domain.TaskKey) {
	t.Helper()
	if _, err := s.CreateTask(key); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := s.Freeze(key, testConfig(), 1); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if _, err := s.AcquireBindings(key, "idem", resources.AcquireRequest{
		Bindings: []resources.BindingRequest{{Serial: "S1", MountPoint: "m0"}},
		Leases: []resources.LeaseRequest{
			{ResourceType: domain.ResourceSink, ResourceID: "sink1", Duration: 1000},
		},
	}); err != nil {
		t.Fatalf("AcquireBindings: %v", err)
	}
}
