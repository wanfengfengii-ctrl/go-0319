package domain

// RoundDiv divides a by a positive divisor b and rounds half away from zero.
// This is the single rounding rule for all acoustic arithmetic: the truncated
// quotient is incremented away from zero when the absolute remainder is at
// least half the divisor. It panics when b is not positive, matching the domain
// invariant that every divisor must be positive.
func RoundDiv(a, b int64) int64 {
	if b <= 0 {
		panic("domain: RoundDiv divisor must be positive")
	}
	q := a / b
	r := a % b
	if r < 0 {
		r = -r
	}
	// Equivalent to 2*r >= b without overflowing when b is large.
	if r >= b-r {
		if a >= 0 {
			q++
		} else {
			q--
		}
	}
	return q
}

// Mul returns a*b and ok == false when the int64 result overflows.
func Mul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if (a == -1 && b == -1<<63) || (b == -1 && a == -1<<63) {
		return 0, false
	}
	r := a * b
	if r/b != a {
		return 0, false
	}
	return r, true
}

// Add returns a+b and ok == false when the int64 result overflows.
func Add(a, b int64) (int64, bool) {
	r := a + b
	if (b > 0 && r < a) || (b < 0 && r > a) {
		return 0, false
	}
	return r, true
}

// Sub returns a-b and ok == false when the int64 result overflows.
func Sub(a, b int64) (int64, bool) {
	r := a - b
	if (b < 0 && r < a) || (b > 0 && r > a) {
		return 0, false
	}
	return r, true
}
