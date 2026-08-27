package domain

// LogicalTime is a microsecond-resolution logical clock value. Every duration,
// timestamp, lease bound and retry schedule in the calibration domain uses this
// integer scale; leases are judged exclusively against logical time, never wall
// clock.
type LogicalTime int64
