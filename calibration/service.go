// Package calibration implements the task aggregate: the strict stage-prefix
// state machine, clock-epoch management, time-division sequence allocation,
// append-only evidence recording and joint baseline solving. It coordinates
// device calls, identity binding and resource leasing through the persistence
// store, judged only against the injected logical clock.
package calibration

import (
	"errors"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/resources"
)

// Service is the calibration aggregate coordinator.
type Service struct {
	store   persistence.Store
	clock   domain.Clock
	devices domain.DeviceAdapter
	leases  *resources.Manager
}

// New constructs a Service.
func New(store persistence.Store, clock domain.Clock, devices domain.DeviceAdapter) *Service {
	return &Service{
		store:   store,
		clock:   clock,
		devices: devices,
		leases:  resources.NewManager(store, clock),
	}
}

// Clock exposes the injected logical clock for tests.
func (s *Service) Clock() domain.Clock { return s.clock }

// CreateTask establishes a new calibration task in the created phase.
func (s *Service) CreateTask(key domain.TaskKey) (domain.MissionTask, error) {
	t := domain.MissionTask{
		Key:                key,
		Phase:              domain.PhaseCreated,
		CreatedLogicalTime: s.clock.Now(),
	}
	if err := s.store.CreateTask(t); err != nil {
		return domain.MissionTask{}, err
	}
	return t, nil
}

// Freeze locks the configuration into place. It validates the profile, slots
// and geometry, computes the canonical digest and rejects a stale
// expected_version. On success the task advances to the config-frozen phase.
func (s *Service) Freeze(key domain.TaskKey, cfg catalog.FrozenConfiguration, expectedVersion int64) (string, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return "", err
	}
	if task.Phase != domain.PhaseCreated {
		return "", domain.NewError(domain.CodeStageOutOfOrder, "task is already frozen or past freezing")
	}
	if expectedVersion != cfg.Version {
		return "", domain.NewError(domain.CodeConfigStale, "expected_version does not match configuration version")
	}

	if err := cfg.Profile.Validate(); err != nil {
		switch {
		case errors.Is(err, catalog.ErrProfileGap):
			return "", domain.NewError(domain.CodeProfileGap, err.Error())
		case errors.Is(err, catalog.ErrProfileOverlap):
			return "", domain.NewError(domain.CodeProfileOverlap, err.Error())
		default:
			return "", domain.NewError(domain.CodeInvalidRequest, err.Error())
		}
	}
	if err := catalog.ValidateSlots(cfg.Slots); err != nil {
		return "", domain.NewError(domain.CodeSlotCollision, err.Error())
	}
	if err := catalog.ValidateGeometry(cfg.ReferencePoints, cfg.Transponders, cfg.Lines); err != nil {
		switch {
		case errors.Is(err, catalog.ErrGraphDisconnected):
			return "", domain.NewError(domain.CodeGraphDisconnected, err.Error())
		case errors.Is(err, catalog.ErrGeometryDegenerate):
			return "", domain.NewError(domain.CodeGeometryDegenerate, err.Error())
		default:
			return "", domain.NewError(domain.CodeGeometryDegenerate, err.Error())
		}
	}

	digest := cfg.Digest()
	if err := s.store.FreezeTask(key, cfg, digest, cfg.Version); err != nil {
		return "", err
	}
	return digest, nil
}

// AcquireBindings atomically binds transponder identities and leases the rig
// resources, then advances the task to the bindings-acquired phase.
func (s *Service) AcquireBindings(key domain.TaskKey, idemKey string, req resources.AcquireRequest) (resources.AcquireResult, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return resources.AcquireResult{}, err
	}
	if task.Phase != domain.PhaseConfigFrozen {
		return resources.AcquireResult{}, domain.NewError(domain.CodeStageOutOfOrder, "bindings require a frozen configuration")
	}
	res, err := s.leases.Acquire(key, idemKey, req)
	if err != nil {
		return resources.AcquireResult{}, err
	}
	if task.Phase == domain.PhaseConfigFrozen {
		if err := s.store.AdvancePhase(key, domain.PhaseConfigFrozen, domain.PhaseBindingsAcquired); err != nil {
			return resources.AcquireResult{}, err
		}
	}
	return res, nil
}

// RenewLease extends a resource lease to the given logical time under its
// token.
func (s *Service) RenewLease(token string, until domain.LogicalTime) (domain.ResourceLease, error) {
	return s.leases.Renew(token, until)
}
