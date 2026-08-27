package calibration

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/resources"
)

type transmissionSequenceGate struct {
	persistence.Store

	mu      sync.Mutex
	enabled bool
	waiting int
	release chan struct{}
	once    sync.Once
}

func (s *transmissionSequenceGate) enable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = true
	s.waiting = 0
	s.release = make(chan struct{})
	s.once = sync.Once{}
}

func (s *transmissionSequenceGate) NextSequence(key domain.TaskKey, epoch int64, transponder string) (uint64, error) {
	s.mu.Lock()
	if !s.enabled {
		s.mu.Unlock()
		return s.Store.NextSequence(key, epoch, transponder)
	}
	release := s.release
	s.mu.Unlock()

	sequence, err := s.Store.NextSequence(key, epoch, transponder)

	s.mu.Lock()
	s.waiting++
	if s.waiting == 2 {
		s.once.Do(func() { close(release) })
		s.enabled = false
	} else {
		time.AfterFunc(time.Second, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.once.Do(func() { close(release) })
			s.enabled = false
		})
	}
	s.mu.Unlock()

	<-release
	return sequence, err
}

func TestModel_ConcurrentTransmissionSequenceAllocation(t *testing.T) {
	tests := []struct {
		name        string
		sequenceMax uint64
		seedLines   []string
		wantEpoch   int64
		wantSeqs    []uint64
	}{
		{
			name:        "same epoch allocations are unique and increasing",
			sequenceMax: 1000,
			seedLines:   []string{"l0"},
			wantEpoch:   1,
			wantSeqs:    []uint64{1, 2},
		},
		{
			name:        "concurrent wrap opens one epoch and records against it",
			sequenceMax: 2,
			seedLines:   []string{"l0", "l1"},
			wantEpoch:   2,
			wantSeqs:    []uint64{0, 1},
		},
	}

	for caseIndex, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := &fakeClock{now: 100}
			device := &fakeDevice{valid: true}
			_, store := setupService(t, clock, device)
			gatedStore := &transmissionSequenceGate{Store: store}
			service := New(gatedStore, clock, device)
			key := domain.TaskKey{VoyageID: "concurrent", LanderID: fmt.Sprintf("L%d", caseIndex), Generation: 1}

			if _, err := service.CreateTask(key); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			cfg := testConfig()
			cfg.SequenceMax = tc.sequenceMax
			if _, err := service.Freeze(key, cfg, cfg.Version); err != nil {
				t.Fatalf("Freeze: %v", err)
			}
			if _, err := service.AcquireBindings(key, "idem", resources.AcquireRequest{
				Bindings: []resources.BindingRequest{{Serial: "S1", MountPoint: "m0"}},
				Leases:   []resources.LeaseRequest{{ResourceType: domain.ResourceSink, ResourceID: "sink1", Duration: 1000}},
			}); err != nil {
				t.Fatalf("AcquireBindings: %v", err)
			}
			if _, err := service.DisciplineClock(key); err != nil {
				t.Fatalf("DisciplineClock: %v", err)
			}
			if err := service.ConfirmLoopback(key); err != nil {
				t.Fatalf("ConfirmLoopback: %v", err)
			}

			for i, line := range tc.seedLines {
				ev, err := service.RecordTransmission(key, "t0", line, domain.LogicalTime(i))
				if err != nil {
					t.Fatalf("seed transmission %s: %v", line, err)
				}
				if ev.Epoch != 1 || ev.Sequence != uint64(i) {
					t.Fatalf("seed transmission %s got epoch/sequence %d/%d, want 1/%d", line, ev.Epoch, ev.Sequence, i)
				}
			}

			gatedStore.enable()
			type result struct {
				ev  domain.TimestampEvidence
				err error
			}
			results := make(chan result, 2)
			for i, line := range []string{"l2", "l3"} {
				go func(line string, transmitUS domain.LogicalTime) {
					ev, err := service.RecordTransmission(key, "t0", line, transmitUS)
					results <- result{ev: ev, err: err}
				}(line, domain.LogicalTime(10+i))
			}

			transmissions := make([]domain.TimestampEvidence, 0, 2)
			for i := 0; i < 2; i++ {
				res := <-results
				if res.err != nil {
					t.Fatalf("concurrent transmission %d: %v", i, res.err)
				}
				transmissions = append(transmissions, res.ev)
			}
			sort.Slice(transmissions, func(i, j int) bool { return transmissions[i].Sequence < transmissions[j].Sequence })
			for i, ev := range transmissions {
				if ev.Epoch != tc.wantEpoch || ev.Sequence != tc.wantSeqs[i] {
					t.Fatalf("concurrent allocation %d got epoch/sequence %d/%d, want %d/%d", i, ev.Epoch, ev.Sequence, tc.wantEpoch, tc.wantSeqs[i])
				}
			}

			task, err := store.GetTask(key)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.CurrentEpoch != tc.wantEpoch {
				t.Fatalf("current epoch = %d, want %d", task.CurrentEpoch, tc.wantEpoch)
			}

			for _, ev := range transmissions {
				outcome, _, err := service.RecordEcho(key, ev.Epoch, ev.Transponder, ev.Sequence, ev.Line, ev.TransmitUS, ev.TransmitUS+100)
				if err != nil || outcome != EchoAccepted {
					t.Fatalf("echo for %s: outcome=%d err=%v", ev.Line, outcome, err)
				}
			}
			first := transmissions[0]
			if outcome, _, err := service.RecordEcho(key, first.Epoch, first.Transponder, first.Sequence, first.Line, first.TransmitUS, first.TransmitUS+100); err != nil || outcome != EchoDuplicate {
				t.Fatalf("identical receive: outcome=%d err=%v", outcome, err)
			}

			if tc.wantEpoch > 1 {
				outcome, late, err := service.RecordEcho(key, 1, "t0", 0, "l0", 0, 100)
				if err != nil || outcome != EchoLate || late.Valid || late.Epoch != 1 {
					t.Fatalf("old epoch receive: outcome=%d evidence=%+v err=%v", outcome, late, err)
				}
			}
		})
	}
}
