package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/arbitration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

func TestModel_IdempotencyScopeAndContent(t *testing.T) {
	base := map[string]any{
		"bindings": []map[string]any{{"serial": "S1", "mount_point": "m0"}},
		"leases": []map[string]any{
			{"resource_type": "sink", "resource_id": "sink1", "duration_us": 1000},
			{"resource_type": "reference_clock_port", "resource_id": "clock1", "duration_us": 1000},
		},
	}
	tests := []struct {
		name      string
		target    string
		request   map[string]any
		wantCode  int
		wantError string
		wantPhase string
	}{
		{
			name:      "identical retry for same task returns original success",
			target:    "v:L:1",
			request:   base,
			wantCode:  http.StatusOK,
			wantPhase: "bindings_acquired",
		},
		{
			name:      "same key and content for another task conflicts",
			target:    "v:L:2",
			request:   base,
			wantCode:  http.StatusConflict,
			wantError: "IDEMPOTENCY_CONFLICT",
			wantPhase: "config_frozen",
		},
		{
			name:   "same key with different duration conflicts",
			target: "v:L:1",
			request: map[string]any{
				"bindings": []map[string]any{{"serial": "S1", "mount_point": "m0"}},
				"leases": []map[string]any{
					{"resource_type": "sink", "resource_id": "sink1", "duration_us": 2000},
					{"resource_type": "reference_clock_port", "resource_id": "clock1", "duration_us": 1000},
				},
			},
			wantCode:  http.StatusConflict,
			wantError: "IDEMPOTENCY_CONFLICT",
			wantPhase: "bindings_acquired",
		},
		{
			name:   "same key with different resource set conflicts",
			target: "v:L:1",
			request: map[string]any{
				"bindings": []map[string]any{{"serial": "S1", "mount_point": "m0"}},
				"leases": []map[string]any{
					{"resource_type": "sink", "resource_id": "sink2", "duration_us": 1000},
					{"resource_type": "reference_clock_port", "resource_id": "clock1", "duration_us": 1000},
				},
			},
			wantCode:  http.StatusConflict,
			wantError: "IDEMPOTENCY_CONFLICT",
			wantPhase: "bindings_acquired",
		},
		{
			name:   "same key with different binding conflicts",
			target: "v:L:1",
			request: map[string]any{
				"bindings": []map[string]any{{"serial": "S1", "mount_point": "m1"}},
				"leases": []map[string]any{
					{"resource_type": "sink", "resource_id": "sink1", "duration_us": 1000},
					{"resource_type": "reference_clock_port", "resource_id": "clock1", "duration_us": 1000},
				},
			},
			wantCode:  http.StatusConflict,
			wantError: "IDEMPOTENCY_CONFLICT",
			wantPhase: "bindings_acquired",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			t.Cleanup(func() { store.Close() })
			clock := &testClock{now: 100}
			handler := New(calibration.New(store, clock, testDevice{}), arbitration.New(store, clock), store).Handler()
			do := func(method, path string, body any, headers map[string]string) (int, string) {
				var payload []byte
				if body != nil {
					var err error
					payload, err = json.Marshal(body)
					if err != nil {
						t.Fatalf("marshal request: %v", err)
					}
				}
				req := httptest.NewRequest(method, path, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				for name, value := range headers {
					req.Header.Set(name, value)
				}
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				return recorder.Code, recorder.Body.String()
			}
			for generation := 1; generation <= 2; generation++ {
				key := "v:L:" + string(rune('0'+generation))
				create := map[string]any{"voyage_id": "v", "lander_id": "L", "generation": generation}
				if status, body := do("POST", "/v1/tasks", create, nil); status != http.StatusCreated {
					t.Fatalf("create %s: status=%d body=%s", key, status, body)
				}
				if status, body := do("POST", "/v1/tasks/"+key+"/freeze", freezeBody(), nil); status != http.StatusOK {
					t.Fatalf("freeze %s: status=%d body=%s", key, status, body)
				}
			}

			status, firstBody := do("POST", "/v1/tasks/v:L:1/bindings:acquire", base, map[string]string{"Idempotency-Key": "shared-key"})
			if status != http.StatusOK {
				t.Fatalf("initial acquire: status=%d body=%s", status, firstBody)
			}

			status, secondBody := do("POST", "/v1/tasks/"+tc.target+"/bindings:acquire", tc.request, map[string]string{"Idempotency-Key": "shared-key"})
			if status != tc.wantCode {
				t.Fatalf("second acquire: status=%d want=%d body=%s", status, tc.wantCode, secondBody)
			}
			if tc.wantError == "" {
				var first, second any
				if err := json.Unmarshal([]byte(firstBody), &first); err != nil {
					t.Fatalf("decode initial response: %v", err)
				}
				if err := json.Unmarshal([]byte(secondBody), &second); err != nil {
					t.Fatalf("decode retry response: %v", err)
				}
				if firstJSON, _ := json.Marshal(first); string(firstJSON) != func() string { b, _ := json.Marshal(second); return string(b) }() {
					t.Fatalf("retry did not return original result: first=%s second=%s", firstBody, secondBody)
				}
			} else {
				var envelope struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal([]byte(secondBody), &envelope); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if envelope.Code != tc.wantError {
					t.Fatalf("error code=%q want=%q body=%s", envelope.Code, tc.wantError, secondBody)
				}
			}

			status, taskBody := do("GET", "/v1/tasks/"+tc.target, nil, nil)
			if status != http.StatusOK {
				t.Fatalf("get target task: status=%d body=%s", status, taskBody)
			}
			var task struct {
				Phase string `json:"phase"`
			}
			if err := json.Unmarshal([]byte(taskBody), &task); err != nil {
				t.Fatalf("decode target task: %v", err)
			}
			if task.Phase != tc.wantPhase {
				t.Fatalf("target phase=%q want=%q", task.Phase, tc.wantPhase)
			}
		})
	}
}
