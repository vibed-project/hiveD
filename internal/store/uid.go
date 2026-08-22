package store

import (
	"crypto/rand"
	"fmt"
)

// newUID returns a random UUIDv4-formatted string. Postgres assigns UIDs
// via gen_random_uuid() at the database layer; MemoryStore needs its own
// generator to keep the same Resource.UID contract.
func newUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Errorf("store: reading random bytes for uid: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
