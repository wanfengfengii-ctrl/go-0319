package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/arbitration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/calibration"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

type testClock struct{ now domain.LogicalTime }

func (c *testClock) Now() domain.LogicalTime { return c.now }

type testDevice struct{}

func (testDevice) Call(kind domain.DeviceKind, attempt int, _ domain.LogicalTime) domain.DeviceResult {
	return domain.DeviceResult{Kind: kind, Attempt: attempt, Valid: true}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	clock := &testClock{now: 100}
	cal := calibration.New(store, clock, testDevice{})
	arb := arbitration.New(store, clock)
	srv := httptest.NewServer(New(cal, arb, store).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any, headers map[string]string) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, rd)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func freezeBody() map[string]any {
	return map[string]any{
		"version":      1,
		"clock_source": "gps",
		"reference_points": []map[string]any{
			{"id": "r0", "x": 0, "y": 0, "z": 0},
			{"id": "r1", "x": 10000, "y": 0, "z": 0},
			{"id": "r2", "x": 0, "y": 10000, "z": 0},
			{"id": "r3", "x": 0, "y": 0, "z": 10000},
		},
		"transponders": []map[string]any{
			{"id": "t0", "serial": "S1", "mount_point": "m0", "x": 2000, "y": 3000, "z": 4000},
		},
		"profile":        []map[string]any{{"top_mm": 0, "bottom_mm": 100000, "speed_mm_s": 1000000}},
		"slots":          []map[string]any{{"id": "s0", "start_us": 0, "end_us": 1000}},
		"transmit_codes": map[string]string{"t0": "code-1"},
		"lines": []map[string]any{
			{"id": "l0", "reference": "r0", "transponder": "t0"},
			{"id": "l1", "reference": "r1", "transponder": "t0"},
			{"id": "l2", "reference": "r2", "transponder": "t0"},
			{"id": "l3", "reference": "r3", "transponder": "t0"},
		},
		"review_qualifications": []map[string]any{
			{"reviewer_id": "alice", "valid_until": 1 << 40},
			{"reviewer_id": "bob", "valid_until": 1 << 40},
		},
		"transducer_delay_us":   100,
		"residual_threshold_mm": 10,
		"drift_threshold_us":    100,
		"counter_modulus":       1000000,
		"sequence_max":          1000,
		"retry_max":             3,
	}
}

func dist(ref catalog.Vec3) int64 {
	d := ref.Sub(catalog.Vec3{X: 2000, Y: 3000, Z: 4000})
	return int64(math.Round(math.Sqrt(float64(d.NormSq()))))
}

func TestFullHTTPWorkflow(t *testing.T) {
	srv := newTestServer(t)
	key := "v:L:1"

	if status, _ := doJSON(t, srv, "GET", "/healthz", nil, nil); status != 200 {
		t.Fatalf("healthz status = %d", status)
	}
	if status, _ := doJSON(t, srv, "POST", "/v1/tasks", map[string]any{"voyage_id": "v", "lander_id": "L", "generation": 1}, nil); status != 201 {
		t.Fatalf("create task status = %d", status)
	}
	if status, body := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/freeze", freezeBody(), nil); status != 200 {
		t.Fatalf("freeze status = %d body=%s", status, body)
	}
	bind := map[string]any{
		"bindings": []map[string]any{{"serial": "S1", "mount_point": "m0"}},
		"leases": []map[string]any{
			{"resource_type": "sink", "resource_id": "sink1", "duration_us": 1000},
			{"resource_type": "reference_clock_port", "resource_id": "clk1", "duration_us": 1000},
		},
	}
	if status, body := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/bindings:acquire", bind, map[string]string{"Idempotency-Key": "idem-1"}); status != 200 {
		t.Fatalf("acquire status = %d body=%s", status, body)
	}
	if status, _ := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/clock:discipline", map[string]any{}, nil); status != 200 {
		t.Fatalf("discipline status = %d", status)
	}
	if status, _ := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/loopback:confirm", map[string]any{}, nil); status != 200 {
		t.Fatalf("loopback status = %d", status)
	}

	refs := []catalog.ReferencePoint{
		{Coord: catalog.Vec3{X: 0, Y: 0, Z: 0}},
		{Coord: catalog.Vec3{X: 10000, Y: 0, Z: 0}},
		{Coord: catalog.Vec3{X: 0, Y: 10000, Z: 0}},
		{Coord: catalog.Vec3{X: 0, Y: 0, Z: 10000}},
	}
	for i := 0; i < 4; i++ {
		line := fmt.Sprintf("l%d", i)
		if status, _ := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/transmissions", map[string]any{"transponder": "t0", "line": line, "transmit_us": 0}, nil); status != 200 {
			t.Fatalf("transmit %d status = %d", i, status)
		}
		d := dist(refs[i].Coord)
		echo := map[string]any{"epoch": 1, "transponder": "t0", "sequence": i, "line": line, "transmit_us": 0, "receive_us": 2*d + 100}
		if status, body := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/echoes", echo, nil); status != 200 {
			t.Fatalf("echo %d status = %d body=%s", i, status, body)
		}
	}

	if status, body := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/solve", map[string]any{}, nil); status != 200 {
		t.Fatalf("solve status = %d body=%s", status, body)
	}
	if status, _ := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/reviews", map[string]any{"reviewer_id": "alice"}, nil); status != 200 {
		t.Fatalf("review alice status = %d", status)
	}
	if status, _ := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/reviews", map[string]any{"reviewer_id": "bob"}, nil); status != 200 {
		t.Fatalf("review bob status = %d", status)
	}
	if status, body := doJSON(t, srv, "POST", "/v1/tasks/"+key+"/terminal-decisions", map[string]any{"state": "admitted"}, nil); status != 200 {
		t.Fatalf("terminal status = %d body=%s", status, body)
	}
	status, body := doJSON(t, srv, "GET", "/v1/tasks/"+key+"/credential", nil, nil)
	if status != 200 {
		t.Fatalf("credential status = %d body=%s", status, body)
	}
	var cred map[string]any
	_ = json.Unmarshal([]byte(body), &cred)
	if cred["credential_digest"] == "" {
		t.Fatalf("empty credential digest: %s", body)
	}
}
