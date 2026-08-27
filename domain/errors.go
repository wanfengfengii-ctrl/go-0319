package domain

// ErrorCode is a stable, machine-readable failure code. HTTP responses use the
// shape {code, message, details} where code is one of these values and details
// is a deterministically sorted structure.
type ErrorCode string

const (
	CodeConfigStale            ErrorCode = "CONFIG_STALE"
	CodeProfileGap             ErrorCode = "PROFILE_GAP"
	CodeProfileOverlap         ErrorCode = "PROFILE_OVERLAP"
	CodeSlotCollision          ErrorCode = "SLOT_COLLISION"
	CodeGraphDisconnected      ErrorCode = "GRAPH_DISCONNECTED"
	CodeGeometryDegenerate     ErrorCode = "GEOMETRY_DEGENERATE"
	CodeArithmeticOverflow     ErrorCode = "ARITHMETIC_OVERFLOW"
	CodeDuplicateIdentity      ErrorCode = "DUPLICATE_IDENTITY"
	CodeResourceBusy           ErrorCode = "RESOURCE_BUSY"
	CodeLeaseExpired           ErrorCode = "LEASE_EXPIRED"
	CodeIdempotencyConflict    ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeDeviceRejected         ErrorCode = "DEVICE_REJECTED"
	CodeDeviceTimeout          ErrorCode = "DEVICE_TIMEOUT"
	CodeDeviceDisconnected     ErrorCode = "DEVICE_DISCONNECTED"
	CodeCalibrationExpired     ErrorCode = "CALIBRATION_EXPIRED"
	CodeDeviceFormatInvalid    ErrorCode = "DEVICE_FORMAT_INVALID"
	CodeDeviceResponseMismatch ErrorCode = "DEVICE_RESPONSE_MISMATCH"
	CodeClockDriftExceeded     ErrorCode = "CLOCK_DRIFT_EXCEEDED"
	CodeEpochMismatch          ErrorCode = "EPOCH_MISMATCH"
	CodeSequenceDuplicate      ErrorCode = "SEQUENCE_DUPLICATE"
	CodeSequenceConflict       ErrorCode = "SEQUENCE_CONFLICT"
	CodeSequenceWrap           ErrorCode = "SEQUENCE_WRAP"
	CodeStageOutOfOrder        ErrorCode = "STAGE_OUT_OF_ORDER"
	CodeTerminalAlreadyDecided ErrorCode = "TERMINAL_ALREADY_DECIDED"
	CodeTaskNotFound           ErrorCode = "TASK_NOT_FOUND"
	CodeInvalidRequest         ErrorCode = "INVALID_REQUEST"
	CodeInsufficientData       ErrorCode = "INSUFFICIENT_CONSTRAINTS"
	CodeReviewNotReady         ErrorCode = "REVIEW_NOT_READY"
	CodeTerminalNotReady       ErrorCode = "TERMINAL_NOT_READY"
)

// DomainError is the single error type surfaced by the service layer. It
// carries a stable code, a human message and a deterministically sorted detail
// structure for the HTTP response.
type DomainError struct {
	Code    ErrorCode
	Message string
	Details any
}

func (e *DomainError) Error() string {
	if e.Message != "" {
		return string(e.Code) + ": " + e.Message
	}
	return string(e.Code)
}

// NewError builds a DomainError with a code and message.
func NewError(code ErrorCode, msg string) *DomainError {
	return &DomainError{Code: code, Message: msg}
}

// WithDetails attaches a stable detail structure.
func (e *DomainError) WithDetails(d any) *DomainError {
	e.Details = d
	return e
}
