package catalog

// Vec3 is an integer three-dimensional coordinate in millimetres.
type Vec3 struct {
	X int64
	Y int64
	Z int64
}

// ReferencePoint is a deck-mounted reference point with known integer
// coordinates. It anchors the weighted baseline constraint graph.
type ReferencePoint struct {
	ID    string
	Coord Vec3
}

// TransponderSpec describes a transponder to be bound at a mount point.
type TransponderSpec struct {
	ID         string
	Serial     string
	MountPoint string
	Coord      Vec3 // planned (a priori) integer coordinates in millimetres
}

// BaselineConstraint is one weighted edge between a reference point and a
// transponder: a measured line with an integer distance and a positive weight.
type BaselineConstraint struct {
	Reference   string
	Transponder string
	Line        string
	DistanceMM  int64
	Weight      int64
	Epoch       int64
}

// Line is a planned calibration line (a measurement between one reference
// point and one transponder) frozen into the configuration.
type Line struct {
	ID          string
	Reference   string
	Transponder string
}

// Sub returns the vector difference v - o.
func (v Vec3) Sub(o Vec3) Vec3 {
	return Vec3{X: v.X - o.X, Y: v.Y - o.Y, Z: v.Z - o.Z}
}

// Dot returns the dot product of two vectors.
func (v Vec3) Dot(o Vec3) int64 {
	return v.X*o.X + v.Y*o.Y + v.Z*o.Z
}

// Cross returns the cross product v x o.
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{
		X: v.Y*o.Z - v.Z*o.Y,
		Y: v.Z*o.X - v.X*o.Z,
		Z: v.X*o.Y - v.Y*o.X,
	}
}

// NormSq returns the squared Euclidean length.
func (v Vec3) NormSq() int64 {
	return v.Dot(v)
}
