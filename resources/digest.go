package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// leaseToken derives a deterministic lease token from the idempotency key and
// the resource identity so that an identical retry reproduces the exact same
// token (and therefore the exact same result).
func leaseToken(idemKey string, t domain.ResourceType, id string) string {
	sum := sha256.Sum256([]byte(idemKey + "|" + string(t) + "|" + id))
	return hex.EncodeToString(sum[:])[:24]
}

// contentDigest computes the canonical content digest of a batch of bindings
// and leases, used to detect idempotent retries versus content changes.
func contentDigest(bindings []domain.TransponderBinding, leases []domain.ResourceLease) string {
	serials := make([]string, 0, len(bindings))
	for _, b := range bindings {
		serials = append(serials, b.Serial+"|"+b.MountPoint)
	}
	sort.Strings(serials)

	resources := make([]string, 0, len(leases))
	for _, l := range leases {
		resources = append(resources, string(l.ResourceType)+"|"+l.ResourceID+"|"+l.LeaseToken)
	}
	sort.Strings(resources)

	var sb strings.Builder
	for _, s := range serials {
		sb.WriteString("b:")
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	for _, s := range resources {
		sb.WriteString("l:")
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
