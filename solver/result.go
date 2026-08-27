package solver

import "github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"

// LineResidual is the per-line reconstruction residual: the difference between
// the measured integer distance and the distance implied by the solved
// coordinate. Residuals are emitted in stable (transponder, line) order.
type LineResidual struct {
	Line        string
	Transponder string
	Reference   string
	ResidualMM  int64
	Weight      int64
}

// SolveResult is the published, recomputable outcome of a joint integer
// baseline solve. Coordinates and residuals are all in integer millimetres and
// are emitted in fixed key order.
type SolveResult struct {
	// TransponderCoords maps each transponder to its solved integer coordinate.
	TransponderCoords map[string]catalog.Vec3
	// Residuals lists every per-line residual in sorted order.
	Residuals []LineResidual
	// LoopResidual is the total absolute closure residual over all lines.
	LoopResidual int64
	// ResidualPassed reports whether every per-line residual is within the
	// configured threshold.
	ResidualPassed bool
	// InputDigest binds the result to the exact inputs that produced it.
	InputDigest string
	// AlgorithmVersion records the published algorithm revision.
	AlgorithmVersion int
}
