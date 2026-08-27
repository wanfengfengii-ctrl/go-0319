// Package solver implements the public integer fixed-point acoustic arithmetic
// shared by propagation, delay compensation and baseline solving.
package solver

import (
	"errors"
	"fmt"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// AlgorithmVersion identifies the published integer fixed-point algorithm.
// Solve results record this version so any vector can be recomputed.
const AlgorithmVersion = 1

// microseconds per second.
const microsPerSecond int64 = 1_000_000

// Arithmetic failures.
var (
	ErrArithmeticOverflow = errors.New("solver: arithmetic overflow")
	ErrNonPositiveSpeed   = errors.New("solver: sound speed must be positive")
	ErrNonPositiveTime    = errors.New("solver: time must be positive")
	ErrInvalidModulus     = errors.New("solver: counter modulus must be positive")
	ErrAmbiguousDelta     = errors.New("solver: ambiguous counter delta")
)

// SingleLayerDistance computes a one-way propagation distance in millimetres
// for a single sound speed layer: round(speed_mm_s * time_us / 1_000_000). It
// fails with ErrArithmeticOverflow if the intermediate product overflows.
func SingleLayerDistance(speedMMS, timeUS int64) (int64, error) {
	if speedMMS <= 0 {
		return 0, ErrNonPositiveSpeed
	}
	if timeUS < 0 {
		return 0, ErrNonPositiveTime
	}
	prod, ok := domain.Mul(speedMMS, timeUS)
	if !ok {
		return 0, ErrArithmeticOverflow
	}
	return domain.RoundDiv(prod, microsPerSecond), nil
}

// LayerTime pairs a layer sound speed with its allocated time slice.
type LayerTime struct {
	SpeedMMS int64
	TimeUS   int64
}

// LayeredDistance sums per-layer rounded propagation distances. Each layer's
// time slice is rounded independently and then summed, matching the rule that
// layered propagation rounds each layer slice before summing.
func LayeredDistance(entries []LayerTime) (int64, error) {
	var total int64
	for i, e := range entries {
		d, err := SingleLayerDistance(e.SpeedMMS, e.TimeUS)
		if err != nil {
			return 0, fmt.Errorf("solver: layer %d: %w", i, err)
		}
		var ok bool
		total, ok = domain.Add(total, d)
		if !ok {
			return 0, ErrArithmeticOverflow
		}
	}
	return total, nil
}

// BidirectionalDistance converts a round-trip acoustic path into the one-way
// distance: round(total_mm / 2).
func BidirectionalDistance(totalMM int64) int64 {
	return domain.RoundDiv(totalMM, 2)
}

// CompensatedTime returns receive-transmit-transducerDelay, which must be
// strictly positive.
func CompensatedTime(receiveUS, transmitUS, delayUS int64) (int64, error) {
	rt, ok := domain.Sub(receiveUS, transmitUS)
	if !ok {
		return 0, ErrArithmeticOverflow
	}
	c, ok := domain.Sub(rt, delayUS)
	if !ok {
		return 0, ErrArithmeticOverflow
	}
	if c <= 0 {
		return 0, ErrNonPositiveTime
	}
	return c, nil
}

// CounterDelta validates a counter time difference under a frozen counter
// modulus. Only a strictly positive difference smaller than half the modulus is
// accepted; anything else is ambiguous or requires a new epoch.
func CounterDelta(modulus, a, b int64) (int64, error) {
	if modulus <= 0 {
		return 0, ErrInvalidModulus
	}
	d, ok := domain.Sub(b, a)
	if !ok {
		return 0, ErrArithmeticOverflow
	}
	half := modulus / 2
	if d <= 0 || d >= half {
		return 0, ErrAmbiguousDelta
	}
	return d, nil
}
