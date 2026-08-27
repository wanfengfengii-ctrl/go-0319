package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// Slot validation failures map to the stable SLOT_COLLISION error code.
var (
	ErrSlotInvalid   = errors.New("catalog: slot must have non-negative increasing bounds")
	ErrSlotCollision = errors.New("catalog: overlapping transmit slots")
)

// Slot is a time-division transmit slot with integer microsecond bounds using a
// left-closed, right-open interval.
type Slot struct {
	ID      string
	StartUS int64
	EndUS   int64
}

// ValidateSlots rejects non-increasing or overlapping slots. Slots are expected
// to be presented in ascending start order.
func ValidateSlots(slots []Slot) error {
	for i, s := range slots {
		if s.StartUS < 0 || s.EndUS <= s.StartUS {
			return fmt.Errorf("%w: slot %q", ErrSlotInvalid, s.ID)
		}
		if i > 0 && s.StartUS < slots[i-1].EndUS {
			return fmt.Errorf("%w: slots %q and %q", ErrSlotCollision, slots[i-1].ID, s.ID)
		}
	}
	return nil
}

// FrozenConfiguration is the immutable configuration captured when a task is
// frozen. After freezing it is read-only and identified by a digest; every
// later stage compares against the frozen version.
type FrozenConfiguration struct {
	// Version is the optimistic concurrency version; freeze requires an
	// expected version and rejects stale submissions.
	Version int64
	// MountBases are the installation bases recorded on deck.
	MountBases []string
	// ReferencePoints anchor the constraint graph.
	ReferencePoints []ReferencePoint
	// Transponders are the array transponders to be bound.
	Transponders []TransponderSpec
	// Profile is the continuous sound speed profile.
	Profile SoundSpeedProfile
	// Slots is the frozen time-division transmit schedule.
	Slots []Slot
	// TransmitCodes are the frozen transmit codes assigned to transponders.
	TransmitCodes map[string]string
	// ClockSource identifies the reference clock source.
	ClockSource string
	// Lines is the frozen calibration line plan.
	Lines []Line
	// ReviewQualifications snapshots reviewer validity at freeze time.
	ReviewQualifications []domain.ReviewQualification
	// TransducerDelayUS is the combined transmit/receive transducer delay.
	TransducerDelayUS int64
	// ResidualThresholdMM is the per-line residual acceptance threshold.
	ResidualThresholdMM int64
	// DriftThresholdUS is the maximum accepted clock drift.
	DriftThresholdUS int64
	// CounterModulus is the frozen counter wrap modulus.
	CounterModulus int64
	// SequenceMax is the explicit unsigned upper bound on a transponder's
	// transmit sequence within one epoch; exceeding it forces a new epoch.
	SequenceMax uint64
	// RetryMax is the maximum number of device retry attempts.
	RetryMax int
}

// canonicalForm is a deterministic JSON representation used for digesting. It
// strips nothing but forces a stable key order and omits the version, which is
// the only mutable field during an attempted freeze.
func (c FrozenConfiguration) canonicalForm() any {
	slots := make([][3]any, 0, len(c.Slots))
	for _, s := range c.Slots {
		slots = append(slots, [3]any{s.ID, s.StartUS, s.EndUS})
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i][0].(string) < slots[j][0].(string) })

	refs := make([][4]any, 0, len(c.ReferencePoints))
	for _, r := range c.ReferencePoints {
		refs = append(refs, [4]any{r.ID, r.Coord.X, r.Coord.Y, r.Coord.Z})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i][0].(string) < refs[j][0].(string) })

	tps := make([][6]any, 0, len(c.Transponders))
	for _, tp := range c.Transponders {
		tps = append(tps, [6]any{tp.ID, tp.Serial, tp.MountPoint, tp.Coord.X, tp.Coord.Y, tp.Coord.Z})
	}
	sort.Slice(tps, func(i, j int) bool { return tps[i][0].(string) < tps[j][0].(string) })

	lines := make([][3]any, 0, len(c.Lines))
	for _, l := range c.Lines {
		lines = append(lines, [3]any{l.ID, l.Reference, l.Transponder})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][0].(string) < lines[j][0].(string) })

	codes := make([][2]string, 0, len(c.TransmitCodes))
	for k, v := range c.TransmitCodes {
		codes = append(codes, [2]string{k, v})
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i][0] < codes[j][0] })

	quals := make([][2]any, 0, len(c.ReviewQualifications))
	for _, q := range c.ReviewQualifications {
		quals = append(quals, [2]any{q.ReviewerID, q.ValidUntil})
	}
	sort.Slice(quals, func(i, j int) bool { return quals[i][0].(string) < quals[j][0].(string) })

	layers := make([][3]int64, 0, len(c.Profile.Layers))
	for _, l := range c.Profile.Layers {
		layers = append(layers, [3]int64{l.TopMM, l.BottomMM, l.SpeedMMS})
	}

	return struct {
		MountBases           []string    `json:"mount_bases"`
		ReferencePoints      [][4]any    `json:"reference_points"`
		Transponders         [][6]any    `json:"transponders"`
		Layers               [][3]int64  `json:"layers"`
		Slots                [][3]any    `json:"slots"`
		TransmitCodes        [][2]string `json:"transmit_codes"`
		ClockSource          string      `json:"clock_source"`
		Lines                [][3]any    `json:"lines"`
		ReviewQualifications [][2]any    `json:"review_qualifications"`
		TransducerDelayUS    int64       `json:"transducer_delay_us"`
		ResidualThresholdMM  int64       `json:"residual_threshold_mm"`
		DriftThresholdUS     int64       `json:"drift_threshold_us"`
		CounterModulus       int64       `json:"counter_modulus"`
		SequenceMax          uint64      `json:"sequence_max"`
		RetryMax             int         `json:"retry_max"`
	}{
		MountBases:           c.MountBases,
		ReferencePoints:      refs,
		Transponders:         tps,
		Layers:               layers,
		Slots:                slots,
		TransmitCodes:        codes,
		ClockSource:          c.ClockSource,
		Lines:                lines,
		ReviewQualifications: quals,
		TransducerDelayUS:    c.TransducerDelayUS,
		ResidualThresholdMM:  c.ResidualThresholdMM,
		DriftThresholdUS:     c.DriftThresholdUS,
		CounterModulus:       c.CounterModulus,
		SequenceMax:          c.SequenceMax,
		RetryMax:             c.RetryMax,
	}
}

// Digest returns the immutable, canonical content digest of the configuration
// (excluding the mutable version field). It is a hexadecimal SHA-256 digest of
// the canonical JSON form.
func (c FrozenConfiguration) Digest() string {
	b, err := json.Marshal(c.canonicalForm())
	if err != nil {
		panic(err)
	}
	return hashHex(b)
}
