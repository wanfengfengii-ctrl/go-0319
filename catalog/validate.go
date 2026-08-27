package catalog

import (
	"errors"
	"fmt"
)

// Graph validation failures.
var (
	ErrGraphDisconnected  = errors.New("catalog: constraint graph is disconnected")
	ErrGeometryDegenerate = errors.New("catalog: geometry is degenerate")
	ErrDuplicateLine      = errors.New("catalog: duplicate equivalent constraint line")
	ErrUnknownNode        = errors.New("catalog: line references an unknown node")
)

// minConstraintsFor3D is the minimum number of independent distance
// constraints a transponder needs for a unique three-dimensional fix.
const minConstraintsFor3D = 4

// ValidateGeometry performs the pre-freeze structural checks over the
// reference points, transponders and planned line set. It verifies that the
// reference points span three dimensions, that every transponder is reachable
// by at least four independent lines, and that no duplicate equivalent line
// exists.
func ValidateGeometry(refs []ReferencePoint, transponders []TransponderSpec, lines []Line) error {
	if len(refs) < minConstraintsFor3D {
		return fmt.Errorf("%w: need at least %d reference points", ErrGeometryDegenerate, minConstraintsFor3D)
	}
	if coplanar(refs) {
		return fmt.Errorf("%w: reference points are coplanar", ErrGeometryDegenerate)
	}

	refIDs := make(map[string]bool, len(refs))
	for _, r := range refs {
		refIDs[r.ID] = true
	}
	tpIDs := make(map[string]bool, len(transponders))
	for _, tp := range transponders {
		tpIDs[tp.ID] = true
	}

	perTp := make(map[string]map[string]bool)
	seen := make(map[string]bool)
	for _, l := range lines {
		if !refIDs[l.Reference] || !tpIDs[l.Transponder] {
			return fmt.Errorf("%w: line %q", ErrUnknownNode, l.ID)
		}
		edgeKey := l.Reference + "\x00" + l.Transponder
		if seen[edgeKey] {
			return fmt.Errorf("%w: line %q duplicates an equivalent line", ErrDuplicateLine, l.ID)
		}
		seen[edgeKey] = true
		if perTp[l.Transponder] == nil {
			perTp[l.Transponder] = make(map[string]bool)
		}
		perTp[l.Transponder][l.Reference] = true
	}

	for _, tp := range transponders {
		if len(perTp[tp.ID]) < minConstraintsFor3D {
			return fmt.Errorf("%w: transponder %q has %d lines, need %d", ErrGeometryDegenerate, tp.ID, len(perTp[tp.ID]), minConstraintsFor3D)
		}
	}
	return nil
}

// ValidateConstraints checks measured baseline constraints (weights, distances
// and node membership) before a solve.
func ValidateConstraints(refs []ReferencePoint, transponders []TransponderSpec, constraints []BaselineConstraint) error {
	refIDs := make(map[string]bool, len(refs))
	for _, r := range refs {
		refIDs[r.ID] = true
	}
	tpIDs := make(map[string]bool, len(transponders))
	for _, tp := range transponders {
		tpIDs[tp.ID] = true
	}
	seen := make(map[string]bool)
	for _, c := range constraints {
		if !refIDs[c.Reference] || !tpIDs[c.Transponder] {
			return fmt.Errorf("%w: constraint %q", ErrUnknownNode, c.Line)
		}
		if c.Weight <= 0 {
			return fmt.Errorf("catalog: constraint %q has non-positive weight", c.Line)
		}
		if c.DistanceMM <= 0 {
			return fmt.Errorf("catalog: constraint %q has non-positive distance", c.Line)
		}
		key := c.Reference + "\x00" + c.Transponder
		if seen[key] {
			return fmt.Errorf("%w: constraint %q", ErrDuplicateLine, c.Line)
		}
		seen[key] = true
	}
	return nil
}

// coplanar reports whether all reference points lie in a single plane. Four
// points are non-coplanar when the scalar triple product of three vectors from
// the first point is non-zero.
func coplanar(refs []ReferencePoint) bool {
	if len(refs) < minConstraintsFor3D {
		return true
	}
	origin := refs[0].Coord
	v1 := refs[1].Coord.Sub(origin)
	v2 := refs[2].Coord.Sub(origin)
	v3 := refs[3].Coord.Sub(origin)
	return v1.Dot(v2.Cross(v3)) == 0
}
