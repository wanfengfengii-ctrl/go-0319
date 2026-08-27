// Package catalog holds the immutable acoustic configuration and the rules
// that validate it before a task is frozen.
package catalog

import (
	"errors"
	"fmt"
)

// Validation failures for the sound speed profile. These map to the stable
// PROFILE_GAP / PROFILE_OVERLAP error codes surfaced by the HTTP API.
var (
	ErrEmptyProfile   = errors.New("catalog: empty sound speed profile")
	ErrLayerDepth     = errors.New("catalog: layer must have positive thickness")
	ErrLayerSpeed     = errors.New("catalog: layer sound speed must be positive")
	ErrProfileGap     = errors.New("catalog: gap between sound speed layers")
	ErrProfileOverlap = errors.New("catalog: overlap between sound speed layers")
)

// SoundSpeedLayer is one layer of a continuous sound speed profile. Depth
// boundaries are integer millimetres using left-closed, right-open intervals;
// adjacent layer endpoints must be equal and the final layer may include its
// upper endpoint.
type SoundSpeedLayer struct {
	TopMM    int64 // inclusive top depth boundary
	BottomMM int64 // exclusive bottom depth boundary
	SpeedMMS int64 // sound speed in millimetres per second
}

// SoundSpeedProfile is an ordered list of contiguous sound speed layers.
type SoundSpeedProfile struct {
	Layers []SoundSpeedLayer
}

// Validate checks that the profile is non-empty, that each layer has positive
// thickness and speed, and that layers are contiguous (no gap, no overlap) and
// ascending.
func (p SoundSpeedProfile) Validate() error {
	if len(p.Layers) == 0 {
		return ErrEmptyProfile
	}
	for i, l := range p.Layers {
		if l.TopMM >= l.BottomMM {
			return fmt.Errorf("%w: layer %d", ErrLayerDepth, i)
		}
		if l.SpeedMMS <= 0 {
			return fmt.Errorf("%w: layer %d", ErrLayerSpeed, i)
		}
		if i == 0 {
			continue
		}
		prev := p.Layers[i-1]
		switch {
		case prev.BottomMM < l.TopMM:
			return fmt.Errorf("%w: between layer %d and %d", ErrProfileGap, i-1, i)
		case prev.BottomMM > l.TopMM:
			return fmt.Errorf("%w: between layer %d and %d", ErrProfileOverlap, i-1, i)
		}
	}
	return nil
}
