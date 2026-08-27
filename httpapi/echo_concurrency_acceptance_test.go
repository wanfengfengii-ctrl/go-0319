package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/arbitration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/httpapi"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

type modelEchoClock struct{}

func (modelEchoClock) Now() domain.LogicalTime { return 100 }

type modelEchoDevice struct{}

func (modelEchoDevice) Call(kind domain.DeviceKind, attempt int, _ domain.LogicalTime) domain.DeviceResult {
	return domain.DeviceResult{Kind: kind, Attempt: attempt, Valid: true}
}

type modelEchoRaceStore struct {
	persistence.Store
	mu      sync.Mutex
	lookups int
	release chan struct{}
}

func (s *modelEchoRaceStore) ValidReceiveDigest(key domain.TaskKey, epoch int64, transponder string, sequence uint64) (string, bool, error) {
	digest, found, err := s.Store.ValidReceiveDigest(key, epoch, transponder, sequence)
	s.mu.Lock()
	s.lookups++
	lookup := s.lookups
	if lookup == 2 {
		close(s.release)
	}
	s.mu.Unlock()
	if lookup <= 2 {
		select {
		case <-s.release:
		case <-time.After(2 * time.Second):
		}
	}
	return digest, found, err
}

func TestModel_ConcurrentEchoReceiveKeyHasStableHTTPOutcome(t *testing.T) {
	type echo struct {
		Epoch       int64  `json:"epoch"`
		Transponder string `json:"transponder"`
		Sequence    uint64 `json:"sequence"`
		Line        string `json:"line"`
		TransmitUS  int64  `json:"transmit_us"`
		ReceiveUS   int64  `json:"receive_us"`
	}
	tests := []struct {
		name         string
		currentEpoch int64
		concurrent   bool
		echoes       []echo
		wantOutcomes []string
		wantValid    int
	}{
		{
			name:         "identical concurrent retry is accepted and duplicate",
			currentEpoch: 1,
			concurrent:   true,
			echoes: []echo{
				{Epoch: 1, Transponder: "t0", Sequence: 7, Line: "l0", TransmitUS: 10, ReceiveUS: 110},
				{Epoch: 1, Transponder: "t0", Sequence: 7, Line: "l0", TransmitUS: 10, ReceiveUS: 110},
			},
			wantOutcomes: []string{"200:accepted", "200:duplicate"},
			wantValid:    1,
		},
		{
			name:         "different concurrent retry is accepted and sequence conflict",
			currentEpoch: 1,
			concurrent:   true,
			echoes: []echo{
				{Epoch: 1, Transponder: "t0", Sequence: 8, Line: "l0", TransmitUS: 10, ReceiveUS: 110},
				{Epoch: 1, Transponder: "t0", Sequence: 8, Line: "l0", TransmitUS: 10, ReceiveUS: 111},
			},
			wantOutcomes: []string{"200:accepted", "409:SEQUENCE_CONFLICT"},
			wantValid:    1,
		},
		{
			name:         "sequential identical retry remains duplicate",
			currentEpoch: 1,
			echoes: []echo{
				{Epoch: 1, Transponder: "t0", Sequence: 9, Line: "l0", TransmitUS: 10, ReceiveUS: 110},
				{Epoch: 1, Transponder: "t0", Sequence: 9, Line: "l0", TransmitUS: 10, ReceiveUS: 110},
			},
			wantOutcomes: []string{"200:accepted", "200:duplicate"},
			wantValid:    1,
		},
		{
			name:         "old epoch remains late audit evidence",
			currentEpoch: 2,
			echoes: []echo{
				{Epoch: 1, Transponder: "t0", Sequence: 10, Line: "l0", TransmitUS: 10, ReceiveUS: 110},
			},
			wantOutcomes: []string{"200:late"},
			wantValid:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := persistence.Open(filepath.Join(t.TempDir(), "echo.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { _ = raw.Close() })

			key := domain.TaskKey{VoyageID: "voyage", LanderID: "lander", Generation: 1}
			if err := raw.CreateTask(domain.MissionTask{Key: key, Phase: domain.PhaseRanging, CurrentEpoch: tc.currentEpoch, CreatedLogicalTime: 1}); err != nil {
				t.Fatalf("create ranging task: %v", err)
			}

			var store persistence.Store = raw
			if tc.concurrent {
				store = &modelEchoRaceStore{Store: raw, release: make(chan struct{})}
			}
			clock := modelEchoClock{}
			cal := calibration.New(store, clock, modelEchoDevice{})
			handler := httpapi.New(cal, arbitration.New(store, clock), store).Handler()

			outcomes := make([]string, len(tc.echoes))
			post := func(i int) {
				body, err := json.Marshal(tc.echoes[i])
				if err != nil {
					outcomes[i] = "marshal-error:" + err.Error()
					return
				}
				req := httptest.NewRequest("POST", "/v1/tasks/voyage:lander:1/echoes", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				responseBody := recorder.Body.Bytes()
				var envelope struct {
					Status string `json:"status"`
					Code   string `json:"code"`
				}
				if err := json.Unmarshal(responseBody, &envelope); err != nil {
					outcomes[i] = fmt.Sprintf("%d:invalid-json", recorder.Code)
					return
				}
				label := envelope.Status
				if label == "" {
					label = envelope.Code
				}
				outcomes[i] = fmt.Sprintf("%d:%s", recorder.Code, label)
			}

			if tc.concurrent {
				var wg sync.WaitGroup
				start := make(chan struct{})
				for i := range tc.echoes {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						<-start
						post(i)
					}(i)
				}
				close(start)
				wg.Wait()
			} else {
				for i := range tc.echoes {
					post(i)
				}
			}

			sort.Strings(outcomes)
			want := append([]string(nil), tc.wantOutcomes...)
			sort.Strings(want)
			if fmt.Sprint(outcomes) != fmt.Sprint(want) {
				t.Fatalf("HTTP outcomes = %v, want %v", outcomes, want)
			}

			evidence, err := raw.ListEvidence(key)
			if err != nil {
				t.Fatalf("list evidence: %v", err)
			}
			valid := 0
			for _, ev := range evidence {
				if ev.Kind == domain.EvidenceReceive && ev.Valid {
					valid++
				}
			}
			if valid != tc.wantValid {
				t.Fatalf("valid receives = %d, want %d (evidence=%+v)", valid, tc.wantValid, evidence)
			}
		})
	}
}
