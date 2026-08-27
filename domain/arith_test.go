package domain

import (
	"math"
	"testing"
)

func TestRoundDivHalfAwayFromZero(t *testing.T) {
	cases := []struct {
		a, b int64
		want int64
	}{
		{5, 2, 3},
		{4, 2, 2},
		{-5, 2, -3},
		{-4, 2, -2},
		{1, 2, 1},
		{-1, 2, -1},
		{3, 2, 2},
		{-3, 2, -2},
		{7, 3, 2},
		{8, 3, 3},
		{-7, 3, -2},
		{-8, 3, -3},
		{0, 5, 0},
		{2, 5, 0},
		{3, 5, 1},
		{-2, 5, 0},
		{-3, 5, -1},
	}
	for _, c := range cases {
		if got := RoundDiv(c.a, c.b); got != c.want {
			t.Errorf("RoundDiv(%d, %d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestMulOverflow(t *testing.T) {
	if _, ok := Mul(math.MaxInt64, 2); ok {
		t.Fatal("expected overflow for MaxInt64*2")
	}
	if _, ok := Mul(math.MinInt64, -1); ok {
		t.Fatal("expected overflow for MinInt64*-1")
	}
	if got, ok := Mul(math.MinInt64, 1); !ok || got != math.MinInt64 {
		t.Fatalf("Mul(MinInt64, 1) = %d, ok=%v; want no overflow", got, ok)
	}
	if got, ok := Mul(6, 7); !ok || got != 42 {
		t.Fatalf("Mul(6,7) = %d, ok=%v; want 42", got, ok)
	}
}

func TestAddOverflow(t *testing.T) {
	if _, ok := Add(math.MaxInt64, 1); ok {
		t.Fatal("expected overflow for MaxInt64+1")
	}
	if _, ok := Add(math.MinInt64, -1); ok {
		t.Fatal("expected overflow for MinInt64-1")
	}
	if got, ok := Add(1, 2); !ok || got != 3 {
		t.Fatalf("Add(1,2) = %d, ok=%v; want 3", got, ok)
	}
}

func TestSubOverflow(t *testing.T) {
	if _, ok := Sub(math.MinInt64, 1); ok {
		t.Fatal("expected overflow for MinInt64-1")
	}
	if _, ok := Sub(math.MaxInt64, -1); ok {
		t.Fatal("expected overflow for MaxInt64-(-1)")
	}
	if got, ok := Sub(5, 2); !ok || got != 3 {
		t.Fatalf("Sub(5,2) = %d, ok=%v; want 3", got, ok)
	}
}
