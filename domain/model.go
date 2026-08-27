package domain

// TaskKey is the stable aggregate identity of a calibration task: the voyage,
// lander and generation jointly form the unique key.
type TaskKey struct {
	VoyageID   string
	LanderID   string
	Generation int
}

// MissionTask is the calibration aggregate root. It records the generation,
// the current phase, the frozen configuration digest and the terminal state.
type MissionTask struct {
	Key                TaskKey
	Phase              Phase
	ConfigVersion      int64
	FrozenDigest       string
	TerminalState      TerminalState
	CreatedLogicalTime LogicalTime
	CurrentEpoch       int64
}

// TerminalState is the immutable outcome of a terminal decision. The zero
// value means "not yet decided".
type TerminalState string

const (
	TerminalNone      TerminalState = ""
	TerminalAdmitted  TerminalState = "admitted"
	TerminalIsolated  TerminalState = "isolated"
	TerminalCancelled TerminalState = "cancelled"
)

// TerminalStateSet lists every legal terminal state, in stable order.
var TerminalStateSet = []TerminalState{
	TerminalAdmitted,
	TerminalIsolated,
	TerminalCancelled,
}

// ClockEpoch is a strictly increasing time epoch. A resynchronisation, drift
// violation or device replacement closes the current epoch and opens a new one;
// valid evidence from an old epoch is never copied forward.
type ClockEpoch struct {
	Key         TaskKey
	Epoch       int64
	Reason      string
	ClockSource string
	DriftUS     int64
	StartTime   LogicalTime
	EndTime     LogicalTime
}

// Epoch reasons.
const (
	EpochReasonInitial     = "initial"
	EpochReasonResync      = "resync"
	EpochReasonDrift       = "drift_exceeded"
	EpochReasonReplacement = "replacement"
	EpochReasonWrap        = "sequence_wrap"
)

// EvidenceKind classifies an append-only timestamp evidence record.
type EvidenceKind string

const (
	EvidenceTransmit EvidenceKind = "transmit"
	EvidenceReceive  EvidenceKind = "receive"
	EvidenceLate     EvidenceKind = "received_late"
	EvidenceRejected EvidenceKind = "rejected"
)

// TimestampEvidence is one append-only record in the
// transponder—epoch—sequence evidence chain. Valid receive records share a
// unique key of (transponder, epoch, sequence) within a task.
type TimestampEvidence struct {
	Key           TaskKey
	Transponder   string
	Epoch         int64
	Sequence      uint64
	Line          string
	Kind          EvidenceKind
	TransmitUS    LogicalTime
	ReceiveUS     LogicalTime
	Valid         bool
	ContentDigest string
	RecordedAt    LogicalTime
}

// ResourceType enumerates the five exclusive resource classes that must be
// leased before the rig is exercised.
type ResourceType string

const (
	ResourceSink               ResourceType = "sink"
	ResourceReferenceClockPort ResourceType = "reference_clock_port"
	ResourceTransmitChannel    ResourceType = "transmit_channel"
	ResourceReceiveChannel     ResourceType = "receive_channel"
	ResourceCalibrationStation ResourceType = "calibration_station"
)

// TransponderBinding binds a physical transponder serial to a mount point for
// exactly one task generation. A serial may be bound at most once while active.
type TransponderBinding struct {
	Key               TaskKey
	Serial            string
	MountPoint        string
	BindingGeneration int64
	BoundAt           LogicalTime
}

// ResourceLease is a time-bounded exclusive lease over a resource, judged only
// against logical time. An unexpired lease blocks every other task.
type ResourceLease struct {
	ResourceType ResourceType
	ResourceID   string
	Key          TaskKey
	LeaseToken   string
	StartTime    LogicalTime
	EndTime      LogicalTime
	Version      int64
}

// ExpiredAt reports whether the lease has lapsed at the given logical time.
func (l ResourceLease) ExpiredAt(now LogicalTime) bool {
	return now >= l.EndTime
}

// RetryCall records one scheduled device retry attempt. Device failures only
// append these records; they never fabricate evidence.
type RetryCall struct {
	Key       TaskKey
	Device    DeviceKind
	CallSeq   int64
	Attempt   int
	NextTime  LogicalTime
	LastError string
}

// RecalibrationBatch is the single, deterministically ordered set of affected
// transponders and lines that must be re-measured in a new epoch.
type RecalibrationBatch struct {
	Key                  TaskKey
	BatchSeq             int64
	Reason               string
	AffectedTransponders []string
	AffectedLines        []string
	NewEpoch             int64
	CreatedAt            LogicalTime
}

// Review is one reviewer's independent sign-off against a specific
// configuration and solve digest.
type Review struct {
	Key          TaskKey
	ReviewerID   string
	ConfigDigest string
	SolveDigest  string
	ReviewedAt   LogicalTime
}

// TerminalDecision is the immutable outcome of the single-writer terminal
// barrier. Exactly one decision commits per task.
type TerminalDecision struct {
	Key              TaskKey
	BarrierSeq       int64
	State            TerminalState
	CredentialDigest string
	DecidedAt        LogicalTime
}

// Credential is the unique, queryable but non-overwritable deployment
// admission credential.
type Credential struct {
	Key      TaskKey
	Digest   string
	IssuedAt LogicalTime
}

// ReviewQualification is a snapshot of a reviewer's validity at logical time.
type ReviewQualification struct {
	ReviewerID string
	ValidUntil LogicalTime
}

// QualifiedAt reports whether the qualification is valid at the given time.
func (q ReviewQualification) QualifiedAt(now LogicalTime) bool {
	return now < q.ValidUntil
}

// IdempotencyRecord stores the canonical content digest of a completed
// idempotent operation so that identical retries return the original result.
type IdempotencyRecord struct {
	Key            string
	ContentDigest  string
	ResponseDigest string
	RecordedAt     LogicalTime
}
