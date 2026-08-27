package calibration

import "github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"

// runDevice executes one scripted device call with retry accounting. A failing
// call appends a retry record and returns a stable device error; the phase is
// never advanced and no evidence is fabricated. Exhausted retries leave the
// task in its current phase.
func (s *Service) runDevice(key domain.TaskKey, kind domain.DeviceKind, retryMax int) (domain.DeviceResult, error) {
	existing := 0
	calls, err := s.store.ListRetryCalls(key)
	if err != nil {
		return domain.DeviceResult{}, err
	}
	for _, c := range calls {
		if c.Device == kind {
			existing++
		}
	}

	res := s.devices.Call(kind, existing, s.clock.Now())
	if res.Valid {
		return res, nil
	}

	code := res.Err
	if code == "" {
		code = domain.CodeDeviceRejected
	}
	next := res.RetryTime
	if next <= s.clock.Now() {
		next = s.clock.Now() + 1
	}
	rc := domain.RetryCall{
		Key:       key,
		Device:    kind,
		CallSeq:   int64(existing),
		Attempt:   existing,
		NextTime:  next,
		LastError: string(code),
	}
	if existing < retryMax {
		if err := s.store.PutRetryCall(rc); err != nil {
			return res, err
		}
	}
	return res, domain.NewError(code, "device call failed for "+string(kind))
}

// DisciplineClock disciplines the reference clock and, when drift is within
// threshold, opens the initial clock epoch. A failing or drifting device stays
// in the bindings-acquired phase.
func (s *Service) DisciplineClock(key domain.TaskKey) (domain.ClockEpoch, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.ClockEpoch{}, err
	}
	if task.Phase != domain.PhaseBindingsAcquired {
		return domain.ClockEpoch{}, domain.NewError(domain.CodeStageOutOfOrder, "discipline requires bindings acquired")
	}
	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return domain.ClockEpoch{}, err
	}

	res, err := s.runDevice(key, domain.DeviceReferenceClock, cfg.RetryMax)
	if err != nil {
		return domain.ClockEpoch{}, err
	}
	if res.DriftUS > cfg.DriftThresholdUS {
		return domain.ClockEpoch{}, domain.NewError(domain.CodeClockDriftExceeded, "reference clock drift exceeds threshold")
	}

	now := s.clock.Now()
	e := domain.ClockEpoch{
		Key:         key,
		Epoch:       task.CurrentEpoch + 1,
		Reason:      domain.EpochReasonInitial,
		ClockSource: cfg.ClockSource,
		DriftUS:     res.DriftUS,
		StartTime:   now,
		EndTime:     now,
	}
	if err := s.store.CreateEpoch(e); err != nil {
		return domain.ClockEpoch{}, err
	}
	if err := s.store.SetCurrentEpoch(key, e.Epoch); err != nil {
		return domain.ClockEpoch{}, err
	}
	if err := s.store.AdvancePhase(key, domain.PhaseBindingsAcquired, domain.PhaseClockDisciplined); err != nil {
		return domain.ClockEpoch{}, err
	}
	return e, nil
}

// ConfirmLoopback confirms the static loopback path and advances the task to
// the loopback-confirmed phase.
func (s *Service) ConfirmLoopback(key domain.TaskKey) error {
	task, err := s.store.GetTask(key)
	if err != nil {
		return err
	}
	if task.Phase != domain.PhaseClockDisciplined {
		return domain.NewError(domain.CodeStageOutOfOrder, "loopback requires a disciplined clock")
	}
	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return err
	}
	if _, err := s.runDevice(key, domain.DeviceTransponder, cfg.RetryMax); err != nil {
		return err
	}
	return s.store.AdvancePhase(key, domain.PhaseClockDisciplined, domain.PhaseLoopbackConfirmed)
}

// ResyncClock re-disciplines the reference clock and opens a new epoch. It is
// used after drift recovery, resynchronisation or device replacement.
func (s *Service) ResyncClock(key domain.TaskKey) (domain.ClockEpoch, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.ClockEpoch{}, err
	}
	if task.Phase < domain.PhaseClockDisciplined {
		return domain.ClockEpoch{}, domain.NewError(domain.CodeStageOutOfOrder, "resync requires a prior epoch")
	}
	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return domain.ClockEpoch{}, err
	}
	if _, err := s.runDevice(key, domain.DeviceReferenceClock, cfg.RetryMax); err != nil {
		return domain.ClockEpoch{}, err
	}
	return s.OpenEpoch(key, domain.EpochReasonResync)
}
