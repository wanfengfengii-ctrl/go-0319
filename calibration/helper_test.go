package calibration

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

// fakeClock is a controllable logical clock for tests.
type fakeClock struct{ now domain.LogicalTime }

func (c *fakeClock) Now() domain.LogicalTime { return c.now }

// fakeDevice is a scripted device adapter with a single configurable outcome.
type fakeDevice struct {
	valid bool
	drift int64
	err   domain.ErrorCode
	calls map[domain.DeviceKind]int
}

func (d *fakeDevice) Call(kind domain.DeviceKind, attempt int, _ domain.LogicalTime) domain.DeviceResult {
	if d.calls == nil {
		d.calls = map[domain.DeviceKind]int{}
	}
	d.calls[kind]++
	return domain.DeviceResult{Kind: kind, Attempt: attempt, Valid: d.valid, DriftUS: d.drift, Err: d.err}
}

// testConfig builds a valid frozen configuration with a single transponder at
// a known coordinate and four non-coplanar references. The sound speed is
// 1_000_000 mm/s so that time-to-distance conversion is exact in tests.
func testConfig() catalog.FrozenConfiguration {
	return catalog.FrozenConfiguration{
		Version:     1,
		ClockSource: "gps",
		ReferencePoints: []catalog.ReferencePoint{
			{ID: "r0", Coord: catalog.Vec3{X: 0, Y: 0, Z: 0}},
			{ID: "r1", Coord: catalog.Vec3{X: 10000, Y: 0, Z: 0}},
			{ID: "r2", Coord: catalog.Vec3{X: 0, Y: 10000, Z: 0}},
			{ID: "r3", Coord: catalog.Vec3{X: 0, Y: 0, Z: 10000}},
		},
		Transponders:  []catalog.TransponderSpec{{ID: "t0", Serial: "S1", MountPoint: "m0", Coord: catalog.Vec3{X: 2000, Y: 3000, Z: 4000}}},
		Profile:       catalog.SoundSpeedProfile{Layers: []catalog.SoundSpeedLayer{{TopMM: 0, BottomMM: 100000, SpeedMMS: 1_000_000}}},
		Slots:         []catalog.Slot{{ID: "s0", StartUS: 0, EndUS: 1000}},
		TransmitCodes: map[string]string{"t0": "code-1"},
		Lines: []catalog.Line{
			{ID: "l0", Reference: "r0", Transponder: "t0"},
			{ID: "l1", Reference: "r1", Transponder: "t0"},
			{ID: "l2", Reference: "r2", Transponder: "t0"},
			{ID: "l3", Reference: "r3", Transponder: "t0"},
		},
		ReviewQualifications: []domain.ReviewQualification{
			{ReviewerID: "alice", ValidUntil: domain.LogicalTime(1 << 40)},
			{ReviewerID: "bob", ValidUntil: domain.LogicalTime(1 << 40)},
		},
		TransducerDelayUS:   100,
		ResidualThresholdMM: 10,
		DriftThresholdUS:    100,
		CounterModulus:      1_000_000,
		SequenceMax:         1000,
		RetryMax:            3,
	}
}

// setupService opens a store and builds a service with the given clock and
// device adapter.
func setupService(t *testing.T, clock *fakeClock, device *fakeDevice) (*Service, *persistence.SQLiteStore) {
	t.Helper()
	store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return New(store, clock, device), store
}

// echoTimes returns (transmit, receive) times that produce the given one-way
// distance with the test config's 1_000_000 mm/s speed and 100 us delay.
func echoTimes(d int64) (domain.LogicalTime, domain.LogicalTime) {
	return 0, domain.LogicalTime(2*d + 100)
}

// targetDistance computes the rounded distance from a reference coordinate to
// the transponder coordinate.
func targetDistance(ref catalog.Vec3) int64 {
	d := ref.Sub(catalog.Vec3{X: 2000, Y: 3000, Z: 4000})
	return int64(math.Round(math.Sqrt(float64(d.NormSq()))))
}
