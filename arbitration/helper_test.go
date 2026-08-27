package arbitration

import (
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

type fakeClock struct{ now domain.LogicalTime }

func (c *fakeClock) Now() domain.LogicalTime { return c.now }

type fakeDevice struct{ valid bool }

func (d *fakeDevice) Call(kind domain.DeviceKind, attempt int, _ domain.LogicalTime) domain.DeviceResult {
	return domain.DeviceResult{Kind: kind, Attempt: attempt, Valid: d.valid}
}

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

func echoTimes(d int64) (domain.LogicalTime, domain.LogicalTime) {
	return 0, domain.LogicalTime(2*d + 100)
}
