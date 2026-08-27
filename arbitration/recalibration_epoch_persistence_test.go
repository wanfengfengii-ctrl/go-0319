package arbitration

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/solver"
)

func TestModel_RecalibrationPersistsClockEpoch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "recalibration.db")
	store, err := persistence.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}
	clock := &fakeClock{now: 200}
	cfg := testConfig()
	cfg.Transponders = append(cfg.Transponders, catalog.TransponderSpec{
		ID: "t1", Serial: "S2", MountPoint: "m1", Coord: catalog.Vec3{X: 2500, Y: 3500, Z: 4500},
	})
	cfg.Lines = []catalog.Line{
		{ID: "z-t1", Reference: "r0", Transponder: "t1"},
		{ID: "l3", Reference: "r3", Transponder: "t0"},
		{ID: "a-t1", Reference: "r3", Transponder: "t1"},
		{ID: "l1", Reference: "r1", Transponder: "t0"},
		{ID: "l0", Reference: "r0", Transponder: "t0"},
		{ID: "l2", Reference: "r2", Transponder: "t0"},
	}

	if err := store.CreateTask(domain.MissionTask{Key: key, Phase: domain.PhaseCreated, CreatedLogicalTime: 100}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := store.FreezeTask(key, cfg, cfg.Digest(), cfg.Version); err != nil {
		t.Fatalf("FreezeTask: %v", err)
	}
	for _, transition := range []struct {
		from domain.Phase
		to   domain.Phase
	}{
		{domain.PhaseConfigFrozen, domain.PhaseBindingsAcquired},
		{domain.PhaseBindingsAcquired, domain.PhaseClockDisciplined},
		{domain.PhaseClockDisciplined, domain.PhaseLoopbackConfirmed},
		{domain.PhaseLoopbackConfirmed, domain.PhaseRanging},
	} {
		if err := store.AdvancePhase(key, transition.from, transition.to); err != nil {
			t.Fatalf("AdvancePhase %v -> %v: %v", transition.from, transition.to, err)
		}
	}
	if err := store.CreateEpoch(domain.ClockEpoch{
		Key: key, Epoch: 1, Reason: domain.EpochReasonInitial, ClockSource: cfg.ClockSource, StartTime: 100, EndTime: 100,
	}); err != nil {
		t.Fatalf("CreateEpoch: %v", err)
	}
	if err := store.SetCurrentEpoch(key, 1); err != nil {
		t.Fatalf("SetCurrentEpoch: %v", err)
	}
	oldEvidence := domain.TimestampEvidence{
		Key: key, Transponder: "t1", Epoch: 1, Sequence: 0, Line: "z-t1", Kind: domain.EvidenceReceive,
		TransmitUS: 10, ReceiveUS: 20, Valid: true, ContentDigest: "old-evidence", RecordedAt: 110,
	}
	if err := store.AppendEvidence(oldEvidence); err != nil {
		t.Fatalf("AppendEvidence: %v", err)
	}
	if err := store.PublishSolve(key, solver.SolveResult{
		Residuals:      []solver.LineResidual{{Line: "z-t1", Transponder: "t1", Reference: "r0", ResidualMM: 11, Weight: 1}},
		ResidualPassed: false,
		InputDigest:    "solve-input",
	}); err != nil {
		t.Fatalf("PublishSolve: %v", err)
	}

	batch, err := New(store, clock).BuildRecalibration(key, "residual_exceeded")
	if err != nil {
		t.Fatalf("BuildRecalibration: %v", err)
	}
	wantTransponders := []string{"t0", "t1"}
	wantLines := []string{"a-t1", "l0", "l1", "l2", "l3", "z-t1"}
	if batch.NewEpoch != 2 {
		t.Fatalf("batch new epoch = %d, want 2", batch.NewEpoch)
	}
	if !reflect.DeepEqual(batch.AffectedTransponders, wantTransponders) {
		t.Fatalf("affected transponders = %v, want %v", batch.AffectedTransponders, wantTransponders)
	}
	if !reflect.DeepEqual(batch.AffectedLines, wantLines) {
		t.Fatalf("affected lines = %v, want %v", batch.AffectedLines, wantLines)
	}
	evidence, err := store.ListEvidence(key)
	if err != nil {
		t.Fatalf("ListEvidence: %v", err)
	}
	if len(evidence) != 1 || evidence[0].ContentDigest != oldEvidence.ContentDigest || evidence[0].Epoch != 1 {
		t.Fatalf("old evidence changed during recalibration: %+v", evidence)
	}

	tests := []struct {
		name    string
		restart bool
	}{
		{name: "current process"},
		{name: "after restart", restart: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.restart {
				if err := store.Close(); err != nil {
					t.Fatalf("Close before restart: %v", err)
				}
				store, err = persistence.Open(dbPath)
				if err != nil {
					t.Fatalf("Open after restart: %v", err)
				}
				if _, err := store.Recover(); err != nil {
					t.Fatalf("Recover: %v", err)
				}
			}

			task, err := store.GetTask(key)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.CurrentEpoch != batch.NewEpoch {
				t.Fatalf("task current epoch = %d, want %d", task.CurrentEpoch, batch.NewEpoch)
			}
			if task.Phase != domain.PhaseRecalibrationDone {
				t.Fatalf("task phase = %v, want %v", task.Phase, domain.PhaseRecalibrationDone)
			}

			epoch, err := calibration.New(store, clock, &fakeDevice{valid: true}).CurrentEpoch(key)
			if err != nil {
				t.Fatalf("CurrentEpoch: %v", err)
			}
			if epoch.Epoch != batch.NewEpoch {
				t.Fatalf("service current epoch = %d, want batch epoch %d", epoch.Epoch, batch.NewEpoch)
			}
			if epoch.Reason != batch.Reason || epoch.ClockSource != cfg.ClockSource {
				t.Fatalf("current epoch metadata = %+v, want reason %q and clock source %q", epoch, batch.Reason, cfg.ClockSource)
			}
		})
	}
}
