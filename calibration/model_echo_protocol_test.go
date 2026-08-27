package calibration

import (
	"errors"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

func TestModel_RecordEchoRequiresRegisteredTransmission(t *testing.T) {
	type testCase struct {
		name string
		run  func(*testing.T, *Service, *persistence.SQLiteStore, domain.TaskKey)
	}

	tests := []testCase{
		{
			name: "current epoch unregistered sequence is rejected",
			run: func(t *testing.T, service *Service, store *persistence.SQLiteStore, key domain.TaskKey) {
				_, _, err := service.RecordEcho(key, 1, "t0", 7, "l0", 0, 100)
				if err == nil {
					t.Fatal("RecordEcho accepted a current-epoch sequence with no matching transmission")
				}

				evidence, listErr := store.ListEvidence(key)
				if listErr != nil {
					t.Fatalf("ListEvidence: %v", listErr)
				}
				for _, ev := range evidence {
					if ev.Epoch == 1 && ev.Transponder == "t0" && ev.Sequence == 7 && ev.Kind == domain.EvidenceReceive && ev.Valid {
						t.Fatalf("unregistered echo became valid receive evidence: %+v", ev)
					}
				}
			},
		},
		{
			name: "registered current epoch echo is accepted",
			run: func(t *testing.T, service *Service, _ *persistence.SQLiteStore, key domain.TaskKey) {
				outcome, ev, err := service.RecordEcho(key, 1, "t0", 0, "l0", 0, 100)
				if err != nil {
					t.Fatalf("RecordEcho: %v", err)
				}
				if outcome != EchoAccepted || ev.Kind != domain.EvidenceReceive || !ev.Valid {
					t.Fatalf("registered echo: outcome=%d evidence=%+v", outcome, ev)
				}
			},
		},
		{
			name: "identical valid receive replay is duplicate",
			run: func(t *testing.T, service *Service, _ *persistence.SQLiteStore, key domain.TaskKey) {
				if outcome, _, err := service.RecordEcho(key, 1, "t0", 0, "l0", 0, 100); err != nil || outcome != EchoAccepted {
					t.Fatalf("initial RecordEcho: outcome=%d err=%v", outcome, err)
				}
				outcome, _, err := service.RecordEcho(key, 1, "t0", 0, "l0", 0, 100)
				if err != nil || outcome != EchoDuplicate {
					t.Fatalf("replayed RecordEcho: outcome=%d err=%v", outcome, err)
				}
			},
		},
		{
			name: "changed valid receive replay conflicts",
			run: func(t *testing.T, service *Service, _ *persistence.SQLiteStore, key domain.TaskKey) {
				if outcome, _, err := service.RecordEcho(key, 1, "t0", 0, "l0", 0, 100); err != nil || outcome != EchoAccepted {
					t.Fatalf("initial RecordEcho: outcome=%d err=%v", outcome, err)
				}
				_, _, err := service.RecordEcho(key, 1, "t0", 0, "l0", 1, 101)
				var domainErr *domain.DomainError
				if !errors.As(err, &domainErr) || domainErr.Code != domain.CodeSequenceConflict {
					t.Fatalf("changed replay error = %v, want %s", err, domain.CodeSequenceConflict)
				}
			},
		},
		{
			name: "old epoch echo remains late audit evidence",
			run: func(t *testing.T, service *Service, _ *persistence.SQLiteStore, key domain.TaskKey) {
				if _, err := service.ResyncClock(key); err != nil {
					t.Fatalf("ResyncClock: %v", err)
				}
				outcome, ev, err := service.RecordEcho(key, 1, "t0", 7, "l0", 0, 100)
				if err != nil {
					t.Fatalf("RecordEcho old epoch: %v", err)
				}
				if outcome != EchoLate || ev.Kind != domain.EvidenceLate || ev.Valid {
					t.Fatalf("old epoch echo: outcome=%d evidence=%+v", outcome, ev)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := &fakeClock{now: 100}
			service, store := setupService(t, clock, &fakeDevice{valid: true})
			key := domain.TaskKey{VoyageID: "protocol", LanderID: "lander", Generation: 1}

			mustFreezeAndAcquire(t, service, key)
			if _, err := service.DisciplineClock(key); err != nil {
				t.Fatalf("DisciplineClock: %v", err)
			}
			if err := service.ConfirmLoopback(key); err != nil {
				t.Fatalf("ConfirmLoopback: %v", err)
			}
			transmission, err := service.RecordTransmission(key, "t0", "l0", 0)
			if err != nil {
				t.Fatalf("RecordTransmission: %v", err)
			}
			if transmission.Epoch != 1 || transmission.Sequence != 0 {
				t.Fatalf("transmission identity = epoch %d sequence %d", transmission.Epoch, transmission.Sequence)
			}

			tc.run(t, service, store, key)
		})
	}
}
