package arbitration

import (
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// DecideTerminal competes for the single-writer terminal barrier. An admit
// requires the reviewed phase and two valid independent reviews; isolate and
// cancel require at least the recalibration-done phase. Exactly one decision
// commits; competitors observe the same committed terminal and fail with
// TERMINAL_ALREADY_DECIDED.
func (s *Service) DecideTerminal(key domain.TaskKey, state domain.TerminalState) (domain.TerminalDecision, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.TerminalDecision{}, err
	}
	if task.TerminalState != domain.TerminalNone {
		existing, err := s.store.GetTerminal(key)
		if err != nil {
			return domain.TerminalDecision{}, err
		}
		return existing, domain.NewError(domain.CodeTerminalAlreadyDecided, "terminal already decided: "+string(existing.State))
	}

	switch state {
	case domain.TerminalAdmitted:
		if task.Phase != domain.PhaseReviewed {
			return domain.TerminalDecision{}, domain.NewError(domain.CodeTerminalNotReady, "admit requires reviewed phase")
		}
	case domain.TerminalIsolated, domain.TerminalCancelled:
		if task.Phase < domain.PhaseRecalibrationDone {
			return domain.TerminalDecision{}, domain.NewError(domain.CodeTerminalNotReady, "isolate/cancel require recalibration done")
		}
	default:
		return domain.TerminalDecision{}, domain.NewError(domain.CodeInvalidRequest, "unknown terminal state")
	}

	barrierSeq := int64(1)

	now := s.clock.Now()
	d := domain.TerminalDecision{
		Key:        key,
		BarrierSeq: barrierSeq,
		State:      state,
		DecidedAt:  now,
	}
	cred := domain.Credential{Key: key, IssuedAt: now}
	if state == domain.TerminalAdmitted {
		cred.Digest = credentialDigest(key, barrierSeq, now)
		d.CredentialDigest = cred.Digest
	}

	if err := s.store.DecideTerminal(key, barrierSeq, d, cred); err != nil {
		return domain.TerminalDecision{}, err
	}
	return d, nil
}
