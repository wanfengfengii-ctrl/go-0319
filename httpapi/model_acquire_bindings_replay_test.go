package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestModel_AcquireBindingsReplayAfterStageAdvance(t *testing.T) {
	tests := []struct {
		name             string
		idempotencyKey   string
		changeContent    bool
		wantStatus       int
		wantCode         string
		wantOriginalBody bool
	}{
		{
			name:             "same key and canonical content returns original acquisition",
			idempotencyKey:   "deck-acquire-1",
			wantStatus:       http.StatusOK,
			wantOriginalBody: true,
		},
		{
			name:           "same key with different content remains an idempotency conflict",
			idempotencyKey: "deck-acquire-1",
			changeContent:  true,
			wantStatus:     http.StatusConflict,
			wantCode:       "IDEMPOTENCY_CONFLICT",
		},
		{
			name:       "missing idempotency key remains an invalid request",
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:           "new key after acquisition remains out of order",
			idempotencyKey: "deck-acquire-2",
			wantStatus:     http.StatusConflict,
			wantCode:       "STAGE_OUT_OF_ORDER",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			taskID := "replay:lander:" + string(rune('1'+i))
			if status, body := doJSON(t, srv, http.MethodPost, "/v1/tasks", map[string]any{
				"voyage_id": "replay", "lander_id": "lander", "generation": i + 1,
			}, nil); status != http.StatusCreated {
				t.Fatalf("create task: status=%d body=%s", status, body)
			}
			if status, body := doJSON(t, srv, http.MethodPost, "/v1/tasks/"+taskID+"/freeze", freezeBody(), nil); status != http.StatusOK {
				t.Fatalf("freeze task: status=%d body=%s", status, body)
			}

			acquisition := map[string]any{
				"bindings": []map[string]any{{"serial": "S1", "mount_point": "m0"}},
				"leases": []map[string]any{
					{"resource_type": "sink", "resource_id": "tank-a", "duration_us": 1000},
					{"resource_type": "reference_clock_port", "resource_id": "clock-a", "duration_us": 1000},
				},
			}
			firstStatus, firstBody := doJSON(t, srv, http.MethodPost, "/v1/tasks/"+taskID+"/bindings:acquire", acquisition, map[string]string{
				"Idempotency-Key": "deck-acquire-1",
			})
			if firstStatus != http.StatusOK {
				t.Fatalf("initial acquisition: status=%d body=%s", firstStatus, firstBody)
			}

			retryBody := acquisition
			if tt.changeContent {
				retryBody = map[string]any{
					"bindings": acquisition["bindings"],
					"leases": []map[string]any{
						{"resource_type": "sink", "resource_id": "tank-b", "duration_us": 1000},
						{"resource_type": "reference_clock_port", "resource_id": "clock-a", "duration_us": 1000},
					},
				}
			}
			headers := map[string]string{}
			if tt.idempotencyKey != "" {
				headers["Idempotency-Key"] = tt.idempotencyKey
			}
			status, body := doJSON(t, srv, http.MethodPost, "/v1/tasks/"+taskID+"/bindings:acquire", retryBody, headers)
			if status != tt.wantStatus {
				t.Fatalf("retry status=%d, want %d; body=%s", status, tt.wantStatus, body)
			}

			if tt.wantOriginalBody {
				var first, replay any
				if err := json.Unmarshal([]byte(firstBody), &first); err != nil {
					t.Fatalf("decode initial response: %v", err)
				}
				if err := json.Unmarshal([]byte(body), &replay); err != nil {
					t.Fatalf("decode replay response: %v", err)
				}
				if !reflect.DeepEqual(replay, first) {
					t.Fatalf("replay response=%s, want original response=%s", body, firstBody)
				}
				return
			}

			var response errorResponse
			if err := json.Unmarshal([]byte(body), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != tt.wantCode {
				t.Fatalf("retry code=%q, want %q; body=%s", response.Code, tt.wantCode, body)
			}
		})
	}
}
