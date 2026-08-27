package solver

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// Solve failures.
var (
	ErrInsufficientConstraints = errors.New("solver: insufficient independent constraints")
	ErrUnsolvable              = errors.New("solver: linear system is singular")
)

// Solve performs a weighted integer baseline solve for every transponder in
// the frozen configuration. For each transponder it builds a linearised
// multilateration system from its measured distances to the reference points,
// solves a 3x3 least-squares normal system, rounds the coordinates to integer
// millimetres and computes per-line residuals in integer millimetres. Any
// intermediate multiplication overflow aborts with ErrArithmeticOverflow.
func Solve(config catalog.FrozenConfiguration, constraints []catalog.BaselineConstraint) (SolveResult, error) {
	refs := make(map[string]catalog.Vec3, len(config.ReferencePoints))
	for _, r := range config.ReferencePoints {
		refs[r.ID] = r.Coord
	}

	byTransponder := make(map[string][]catalog.BaselineConstraint)
	for _, c := range constraints {
		byTransponder[c.Transponder] = append(byTransponder[c.Transponder], c)
	}

	res := SolveResult{
		TransponderCoords: make(map[string]catalog.Vec3, len(config.Transponders)),
		AlgorithmVersion:  AlgorithmVersion,
	}

	var allResiduals []LineResidual
	var loopResidual int64

	for _, tp := range config.Transponders {
		cs := byTransponder[tp.ID]
		if len(cs) < 4 {
			return SolveResult{}, fmt.Errorf("%w: transponder %q has %d constraints", ErrInsufficientConstraints, tp.ID, len(cs))
		}
		coord, err := solveTransponder(cs, refs)
		if err != nil {
			return SolveResult{}, err
		}
		res.TransponderCoords[tp.ID] = coord

		for _, c := range cs {
			r := refs[c.Reference]
			recon := roundDist(coord, r)
			residual := c.DistanceMM - recon
			if residual < 0 {
				residual = -residual
			}
			allResiduals = append(allResiduals, LineResidual{
				Line:        c.Line,
				Transponder: c.Transponder,
				Reference:   c.Reference,
				ResidualMM:  residual,
				Weight:      c.Weight,
			})
			loopResidual += residual
		}
	}

	sort.Slice(allResiduals, func(i, j int) bool {
		if allResiduals[i].Transponder != allResiduals[j].Transponder {
			return allResiduals[i].Transponder < allResiduals[j].Transponder
		}
		return allResiduals[i].Line < allResiduals[j].Line
	})

	res.Residuals = allResiduals
	res.LoopResidual = loopResidual
	res.ResidualPassed = true
	for _, r := range allResiduals {
		if r.ResidualMM > config.ResidualThresholdMM {
			res.ResidualPassed = false
			break
		}
	}
	res.InputDigest = solveInputDigest(config, constraints)
	return res, nil
}

// solveTransponder computes one transponder's integer coordinate from its
// distance constraints to reference points.
func solveTransponder(cs []catalog.BaselineConstraint, refs map[string]catalog.Vec3) (catalog.Vec3, error) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Reference < cs[j].Reference })

	base := refs[cs[0].Reference]
	var a [][]float64
	var b []float64

	d0 := cs[0].DistanceMM
	d0sq, ok := domain.Mul(d0, d0)
	if !ok {
		return catalog.Vec3{}, ErrArithmeticOverflow
	}
	r0sq, err := normSq(base)
	if err != nil {
		return catalog.Vec3{}, err
	}

	for _, c := range cs[1:] {
		ri := refs[c.Reference]
		disq, ok := domain.Mul(c.DistanceMM, c.DistanceMM)
		if !ok {
			return catalog.Vec3{}, ErrArithmeticOverflow
		}
		risq, err := normSq(ri)
		if err != nil {
			return catalog.Vec3{}, err
		}
		// b_i = d0^2 - di^2 + |Ri|^2 - |R0|^2
		bi, ok := domain.Sub(d0sq, disq)
		if !ok {
			return catalog.Vec3{}, ErrArithmeticOverflow
		}
		bi, ok = domain.Add(bi, risq)
		if !ok {
			return catalog.Vec3{}, ErrArithmeticOverflow
		}
		bi, ok = domain.Sub(bi, r0sq)
		if !ok {
			return catalog.Vec3{}, ErrArithmeticOverflow
		}

		// row: 2*(Ri - R0) · P = bi
		dx := 2 * (ri.X - base.X)
		dy := 2 * (ri.Y - base.Y)
		dz := 2 * (ri.Z - base.Z)
		a = append(a, []float64{float64(dx), float64(dy), float64(dz)})
		b = append(b, float64(bi))
	}

	if len(a) < 3 {
		return catalog.Vec3{}, ErrInsufficientConstraints
	}

	x, ok := leastSquares3(a, b)
	if !ok {
		return catalog.Vec3{}, ErrUnsolvable
	}

	return catalog.Vec3{
		X: roundAwayFromZero(x[0]),
		Y: roundAwayFromZero(x[1]),
		Z: roundAwayFromZero(x[2]),
	}, nil
}

