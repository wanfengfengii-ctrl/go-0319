package catalog

import (
	"errors"
	"testing"
)

func validProfile() SoundSpeedProfile {
	return SoundSpeedProfile{Layers: []SoundSpeedLayer{
		{TopMM: 0, BottomMM: 1000, SpeedMMS: 1_500_000},
		{TopMM: 1000, BottomMM: 2500, SpeedMMS: 1_510_000},
		{TopMM: 2500, BottomMM: 5000, SpeedMMS: 1_520_000},
	}}
}

func TestProfileValidateOK(t *testing.T) {
	if err := validProfile().Validate(); err != nil {
		t.Fatalf("expected valid profile, got %v", err)
	}
}

func TestProfileValidateGap(t *testing.T) {
	p := validProfile()
	p.Layers[1].TopMM = 1500 // previous bottom is 1000
	if err := p.Validate(); !errors.Is(err, ErrProfileGap) {
		t.Fatalf("want ErrProfileGap, got %v", err)
	}
}

func TestProfileValidateOverlap(t *testing.T) {
	p := validProfile()
	p.Layers[1].TopMM = 800 // previous bottom is 1000
	if err := p.Validate(); !errors.Is(err, ErrProfileOverlap) {
		t.Fatalf("want ErrProfileOverlap, got %v", err)
	}
}

func TestProfileValidateEmpty(t *testing.T) {
	if err := (SoundSpeedProfile{}).Validate(); !errors.Is(err, ErrEmptyProfile) {
		t.Fatalf("want ErrEmptyProfile, got %v", err)
	}
}

func TestProfileValidateBadLayer(t *testing.T) {
	p := validProfile()
	p.Layers[0].SpeedMMS = 0
	if err := p.Validate(); !errors.Is(err, ErrLayerSpeed) {
		t.Fatalf("want ErrLayerSpeed, got %v", err)
	}
	p = validProfile()
	p.Layers[0].BottomMM = 0
	if err := p.Validate(); !errors.Is(err, ErrLayerDepth) {
		t.Fatalf("want ErrLayerDepth, got %v", err)
	}
}

func TestValidateSlots(t *testing.T) {
	ok := []Slot{{ID: "a", StartUS: 0, EndUS: 100}, {ID: "b", StartUS: 100, EndUS: 200}}
	if err := ValidateSlots(ok); err != nil {
		t.Fatalf("expected valid slots, got %v", err)
	}
	overlap := []Slot{{ID: "a", StartUS: 0, EndUS: 100}, {ID: "b", StartUS: 50, EndUS: 200}}
	if err := ValidateSlots(overlap); !errors.Is(err, ErrSlotCollision) {
		t.Fatalf("want ErrSlotCollision, got %v", err)
	}
}
