package domain

// Clock is a logical-time source. Resource leases, stage transitions and retry
// scheduling depend solely on logical time supplied by this interface, never on
// wall clock.
type Clock interface {
	Now() LogicalTime
}