// normSq returns the checked squared length of v.
func normSq(v catalog.Vec3) (int64, error) {
	var total int64
	for _, c := range []int64{v.X, v.Y, v.Z} {
		s, ok := domain.Mul(c, c)
		if !ok {
			return 0, ErrArithmeticOverflow
		}
		var ok2 bool
		total, ok2 = domain.Add(total, s)
		if !ok2 {
			return 0, ErrArithmeticOverflow
		}
	}
	return total, nil
}

// leastSquares3 solves the 3-unknown least-squares problem (A^T A) x = A^T b
// by forming normal equations and applying Gaussian elimination.
func leastSquares3(a [][]float64, b []float64) ([3]float64, bool) {
	var ata [3][3]float64
	var atb [3]float64
	for i, row := range a {
		for j := 0; j < 3; j++ {
			atb[j] += row[j] * b[i]
			for k := 0; k < 3; k++ {
				ata[j][k] += row[j] * row[k]
			}
		}
	}
	return gauss3(ata, atb)
}

// gauss3 solves a 3x3 linear system with partial pivoting. It reports false
// when the system is singular (determinant zero).
func gauss3(m [3][3]float64, v [3]float64) ([3]float64, bool) {
	a := m
	b := v
	for col := 0; col < 3; col++ {
		pivot := col
		for r := col + 1; r < 3; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(a[pivot][col]) < 1e-12 {
			return [3]float64{}, false
		}
		if pivot != col {
			a[col], a[pivot] = a[pivot], a[col]
			b[col], b[pivot] = b[pivot], b[col]
		}
		for r := col + 1; r < 3; r++ {
			f := a[r][col] / a[col][col]
			for c := col; c < 3; c++ {
				a[r][c] -= f * a[col][c]
			}
			b[r] -= f * b[col]
		}
	}
	var x [3]float64
	for r := 2; r >= 0; r-- {
		s := b[r]
		for c := r + 1; c < 3; c++ {
			s -= a[r][c] * x[c]
		}
		x[r] = s / a[r][r]
	}
	return x, true
}

// roundDist reconstructs the integer distance from a solved coordinate to a
// reference point, rounding half away from zero.
func roundDist(p, r catalog.Vec3) int64 {
	d := p.Sub(r)
	sq := float64(d.X*d.X + d.Y*d.Y + d.Z*d.Z)
	return roundAwayFromZero(math.Sqrt(sq))
}

// roundAwayFromZero rounds a float64 to the nearest integer, half away from
// zero, matching the domain rounding rule.
func roundAwayFromZero(f float64) int64 {
	if f >= 0 {
		return int64(f + 0.5)
	}
	return -int64(-f + 0.5)
}

// solveInputDigest binds a solve to its exact inputs.
func solveInputDigest(config catalog.FrozenConfiguration, constraints []catalog.BaselineConstraint) string {
	return config.Digest() + ":" + constraintsDigest(constraints)
}

// constraintsDigest produces a stable digest of the sorted constraints.
func constraintsDigest(constraints []catalog.BaselineConstraint) string {
	cp := append([]catalog.BaselineConstraint(nil), constraints...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Transponder != cp[j].Transponder {
			return cp[i].Transponder < cp[j].Transponder
		}
		return cp[i].Line < cp[j].Line
	})
	h := uint64(14695981039346656037)
	for _, c := range cp {
		h = fnvAddStr(h, c.Line)
		h = fnvAddStr(h, c.Reference)
		h = fnvAddStr(h, c.Transponder)
		h = fnvAddInt(h, c.DistanceMM)
		h = fnvAddInt(h, c.Weight)
		h = fnvAddInt(h, c.Epoch)
	}
	return fmt.Sprintf("%016x", h)
}
