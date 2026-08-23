package store

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// MaxPageSize bounds a single List/ListEvents page. Page size is fully
// client-controlled, so without a ceiling one request can ask for every row
// in the table and the Keeper accumulates all of them in memory before
// replying.
const MaxPageSize = 1000

// DefaultPageSize is used when a caller does not ask for a specific size.
const DefaultPageSize = 500

// clampPageSize turns a caller-supplied page size into a safe one. Callers
// pass int32 straight off the wire, so this also keeps arithmetic on the
// result away from the int32 boundary.
func clampPageSize(n int32) int32 {
	switch {
	case n <= 0:
		return DefaultPageSize
	case n > MaxPageSize:
		return MaxPageSize
	default:
		return n
	}
}

// Page tokens encode the (colony, name) of the last row on the previous page.
// Both halves are opaque, caller-supplied strings with no charset validation,
// so they are joined on a NUL (which cannot appear in a Postgres text value)
// and base64-encoded rather than concatenated with a printable separator.
const pageTokenSep = "\x00"

func encodePageToken(colony, name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(colony + pageTokenSep + name))
}

// decodePageToken returns the (colony, name) cursor a token carries. An empty
// token means "start from the beginning" and yields empty strings; callers
// must therefore test the token itself, not the decoded values, to decide
// whether to apply the cursor predicate.
func decodePageToken(token string) (colony, name string, err error) {
	if token == "" {
		return "", "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", fmt.Errorf("%w: page token is not valid base64", ErrInvalidPageToken)
	}
	colony, name, found := strings.Cut(string(raw), pageTokenSep)
	if !found {
		return "", "", fmt.Errorf("%w: page token is malformed", ErrInvalidPageToken)
	}
	return colony, name, nil
}
