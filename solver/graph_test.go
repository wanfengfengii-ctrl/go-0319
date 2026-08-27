package solver

import (
	"errors"
	"math"
	"testing"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
)

// solveFixture returns a config with four non-coplanar reference points and a
// single transponder at a known coordinate, plus measured integer distances.
func solveFixture() (catalog.FrozenConfiguration, []catalog.BaselineConstraint) {
	cfg := catalog.FrozenConfiguration{
		ResidualThresholdMM: 10,
		ReferencePoints: []catalog.ReferencePoint{
			{ID: "r0", Coord: catalog.Vec3{X: 0, Y: 0, Z: 0}},
			{ID: "r1", Coord: catalog.Vec3{X: 10000, Y: 0, Z: 0}},
			{ID: "r2", Coord: catalog.Vec3{X: 0, Y: 10000, Z: 0}},
			{ID: "r3", Coord: catalog.Vec3{X: 0, Y: 0, Z: 10000}},
		},
		Transponders: []catalog.TransponderSpec{{ID: "t0", Coord: catalog.Vec3{X: 2000, Y: 3000, Z: 4000}}},
	}
	dist := func(r catalog.Vec3) int64 {
		d := r.Sub(catalog.Vec3{X: 2000, Y: 3000, Z: 4000})
		return int64(math.Round(math.Sqrt(float64(d.NormSq()))))
	}
	constraints := []catalog.BaselineConstraint{
		{Reference: "r0", Transponder: "t0", Line: "l0", DistanceMM: dist(cfg.ReferencePoints[0].Coord), Weight: 1},
		{Reference: "r1", Transponder: "t0", Line: "l1", DistanceMM: dist(cfg.ReferencePoints[1].Coord), Weight: 1},
		{Reference: "r2", Transponder: "t0", Line: "l2", DistanceMM: dist(cfg.ReferencePoints[2].Coord), Weight: 1},
		{Reference: "r3", Transponder: "t0", Line: "l3", DistanceMM: dist(cfg.ReferencePoints[3].Coord), Weight: 1},
	}
	return cfg, constraints
}

func TestSolveRecoversCoordinates(t *testing.T) {
	cfg, constraints := solveFixture()
	res, err := Solve(cfg, constraints)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	coord := res.TransponderCoords["t0"]
	if abs64(coord.X-2000) > 15 || abs64(coord.Y-3000) > 15 || abs64(coord.Z-4000) > 15 {
		t.Fatalf("solved coordinate = %+v, want near (2000,3000,4000)", coord)
	}
	if !res.ResidualPassed {
		t.Fatalf("expected residual to pass, residuals=%+v", res.Residuals)
	}
	if len(res.Residuals) != 4 {
		t.Fatalf("expected 4 residuals, got %d", len(res.Residuals))
	}
	if res.AlgorithmVersion != AlgorithmVersion {
		t.Fatalf("algorithm version = %d", res.AlgorithmVersion)
	}
}

func TestSolveInsufficientConstraints(t *testing.T) {
	cfg, constraints := solveFixture()
	cfg.Transponders = []catalog.TransponderSpec{{ID: "t0"}}
	if _, err := Solve(cfg, constraints[:3]); !errors.Is(err, ErrInsufficientConstraints) {
		t.Fatalf("want ErrInsufficientConstraints, got %v", err)
	}
}

func TestSolveArithmeticOverflow(t *testing.T) {
	cfg, _ := solveFixture()
	constraints := []catalog.BaselineConstraint{
		{Reference: "r0", Transponder: "t0", Line: "l0", DistanceMM: 4_000_000_000, Weight: 1},
		{Reference: "r1", Transponder: "t0", Line: "l1", DistanceMM: 4_000_000_000, Weight: 1},
		{Reference: "r2", Transponder: "t0", Line: "l2", DistanceMM: 4_000_000_000, Weight: 1},
		{Reference: "r3", Transponder: "t0", Line: "l3", DistanceMM: 4_000_000_000, Weight: 1},
	}
	if _, err := Solve(cfg, constraints); !errors.Is(err, ErrArithmeticOverflow) {
		t.Fatalf("want ErrArithmeticOverflow, got %v", err)
	}
}

func TestSolveResidualExceeded(t *testing.T) {
	cfg, constraints := solveFixture()
	cfg.ResidualThresholdMM = 1
	// Inject a large error on one line.
	constraints[0].DistanceMM += 500
	res, err := Solve(cfg, constraints)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if res.ResidualPassed {
		t.Fatalf("expected residual to fail")
	}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
