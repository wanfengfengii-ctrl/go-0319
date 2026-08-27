// Package persistence implements the SQLite-backed, WAL-mode store that
// provides real durability and crash recovery for the calibration domain. All
// multi-entity writes (freeze, bind+lease, solve publish, recalibration and
// terminal decision) commit inside a single transaction so a failure can never
// leave partial state.
package persistence

import (
	"errors"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/catalog"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
	"github.com/deep-sea-lander/acoustic-array-deployment-gate/solver"
)

// Store is the persistence contract consumed by the service layer.
type Store interface {
	// Lifecycle
	Close() error
	// Recover runs migrations and verifies (and self-heals) the snapshot
	// digest. It returns true when a recovery repair was performed.
	Recover() (repaired bool, err error)

	// Tasks
	CreateTask(t domain.MissionTask) error
	GetTask(key domain.TaskKey) (domain.MissionTask, error)
	AdvancePhase(key domain.TaskKey, from, to domain.Phase) error
	FreezeTask(key domain.TaskKey, cfg catalog.FrozenConfiguration, digest string, version int64) error
	GetConfig(key domain.TaskKey) (catalog.FrozenConfiguration, error)
	SetCurrentEpoch(key domain.TaskKey, epoch int64) error

	// Bindings, leases and idempotency (one transaction).
	AcquireBindingsAndLeases(key domain.TaskKey, bindings []domain.TransponderBinding, leases []domain.ResourceLease, idem domain.IdempotencyRecord, now domain.LogicalTime) error
	RenewLease(token string, until domain.LogicalTime) (domain.ResourceLease, error)
	GetIdempotency(idemKey string) (domain.IdempotencyRecord, error)

	// Epochs
	CreateEpoch(e domain.ClockEpoch) error
	CurrentEpoch(key domain.TaskKey) (domain.ClockEpoch, error)

	// Evidence
	AppendEvidence(e domain.TimestampEvidence) error
	ListEvidence(key domain.TaskKey) ([]domain.TimestampEvidence, error)
	HasValidReceive(key domain.TaskKey, epoch int64, transponder string, seq uint64) (bool, error)
	ValidReceiveDigest(key domain.TaskKey, epoch int64, transponder string, seq uint64) (string, bool, error)
	NextSequence(key domain.TaskKey, epoch int64, transponder string) (uint64, error)

	// Solve
	PublishSolve(key domain.TaskKey, sr solver.SolveResult) error
	GetSolve(key domain.TaskKey) (solver.SolveResult, error)

	// Retry calls
	PutRetryCall(rc domain.RetryCall) error
	ListRetryCalls(key domain.TaskKey) ([]domain.RetryCall, error)

	// Recalibration
	PutRecalibration(b domain.RecalibrationBatch) error
	LatestRecalibration(key domain.TaskKey) (domain.RecalibrationBatch, bool, error)

	// Reviews
	PutReview(r domain.Review) error
	ListReviews(key domain.TaskKey) ([]domain.Review, error)

	// Terminal + credential (single-writer barrier).
	DecideTerminal(key domain.TaskKey, barrierSeq int64, d domain.TerminalDecision, cred domain.Credential) error
	GetTerminal(key domain.TaskKey) (domain.TerminalDecision, error)
	GetCredential(key domain.TaskKey) (domain.Credential, error)
}

// ErrNotFound is returned when a requested aggregate or record does not exist.
var ErrNotFound = errors.New("persistence: not found")
