package arbitration

import (
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// AddReview records one independent reviewer's sign-off against the current
// configuration and solve digests. It rejects unqualified reviewers, repeated
// reviewers and reviewers signing stale digests. Two distinct qualified
// reviewers advance the task to the reviewed phase.
func (s *Service) AddReview(key domain.TaskKey, reviewerID string) (domain.Review, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.Review{}, err
	}
	if task.Phase != domain.PhaseRecalibrationDone && task.Phase != domain.PhaseReviewed {
		return domain.Review{}, domain.NewError(domain.CodeStageOutOfOrder, "review requires recalibration done")
	}
	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return domain.Review{}, err
	}
	sr, err := s.store.GetSolve(key)
	if err != nil {
		return domain.Review{}, err
	}

	now := s.clock.Now()
	var qual *domain.ReviewQualification
	for i := range cfg.ReviewQualifications {
		if cfg.ReviewQualifications[i].ReviewerID == reviewerID {
			qual = &cfg.ReviewQualifications[i]
			break
		}
	}
	if qual == nil {
		return domain.Review{}, domain.NewError(domain.CodeReviewNotReady, "reviewer not in qualification snapshot")
	}
	if !qual.QualifiedAt(now) {
		return domain.Review{}, domain.NewError(domain.CodeReviewNotReady, "reviewer qualification expired")
	}

	reviews, err := s.store.ListReviews(key)
	if err != nil {
		return domain.Review{}, err
	}
	for _, r := range reviews {
		if r.ReviewerID == reviewerID {
			return domain.Review{}, domain.NewError(domain.CodeReviewNotReady, "reviewer already reviewed")
		}
	}

	r := domain.Review{
		Key:          key,
		ReviewerID:   reviewerID,
		ConfigDigest: cfg.Digest(),
		SolveDigest:  sr.InputDigest,
		ReviewedAt:   now,
	}
	if err := s.store.PutReview(r); err != nil {
		return domain.Review{}, err
	}

	reviews, _ = s.store.ListReviews(key)
	if task.Phase == domain.PhaseRecalibrationDone && len(reviews) >= 2 {
		if err := s.store.AdvancePhase(key, domain.PhaseRecalibrationDone, domain.PhaseReviewed); err != nil {
			return domain.Review{}, err
		}
	}
	return r, nil
}
