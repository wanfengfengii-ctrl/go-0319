package domain

// DeviceKind identifies a scriptable hardware device in the calibration rig.
type DeviceKind string

const (
	DeviceReferenceClock DeviceKind = "reference_clock"
	DeviceTransmitter    DeviceKind = "transmitter"
	DeviceReceiver       DeviceKind = "receiver"
	DeviceCTD            DeviceKind = "ctd"
	DeviceTransponder    DeviceKind = "transponder"
)

// DeviceResult is the raw, append-only outcome of a single device call. A
// failing call sets Retry, schedules the next attempt at RetryTime and
// classifies the failure with a stable code in Err.
type DeviceResult struct {
	Kind      DeviceKind
	Attempt   int
	Raw       []byte
	Valid     bool
	Retry     bool
	RetryTime LogicalTime
	Err       ErrorCode
	DriftUS   int64
}

// DeviceAdapter executes scripted or real device calls. The HTTP API injects a
// scripted adapter for tests and development; production replaces it with real
// hardware clients without changing the calibration domain.
type DeviceAdapter interface {
	Call(kind DeviceKind, attempt int, now LogicalTime) DeviceResult
}
