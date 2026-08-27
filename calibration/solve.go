package calibration

import (
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/solver"
)

// Solve builds the current-epoch baseline constraints from valid receive
// evidence, runs the public integer baseline solve, and publishes the result.
// It advances the task to the solved phase and, when the residual check passes
// and no anomalies remain, through residual-passed to recalibration-done.
func (s *Service) Solve(key domain.TaskKey) (solver.SolveResult, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return solver.SolveResult{}, err
	}
	if task.Phase != domain.PhaseRanging {
		return solver.SolveResult{}, domain.NewError(domain.CodeStageOutOfOrder, "solve requires ranging phase")
	}
	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return solver.SolveResult{}, err
	}

	constraints, err := s.buildConstraints(key, cfg)
	if err != nil {
		return solver.SolveResult{}, err
	}
	if err := catalog.ValidateConstraints(cfg.ReferencePoints, cfg.Transponders, constraints); err != nil {
		return solver.SolveResult{}, domain.NewError(domain.CodeInsufficientData, err.Error())
	}

	res, err := solver.Solve(cfg, constraints)
	if err != nil {
		return solver.SolveResult{}, mapSolveError(err)
	}

	if err := s.store.PublishSolve(key, res); err != nil {
		return solver.SolveResult{}, err
	}
	if res.ResidualPassed {
		if err := s.store.AdvancePhase(key, domain.PhaseSolved, domain.PhaseResidualPassed); err != nil {
			return solver.SolveResult{}, err
		}
		if err := s.store.AdvancePhase(key, domain.PhaseResidualPassed, domain.PhaseRecalibrationDone); err != nil {
			return solver.SolveResult{}, err
		}
	}
	return res, nil
}

// buildConstraints derives one integer distance per line from the valid
// receive evidence of the current epoch.
func (s *Service) buildConstraints(key domain.TaskKey, cfg catalog.FrozenConfiguration) ([]catalog.BaselineConstraint, error) {
	evidence, err := s.store.ListEvidence(key)
	if err != nil {
		return nil, err
	}
	refByLine := make(map[string]string, len(cfg.Lines))
	for _, l := range cfg.Lines {
		refByLine[l.ID] = l.Reference
	}

	byLine := make(map[string]catalog.BaselineConstraint)
	for _, e := range evidence {
		if e.Kind != domain.EvidenceReceive || !e.Valid {
			continue
		}
		compensated := int64(e.ReceiveUS) - int64(e.TransmitUS) - cfg.TransducerDelayUS
		if compensated <= 0 {
			continue
		}
		topSpeed := cfg.Profile.Layers[0].SpeedMMS
		roundTrip, err := solver.SingleLayerDistance(topSpeed, compensated)
		if err != nil {
			return nil, err
		}
		distance := solver.BidirectionalDistance(roundTrip)
		ref, ok := refByLine[e.Line]
		if !ok {
			continue
		}
		byLine[e.Line] = catalog.BaselineConstraint{
			Reference:   ref,
			Transponder: e.Transponder,
			Line:        e.Line,
			DistanceMM:  distance,
			Weight:      1,
			Epoch:       e.Epoch,
		}
	}

	constraints := make([]catalog.BaselineConstraint, 0, len(byLine))
	for _, c := range byLine {
		constraints = append(constraints, c)
	}
	return constraints, nil
}

// mapSolveError translates solver failures into stable domain error codes.
func mapSolveError(err error) error {
	switch err {
	case solver.ErrArithmeticOverflow:
		return domain.NewError(domain.CodeArithmeticOverflow, err.Error())
	case solver.ErrInsufficientConstraints:
		return domain.NewError(domain.CodeInsufficientData, err.Error())
	default:
		return domain.NewError(domain.CodeGeometryDegenerate, err.Error())
	}
}
