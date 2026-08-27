package resources

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

type fakeClock struct{ now domain.LogicalTime }

func (c *fakeClock) Now() domain.LogicalTime { return c.now }

func openStore(t *testing.T) persistence.Store {
	t.Helper()
	s, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLeaseContentionAndExpiry(t *testing.T) {
	clock := &fakeClock{now: 100}
	store := openStore(t)
	m := NewManager(store, clock)

	keyA := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}
	keyB := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 2}
	for _, k := range []domain.TaskKey{keyA, keyB} {
		if err := store.CreateTask(domain.MissionTask{Key: k, Phase: domain.PhaseConfigFrozen}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}

	req := AcquireRequest{
		Leases: []LeaseRequest{{ResourceType: domain.ResourceSink, ResourceID: "sink1", Duration: 50}},
	}
	if _, err := m.Acquire(keyA, "idem-a", req); err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	// Task B contends for the same sink while the lease is active.
	_, err := m.Acquire(keyB, "idem-b", req)
	var de *domain.DomainError
	if !errors.As(err, &de) || de.Code != domain.CodeResourceBusy {
		t.Fatalf("want RESOURCE_BUSY, got %v", err)
	}
	// Advance logical time past expiry; B can now acquire.
	clock.now = 200
	if _, err := m.Acquire(keyB, "idem-b", req); err != nil {
		t.Fatalf("acquire B after expiry: %v", err)
	}
}

func TestConcurrentLeaseOnlyOneWins(t *testing.T) {
	clock := &fakeClock{now: 100}
	store := openStore(t)
	m := NewManager(store, clock)

	keys := []domain.TaskKey{
		{VoyageID: "v", LanderID: "L", Generation: 1},
		{VoyageID: "v", LanderID: "L", Generation: 2},
	}
	for _, k := range keys {
		if err := store.CreateTask(domain.MissionTask{Key: k, Phase: domain.PhaseConfigFrozen}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, len(keys))
	for i, k := range keys {
		wg.Add(1)
		go func(i int, k domain.TaskKey) {
			defer wg.Done()
			<-start
			_, errs[i] = m.Acquire(k, "idem-"+string(rune('a'+i)), AcquireRequest{
				Leases: []LeaseRequest{{ResourceType: domain.ResourceSink, ResourceID: "shared-sink", Duration: 100}},
			})
		}(i, k)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one lease winner, got %d", wins)
	}
}
