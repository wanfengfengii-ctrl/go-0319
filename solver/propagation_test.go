package solver

import (
	"errors"
	"testing"
)

func TestSingleLayerDistance(t *testing.T) {
	cases := []struct {
		speed, timeUS int64
		want          int64
	}{
		{1_500_000, 1_000_000, 1_500_000}, // 1s at 1500 m/s
		{1, 500_000, 1},                   // 0.5mm rounds away from zero
		{1, 499_999, 0},
		{1, 500_001, 1},
		{2, 500_000, 1}, // exactly 1.0mm
	}
	for _, c := range cases {
		got, err := SingleLayerDistance(c.speed, c.timeUS)
		if err != nil {
			t.Fatalf("SingleLayerDistance(%d,%d) err: %v", c.speed, c.timeUS, err)
		}
		if got != c.want {
			t.Errorf("SingleLayerDistance(%d,%d) = %d, want %d", c.speed, c.timeUS, got, c.want)
		}
	}
}

func TestSingleLayerDistanceErrors(t *testing.T) {
	if _, err := SingleLayerDistance(0, 100); !errors.Is(err, ErrNonPositiveSpeed) {
		t.Fatalf("want ErrNonPositiveSpeed, got %v", err)
	}
	if _, err := SingleLayerDistance(100, -1); !errors.Is(err, ErrNonPositiveTime) {
		t.Fatalf("want ErrNonPositiveTime, got %v", err)
	}
	if _, err := SingleLayerDistance(1<<62, 1<<62); !errors.Is(err, ErrArithmeticOverflow) {
		t.Fatalf("want ErrArithmeticOverflow, got %v", err)
	}
}

func TestLayeredDistance(t *testing.T) {
	entries := []LayerTime{
		{SpeedMMS: 1_500_000, TimeUS: 1_000_000},
		{SpeedMMS: 1_510_000, TimeUS: 500_000},
	}
	got, err := LayeredDistance(entries)
	if err != nil {
		t.Fatalf("LayeredDistance err: %v", err)
	}
	// 1_500_000 + round(1_510_000*500_000/1e6)=1_500_000+755_000
	if want := int64(2_255_000); got != want {
		t.Errorf("LayeredDistance = %d, want %d", got, want)
	}
}

func TestBidirectionalDistance(t *testing.T) {
	cases := []struct{ total, want int64 }{
		{3, 2},
		{5, 3},
		{4, 2},
		{1, 1},
	}
	for _, c := range cases {
		if got := BidirectionalDistance(c.total); got != c.want {
			t.Errorf("BidirectionalDistance(%d) = %d, want %d", c.total, got, c.want)
		}
	}
}

func TestCompensatedTime(t *testing.T) {
	got, err := CompensatedTime(100, 10, 5)
	if err != nil || got != 85 {
		t.Fatalf("CompensatedTime = %d, %v; want 85", got, err)
	}
	if _, err := CompensatedTime(10, 100, 5); !errors.Is(err, ErrNonPositiveTime) {
		t.Fatalf("want ErrNonPositiveTime, got %v", err)
	}
}

func TestCounterDelta(t *testing.T) {
	if got, err := CounterDelta(100, 10, 30); err != nil || got != 20 {
		t.Fatalf("CounterDelta = %d, %v; want 20", got, err)
	}
	// negative difference is ambiguous
	if _, err := CounterDelta(100, 30, 10); !errors.Is(err, ErrAmbiguousDelta) {
		t.Fatalf("want ErrAmbiguousDelta for negative, got %v", err)
	}
	// difference equal to half modulus is rejected (strictly less required)
	if _, err := CounterDelta(100, 10, 60); !errors.Is(err, ErrAmbiguousDelta) {
		t.Fatalf("want ErrAmbiguousDelta for half modulus, got %v", err)
	}
	if _, err := CounterDelta(100, 10, 59); err != nil {
		t.Fatalf("unexpected error for 49 delta: %v", err)
	}
	if _, err := CounterDelta(0, 1, 2); !errors.Is(err, ErrInvalidModulus) {
		t.Fatalf("want ErrInvalidModulus, got %v", err)
	}
}
