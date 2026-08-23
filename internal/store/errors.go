package store

import "errors"

var (
	// ErrNotFound is returned by Get/ApplyStatus when no matching,
	// non-deleted resource exists.
	ErrNotFound = errors.New("store: resource not found")
	// ErrConflict is returned by Apply when ifResourceVersion is set and
	// does not match the currently stored resource_version.
	ErrConflict = errors.New("store: resource version conflict")
	// ErrImmutable is returned by Apply when a kind in ImmutableKinds
	// already exists with a different spec.
	ErrImmutable = errors.New("store: resource spec is immutable")
	// ErrInvalidPageToken is returned by List when the caller supplies a
	// page token that did not come from a previous ListResult.
	ErrInvalidPageToken = errors.New("store: invalid page token")
)
