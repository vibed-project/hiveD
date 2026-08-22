package store

import "testing"

func TestMemoryStore_ResourceStoreConformance(t *testing.T) {
	runResourceStoreConformance(t, func(t *testing.T) ResourceStore {
		return NewMemoryStore()
	})
}

func TestMemoryStore_EventStoreConformance(t *testing.T) {
	runEventStoreConformance(t, func(t *testing.T) EventStore {
		return NewMemoryStore()
	})
}
