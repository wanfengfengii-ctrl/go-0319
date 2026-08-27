// Package resources implements the device-identity and resource-lease manager.
// It atomically binds transponder serials and leases the five exclusive rig
// resources, judged only against the injected logical clock, and supports
// idempotent operation registration, renewal and expiry.
package resources

import (
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

// BindingRequest is one transponder identity to bind.
type BindingRequest struct {
	Serial     string
	MountPoint string
}

// LeaseRequest is one exclusive resource lease request. The duration is a
// logical-time interval; the absolute bounds are computed from the injected
// clock at acquisition time.
type LeaseRequest struct {
	ResourceType domain.ResourceType
	ResourceID   string
	Duration     domain.LogicalTime
}

// AcquireRequest is the full atomic bind-and-lease input.
type AcquireRequest struct {
	Bindings []BindingRequest
	Leases   []LeaseRequest
}

// AcquireResult echoes the committed bindings and leases (with tokens).
type AcquireResult struct {
	Bindings []domain.TransponderBinding
	Leases   []domain.ResourceLease
}

// Manager coordinates identity binding and resource leasing.
type Manager struct {
	store persistence.Store
	clock domain.Clock
}

// NewManager constructs a Manager.
func NewManager(store persistence.Store, clock domain.Clock) *Manager {
	return &Manager{store: store, clock: clock}
}

// Acquire atomically binds every requested serial and leases every requested
// resource for the task, under the given idempotency key. A retry with the
// same key and canonical content returns the original result; a different
// content returns IDEMPOTENCY_CONFLICT.
func (m *Manager) Acquire(key domain.TaskKey, idemKey string, req AcquireRequest) (AcquireResult, error) {
	now := m.clock.Now()

	bindings := make([]domain.TransponderBinding, 0, len(req.Bindings))
	for _, b := range req.Bindings {
		bindings = append(bindings, domain.TransponderBinding{
			Key:               key,
			Serial:            b.Serial,
			MountPoint:        b.MountPoint,
			BindingGeneration: int64(key.Generation),
			BoundAt:           now,
		})
	}

	leases := make([]domain.ResourceLease, 0, len(req.Leases))
	for _, l := range req.Leases {
		if l.Duration <= 0 {
			return AcquireResult{}, domain.NewError(domain.CodeInvalidRequest, "lease duration must be positive")
		}
		leases = append(leases, domain.ResourceLease{
			ResourceType: l.ResourceType,
			ResourceID:   l.ResourceID,
			Key:          key,
			LeaseToken:   leaseToken(idemKey, l.ResourceType, l.ResourceID),
			StartTime:    now,
			EndTime:      now + l.Duration,
			Version:      1,
		})
	}

	digest := contentDigest(bindings, leases)
	idem := domain.IdempotencyRecord{
		Key:            idemKey,
		ContentDigest:  digest,
		ResponseDigest: digest,
		RecordedAt:     now,
	}

	if err := m.store.AcquireBindingsAndLeases(key, bindings, leases, idem, now); err != nil {
		return AcquireResult{}, err
	}

	return AcquireResult{Bindings: bindings, Leases: leases}, nil
}

// Renew extends a lease to the given logical time under its token.
func (m *Manager) Renew(token string, until domain.LogicalTime) (domain.ResourceLease, error) {
	if until <= m.clock.Now() {
		return domain.ResourceLease{}, domain.NewError(domain.CodeLeaseExpired, "renewal target is not in the future")
	}
	return m.store.RenewLease(token, until)
}
