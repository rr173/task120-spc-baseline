package domain

import "errors"

// Sentinel errors for the SPC engine. Callers wrap these via fmt.Errorf("...:
// %w", ErrXxx) so httpapi.writeServiceError can map them to status codes.
var (
	// ErrInvalid means a request failed a structural validation (bad chart type,
	// subgroup size mismatch, defectives out of [0,n], negative amount...).
	// Maps to 400.
	ErrInvalid = errors.New("invalid input")

	// ErrNotFound means the referenced chart or measurement does not exist.
	// Maps to 404.
	ErrNotFound = errors.New("not found")

	// ErrInvariant means a business invariant was violated (subgroup_seq not
	// strictly increasing, capability preconditions unmet, zero sigma...).
	// Maps to 422.
	ErrInvariant = errors.New("invariant violation")

	// ErrConflict means a state transition is not allowed in the current state
	// (e.g. archiving an already-archived chart, restoring a non-excluded
	// point that is already included). Maps to 409.
	ErrConflict = errors.New("state conflict")

	// ErrTerminal means the chart is archived and rejects further mutation.
	// Maps to 409.
	ErrTerminal = errors.New("chart archived")

	// ErrNotEstimable means a capability index cannot be computed because the
	// preconditions (spec present, >=2 baseline points, sigma_within>0...) are
	// not met. It is a normal status, not a hard error: callers surface it as a
	// 200 with status:"not_estimable" rather than a 4xx.
	ErrNotEstimable = errors.New("not estimable")
)
