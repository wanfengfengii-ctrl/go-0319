package catalog

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashHex returns the lowercase hexadecimal SHA-256 digest of b.
func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
