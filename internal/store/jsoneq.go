package store

import (
	"encoding/json"
	"reflect"
)

// jsonEqual reports whether two JSON byte strings are structurally
// equivalent, independent of key order or whitespace. Empty/nil is treated
// as equal to itself only.
func jsonEqual(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
