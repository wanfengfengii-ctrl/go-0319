package domain

// Phase is a stage in the deterministic calibration workflow. Phases form a
// strict prefix: a later phase is reachable only after every earlier phase has
// completed, and an out-of-order command must be rejected without recording any
// stage transition or evidence.
type Phase uint8

const (
	PhaseCreated Phase = iota
	PhaseConfigFrozen
	PhaseBindingsAcquired
	PhaseClockDisciplined
	PhaseLoopbackConfirmed
	PhaseRanging
	PhaseSolved
	PhaseResidualPassed
	PhaseRecalibrationDone
	PhaseReviewed
	PhaseTerminal
)

var phaseNames = [...]string{
	PhaseCreated:           "created",
	PhaseConfigFrozen:      "config_frozen",
	PhaseBindingsAcquired:  "bindings_acquired",
	PhaseClockDisciplined:  "clock_disciplined",
	PhaseLoopbackConfirmed: "loopback_confirmed",
	PhaseRanging:           "ranging",
	PhaseSolved:            "solved",
	PhaseResidualPassed:    "residual_passed",
	PhaseRecalibrationDone: "recalibration_done",
	PhaseReviewed:          "reviewed",
	PhaseTerminal:          "terminal",
}

// String returns the stable wire name for the phase.
func (p Phase) String() string {
	if int(p) < len(phaseNames) {
		return phaseNames[p]
	}
	return "unknown"
}
