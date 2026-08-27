package calibration

import (
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// OpenEpoch closes the current epoch and opens a strictly increasing new one
// for the given reason. Old-epoch evidence remains read-only and is never
// copied forward.
func (s *Service) OpenEpoch(key domain.TaskKey, reason string) (domain.ClockEpoch, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.ClockEpoch{}, err
	}
	next := task.CurrentEpoch + 1

	var source string
	if cfg, err := s.store.GetConfig(key); err == nil {
		source = cfg.ClockSource
	}

	now := s.clock.Now()
	e := domain.ClockEpoch{
		Key:         key,
		Epoch:       next,
		Reason:      reason,
		ClockSource: source,
		StartTime:   now,
		EndTime:     now,
	}
	if err := s.store.CreateEpoch(e); err != nil {
		return domain.ClockEpoch{}, err
	}
	if err := s.store.SetCurrentEpoch(key, next); err != nil {
		return domain.ClockEpoch{}, err
	}
	return e, nil
}

// CurrentEpoch returns the task's active epoch.
func (s *Service) CurrentEpoch(key domain.TaskKey) (domain.ClockEpoch, error) {
	return s.store.CurrentEpoch(key)
}
