package arbitration

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/resources"
)

func TestComputeClosure(t *testing.T) {
	lines := []catalog.Line{
		{ID: "a0", Reference: "r0", Transponder: "t0"},
		{ID: "a1", Reference: "r1", Transponder: "t0"},
		{ID: "b0", Reference: "r0", Transponder: "t1"}, // shares r0 with t0
		{ID: "b1", Reference: "r2", Transponder: "t1"},
		{ID: "c0", Reference: "r3", Transponder: "t2"}, // isolated
	}
	tps, ls := ComputeClosure(lines, []string{"t0"})
	wantTp := []string{"t0", "t1"}
	for i, w := range wantTp {
		if i >= len(tps) || tps[i] != w {
			t.Fatalf("affected transponders = %v, want %v", tps, wantTp)
		}
	}
	if len(ls) != 4 {
		t.Fatalf("affected lines = %v, want 4", ls)
	}
}

// rig drives a task through the solved phase and returns the services.
type rig struct {
	key   domain.TaskKey
	cal   *calibration.Service
	arb   *Service
	store persistence.Store
	clock *fakeClock
}

func newRig(t *testing.T) *rig {
	t.Helper()
	clock := &fakeClock{now: 100}
	dev := &fakeDevice{valid: true}
	store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	cal := calibration.New(store, clock, dev)
	key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}
	r := &rig{key: key, cal: cal, arb: New(store, clock), store: store, clock: clock}

	if _, err := cal.CreateTask(key); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := cal.Freeze(key, testConfig(), 1); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if _, err := cal.AcquireBindings(key, "idem", resources.AcquireRequest{
		Bindings: []resources.BindingRequest{{Serial: "S1", MountPoint: "m0"}},
		Leases:   []resources.LeaseRequest{{ResourceType: domain.ResourceSink, ResourceID: "sink1", Duration: 1000}},
	}); err != nil {
		t.Fatalf("AcquireBindings: %v", err)
	}
	if _, err := cal.DisciplineClock(key); err != nil {
		t.Fatalf("DisciplineClock: %v", err)
	}
	if err := cal.ConfirmLoopback(key); err != nil {
		t.Fatalf("ConfirmLoopback: %v", err)
	}
	for i, ln := range []struct {
		id string
		d  int64
	}{
		{"l0", 5385}, {"l1", 9434}, {"l2", 8307}, {"l3", 7000},
	} {
		if _, err := cal.RecordTransmission(key, "t0", ln.id, 0); err != nil {
			t.Fatalf("transmit: %v", err)
		}
		tx, rx := echoTimes(ln.d)
		if out, _, err := cal.RecordEcho(key, 1, "t0", uint64(i), ln.id, tx, rx); err != nil || out != calibration.EchoAccepted {
			t.Fatalf("echo: out=%d err=%v", out, err)
		}
	}
	if _, err := cal.Solve(key); err != nil {
		t.Fatalf("Solve: %v", err)
	}
	return r
}

func TestReviewRequiresTwoDistinctQualified(t *testing.T) {
	r := newRig(t)
	if _, err := r.arb.AddReview(r.key, "alice"); err != nil {
		t.Fatalf("alice review: %v", err)
	}
	// Same reviewer again is rejected.
	if _, err := r.arb.AddReview(r.key, "alice"); err == nil {
		t.Fatalf("expected duplicate reviewer rejection")
	}
	// Unknown reviewer is rejected.
	if _, err := r.arb.AddReview(r.key, "charlie"); err == nil {
		t.Fatalf("expected unknown reviewer rejection")
	}
	// Second distinct qualified reviewer advances to reviewed.
	if _, err := r.arb.AddReview(r.key, "bob"); err != nil {
		t.Fatalf("bob review: %v", err)
	}
	task, _ := r.store.GetTask(r.key)
	if task.Phase != domain.PhaseReviewed {
		t.Fatalf("phase = %v, want reviewed", task.Phase)
	}
}

func TestTerminalSingleWinner(t *testing.T) {
	r := newRig(t)
	_, _ = r.arb.AddReview(r.key, "alice")
	_, _ = r.arb.AddReview(r.key, "bob")

	if _, err := r.arb.DecideTerminal(r.key, domain.TerminalAdmitted); err != nil {
		t.Fatalf("admit: %v", err)
	}
	// A second decision is rejected.
	_, err := r.arb.DecideTerminal(r.key, domain.TerminalIsolated)
	var de *domain.DomainError
	if !errors.As(err, &de) || de.Code != domain.CodeTerminalAlreadyDecided {
		t.Fatalf("want TERMINAL_ALREADY_DECIDED, got %v", err)
	}
	// Exactly one credential exists.
	cred, err := r.store.GetCredential(r.key)
	if err != nil || cred.Digest == "" {
		t.Fatalf("credential missing: %v", err)
	}
}

func TestTerminalConcurrentBarrier(t *testing.T) {
	r := newRig(t)
	_, _ = r.arb.AddReview(r.key, "alice")
	_, _ = r.arb.AddReview(r.key, "bob")

	states := []domain.TerminalState{domain.TerminalAdmitted, domain.TerminalIsolated, domain.TerminalCancelled}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, len(states))
	for i, st := range states {
		wg.Add(1)
		go func(i int, st domain.TerminalState) {
			defer wg.Done()
			<-start
			_, results[i] = r.arb.DecideTerminal(r.key, st)
		}(i, st)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one terminal decision, got %d", successes)
	}
	term, err := r.store.GetTerminal(r.key)
	if err != nil {
		t.Fatalf("GetTerminal: %v", err)
	}
	if term.State != domain.TerminalAdmitted && term.State != domain.TerminalIsolated && term.State != domain.TerminalCancelled {
		t.Fatalf("unexpected terminal state %q", term.State)
	}
}
