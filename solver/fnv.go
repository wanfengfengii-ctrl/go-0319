package solver

// fnvAddStr folds a string into the running FNV-1a 64-bit hash.
func fnvAddStr(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// fnvAddInt folds an int64 into the running FNV-1a 64-bit hash.
func fnvAddInt(h uint64, v int64) uint64 {
	u := uint64(v)
	for i := 0; i < 8; i++ {
		h ^= u & 0xff
		h *= 1099511628211
		u >>= 8
	}
	return h
}
