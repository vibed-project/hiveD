package store

import "time"

// pollInterval and bookmarkInterval govern the M0 poll-based Watch
// implementation shared (in spirit) by MemoryStore and PostgresStore. M1
// replaces the Postgres side with LISTEN/NOTIFY behind the same
// since-cursor contract; callers of Watch are unaffected by that swap.
const (
	pollInterval     = 250 * time.Millisecond
	bookmarkInterval = 30 * time.Second
)

// classifyWatchEvent decides ADDED / MODIFIED / DELETED for a resource
// observed during a poll, given the set of UIDs this Watch session has
// already emitted at least one event for. A resource is DELETED whenever
// DeletedAt is set, even the first time it's observed in this session.
func classifyWatchEvent(r Resource, seen map[string]bool) WatchEventType {
	wasSeen := seen[r.UID]
	seen[r.UID] = true
	switch {
	case r.DeletedAt != nil:
		return WatchDeleted
	case !wasSeen:
		return WatchAdded
	default:
		return WatchModified
	}
}
