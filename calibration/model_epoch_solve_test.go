package calibration

import (
	"errors"
	"fmt"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

func TestModel_SolveRejectsStaleEpochEvidence(t *testing.T) {
	tests := []struct {
		name         string
		epochReason  string
		currentLines int
		wantSolve    bool
	}{
		{name: "resync without current epoch receives", epochReason: domain.EpochReasonResync},
		{name: "counter wrap without current epoch receives", epochReason: domain.EpochReasonWrap},
		{name: "complete current epoch remains solvable", epochReason: domain.EpochReasonResync, currentLines: 4, wantSolve: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{now: 100}
			s, store := setupService(t, clock, &fakeDevice{valid: true})
			key := domain.TaskKey{VoyageID: "v", LanderID: "L", Generation: 1}
			mustFreezeAndAcquire(t, s, key)

			if _, err := s.DisciplineClock(key); err != nil {
				t.Fatalf("DisciplineClock: %v", err)
			}
			if err := s.ConfirmLoopback(key); err != nil {
				t.Fatalf("ConfirmLoopback: %v", err)
			}

			cfg := testConfig()
			for i, ref := range cfg.ReferencePoints {
				line := fmt.Sprintf("l%d", i)
				if _, err := s.RecordTransmission(key, "t0", line, 0); err != nil {
					t.Fatalf("epoch 1 transmission %s: %v", line, err)
				}
				tx, rx := echoTimes(targetDistance(ref.Coord))
				if outcome, _, err := s.RecordEcho(key, 1, "t0", uint64(i), line, tx, rx); err != nil || outcome != EchoAccepted {
					t.Fatalf("epoch 1 echo %s: outcome=%d err=%v", line, outcome, err)
				}
			}

			if tt.epochReason == domain.EpochReasonResync {
				if _, err := s.ResyncClock(key); err != nil {
					t.Fatalf("ResyncClock: %v", err)
				}
			} else if _, err := s.OpenEpoch(key, tt.epochReason); err != nil {
				t.Fatalf("OpenEpoch(%q): %v", tt.epochReason, err)
			}

			outcome, late, err := s.RecordEcho(key, 1, "t0", 99, "l0", 0, 12345)
			if err != nil {
				t.Fatalf("late epoch 1 echo: %v", err)
			}
			if outcome != EchoLate || late.Valid || late.Kind != domain.EvidenceLate {
				t.Fatalf("late echo = {outcome:%d valid:%v kind:%q}, want late invalid audit evidence", outcome, late.Valid, late.Kind)
			}

			for i := 0; i < tt.currentLines; i++ {
				line := fmt.Sprintf("l%d", i)
				if _, err := s.RecordTransmission(key, "t0", line, 0); err != nil {
					t.Fatalf("epoch 2 transmission %s: %v", line, err)
				}
				tx, rx := echoTimes(targetDistance(cfg.ReferencePoints[i].Coord))
				if outcome, _, err := s.RecordEcho(key, 2, "t0", uint64(i), line, tx, rx); err != nil || outcome != EchoAccepted {
					t.Fatalf("epoch 2 echo %s: outcome=%d err=%v", line, outcome, err)
				}
			}

			res, solveErr := s.Solve(key)
			if tt.wantSolve {
				if solveErr != nil {
					t.Fatalf("Solve with complete current epoch: %v", solveErr)
				}
				if !res.ResidualPassed {
					t.Fatalf("current epoch solve did not pass residuals: %+v", res.Residuals)
				}
			} else {
				var domainErr *domain.DomainError
				if !errors.As(solveErr, &domainErr) || domainErr.Code != domain.CodeInsufficientData {
					t.Fatalf("Solve error = %v, want %s", solveErr, domain.CodeInsufficientData)
				}
			}

			task, err := store.GetTask(key)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if tt.wantSolve {
				if task.Phase != domain.PhaseRecalibrationDone {
					t.Fatalf("phase = %v, want %v", task.Phase, domain.PhaseRecalibrationDone)
				}
				if _, err := store.GetSolve(key); err != nil {
					t.Fatalf("GetSolve after successful solve: %v", err)
				}
			} else {
				if task.Phase != domain.PhaseRanging {
					t.Fatalf("phase = %v, want %v after rejected solve", task.Phase, domain.PhaseRanging)
				}
				if _, err := store.GetSolve(key); err == nil {
					t.Fatal("rejected solve wrote a solve_results row")
				}
			}

			evidence, err := store.ListEvidence(key)
			if err != nil {
				t.Fatalf("ListEvidence: %v", err)
			}
			oldValid, currentValid, lateInvalid := 0, 0, 0
			for _, ev := range evidence {
				switch {
				case ev.Epoch == 1 && ev.Kind == domain.EvidenceReceive && ev.Valid:
					oldValid++
				case ev.Epoch == 2 && ev.Kind == domain.EvidenceReceive && ev.Valid:
					currentValid++
				case ev.Epoch == 1 && ev.Kind == domain.EvidenceLate && !ev.Valid:
					lateInvalid++
				}
			}
			if oldValid != 4 || currentValid != tt.currentLines || lateInvalid != 1 {
				t.Fatalf("evidence counts old-valid=%d current-valid=%d late-invalid=%d, want 4/%d/1", oldValid, currentValid, lateInvalid, tt.currentLines)
			}
		})
	}
}
