package calibration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/deep-sea-lander/acoustic-array-deployment-gate/domain"
)

// EchoOutcome classifies the result of recording one echo.
type EchoOutcome int

const (
	// EchoAccepted means the echo became valid receive evidence.
	EchoAccepted EchoOutcome = iota
	// EchoLate means the echo arrived for an older epoch and was appended as
	// invalid audit evidence only.
	EchoLate
	// EchoDuplicate means an identical valid receive already exists.
	EchoDuplicate
)

// RecordTransmission registers a planned transmission and advances the task
// into the ranging phase. Sequence numbers are allocated in frozen slot order
// per (transponder, epoch); reaching the configured upper bound closes the
// epoch and starts a new one.
func (s *Service) RecordTransmission(key domain.TaskKey, transponder, line string, transmitUS domain.LogicalTime) (domain.TimestampEvidence, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return domain.TimestampEvidence{}, err
	}
	if task.Phase == domain.PhaseLoopbackConfirmed {
		if err := s.store.AdvancePhase(key, domain.PhaseLoopbackConfirmed, domain.PhaseRanging); err != nil {
			return domain.TimestampEvidence{}, err
		}
	} else if task.Phase != domain.PhaseRanging {
		return domain.TimestampEvidence{}, domain.NewError(domain.CodeStageOutOfOrder, "transmission requires ranging phase")
	}

	cfg, err := s.store.GetConfig(key)
	if err != nil {
		return domain.TimestampEvidence{}, err
	}

	epoch := task.CurrentEpoch
	if epoch <= 0 {
		return domain.TimestampEvidence{}, domain.NewError(domain.CodeStageOutOfOrder, "no clock epoch established")
	}

	seq, err := s.store.NextSequence(key, epoch, transponder)
	if err != nil {
		return domain.TimestampEvidence{}, err
	}
	if cfg.SequenceMax > 0 && seq >= cfg.SequenceMax {
		if _, err := s.OpenEpoch(key, domain.EpochReasonWrap); err != nil {
			return domain.TimestampEvidence{}, err
		}
		epoch++
		seq = 0
	}

	ev := domain.TimestampEvidence{
		Key:           key,
		Transponder:   transponder,
		Epoch:         epoch,
		Sequence:      seq,
		Line:          line,
		Kind:          domain.EvidenceTransmit,
		TransmitUS:    transmitUS,
		Valid:         false,
		ContentDigest: echoDigest(key, transponder, epoch, seq, line, transmitUS, 0),
		RecordedAt:    s.clock.Now(),
	}
	if err := s.store.AppendEvidence(ev); err != nil {
		return domain.TimestampEvidence{}, err
	}
	return ev, nil
}

// RecordEcho records a received echo, which may arrive out of order. It
// enforces the single-valid-receive-per-(transponder, epoch, sequence) rule:
// an identical duplicate is idempotent, a different duplicate is a conflict,
// and a late old-epoch echo is appended only as invalid audit evidence.
func (s *Service) RecordEcho(key domain.TaskKey, epoch int64, transponder string, sequence uint64, line string, transmitUS, receiveUS domain.LogicalTime) (EchoOutcome, domain.TimestampEvidence, error) {
	task, err := s.store.GetTask(key)
	if err != nil {
		return 0, domain.TimestampEvidence{}, err
	}
	if task.Phase != domain.PhaseRanging {
		return 0, domain.TimestampEvidence{}, domain.NewError(domain.CodeStageOutOfOrder, "echo requires ranging phase")
	}

	if epoch != task.CurrentEpoch {
		if epoch < task.CurrentEpoch {
			ev := domain.TimestampEvidence{
				Key:           key,
				Transponder:   transponder,
				Epoch:         epoch,
				Sequence:      sequence,
				Line:          line,
				Kind:          domain.EvidenceLate,
				TransmitUS:    transmitUS,
				ReceiveUS:     receiveUS,
				Valid:         false,
				ContentDigest: echoDigest(key, transponder, epoch, sequence, line, transmitUS, receiveUS),
				RecordedAt:    s.clock.Now(),
			}
			if err := s.store.AppendEvidence(ev); err != nil {
				return 0, domain.TimestampEvidence{}, err
			}
			return EchoLate, ev, nil
		}
		return 0, domain.TimestampEvidence{}, domain.NewError(domain.CodeEpochMismatch, "echo epoch is in the future")
	}

	digest := echoDigest(key, transponder, epoch, sequence, line, transmitUS, receiveUS)
	if existing, found, err := s.store.ValidReceiveDigest(key, epoch, transponder, sequence); err != nil {
		return 0, domain.TimestampEvidence{}, err
	} else if found {
		if existing == digest {
			return EchoDuplicate, domain.TimestampEvidence{}, nil
		}
		return 0, domain.TimestampEvidence{}, domain.NewError(domain.CodeSequenceConflict, "sequence already has a different valid receive")
	}

	ev := domain.TimestampEvidence{
		Key:           key,
		Transponder:   transponder,
		Epoch:         epoch,
		Sequence:      sequence,
		Line:          line,
		Kind:          domain.EvidenceReceive,
		TransmitUS:    transmitUS,
		ReceiveUS:     receiveUS,
		Valid:         true,
		ContentDigest: digest,
		RecordedAt:    s.clock.Now(),
	}
	if err := s.store.AppendEvidence(ev); err != nil {
		return 0, domain.TimestampEvidence{}, err
	}
	return EchoAccepted, ev, nil
}

// echoDigest computes the canonical content digest of one echo.
func echoDigest(key domain.TaskKey, transponder string, epoch int64, sequence uint64, line string, transmitUS, receiveUS domain.LogicalTime) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s|%d|%d|%s|%d|%d",
		key.VoyageID, key.LanderID, key.Generation, transponder, epoch, sequence, line, transmitUS, receiveUS)
	return hex.EncodeToString(h.Sum(nil))
}
