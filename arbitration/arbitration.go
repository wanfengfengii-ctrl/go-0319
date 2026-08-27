// Package arbitration implements the recalibration and terminal arbitration
// components: anomaly impact-closure computation, dual-review validation and
// the single-writer terminal barrier that issues the unique deployment
// credential.
package arbitration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/persistence"
)

// Service coordinates recalibration, review and terminal arbitration.
type Service struct {
	store persistence.Store
	clock domain.Clock
}

// New constructs an arbitration Service.
func New(store persistence.Store, clock domain.Clock) *Service {
	return &Service{store: store, clock: clock}
}

// ComputeClosure computes the deterministic minimal impact closure from a set
// of seed (anomalous) transponders by propagating over the constraint graph:
// two transponders are adjacent when they share a reference point. The result
// is a sorted set of affected transponders and all lines touching them.
func ComputeClosure(lines []catalog.Line, seedTransponders []string) ([]string, []string) {
	adj := make(map[string]map[string]bool)
	for _, l := range lines {
		if adj[l.Transponder] == nil {
			adj[l.Transponder] = make(map[string]bool)
		}
		adj[l.Transponder][l.Reference] = true
	}
	// Build transponder adjacency via shared references.
	refToTp := make(map[string]map[string]bool)
	for _, l := range lines {
		if refToTp[l.Reference] == nil {
			refToTp[l.Reference] = make(map[string]bool)
		}
		refToTp[l.Reference][l.Transponder] = true
	}

	affected := make(map[string]bool)
	queue := append([]string(nil), seedTransponders...)
	for _, t := range seedTransponders {
		affected[t] = true
	}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		refs := adj[t]
		var shared []string
		for ref := range refs {
			for other := range refToTp[ref] {
				if other != t {
					shared = append(shared, other)
				}
			}
		}
		for _, other := range shared {
			if !affected[other] {
				affected[other] = true
				queue = append(queue, other)
			}
		}
	}

	var transponders []string
	for t := range affected {
		transponders = append(transponders, t)
	}
	sort.Strings(transponders)

	lineSet := make(map[string]bool)
	for _, l := range lines {
		if affected[l.Transponder] {
			lineSet[l.ID] = true
		}
	}
	var affectedLines []string
	for id := range lineSet {
		affectedLines = append(affectedLines, id)
	}
	sort.Strings(affectedLines)

	return transponders, affectedLines
}

// BuildRecalibration computes the impact closure for the current residual
// anomalies, opens a new epoch and stores the recalibration batch, advancing
// the task to the recalibration-done phase.
func (s *Service) BuildRecalibration(key domain.TaskKey, reason string) (domain.RecalibrationBatch, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.RecalibrationBatch{}, err
	}
	if task.Phase != domain.PhaseSolved {
		return domain.RecalibrationBatch{}, domain.NewError(domain.CodeStageOutOfOrder, "recalibration requires solved phase with residual anomalies")
	}
	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return domain.RecalibrationBatch{}, err
	}
	sr, err := s.store.GetSolve(key)
	if err != nil {
		return domain.RecalibrationBatch{}, err
	}

	var seeds []string
	seen := make(map[string]bool)
	for _, r := range sr.Residuals {
		if r.ResidualMM > cfg.ResidualThresholdMM && !seen[r.Transponder] {
			seen[r.Transponder] = true
			seeds = append(seeds, r.Transponder)
		}
	}
	transponders, lines := ComputeClosure(cfg.Lines, seeds)

	prev, found, err := s.store.LatestRecalibration(key)
	if err != nil {
		return domain.RecalibrationBatch{}, err
	}
	seq := int64(1)
	if found {
		seq = prev.BatchSeq + 1
	}
	newEpoch := task.CurrentEpoch + 1

	b := domain.RecalibrationBatch{
		Key:                  key,
		BatchSeq:             seq,
		Reason:               reason,
		AffectedTransponders: transponders,
		AffectedLines:        lines,
		NewEpoch:             newEpoch,
		CreatedAt:            s.clock.Now(),
	}
	if err := s.store.PutRecalibration(b); err != nil {
		return domain.RecalibrationBatch{}, err
	}
	if err := s.store.AdvancePhase(key, domain.PhaseSolved, domain.PhaseRecalibrationDone); err != nil {
		return domain.RecalibrationBatch{}, err
	}
	return b, nil
}

// credentialDigest produces the unique, non-overwritable admission credential
// digest for a terminal admission.
func credentialDigest(key domain.TaskKey, barrierSeq int64, at domain.LogicalTime) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d|%d", key.VoyageID, key.LanderID, key.Generation, barrierSeq, at)))
	return hex.EncodeToString(sum[:])
}
