// Package identity defines the Principal a request carries and the
// Verifier seam that resolves a bearer token to one. Real PASETO v4
// issuance and verification (ADR-0002) ship in M1; StubVerifier is the
// only implementation in M0 and accepts any token, so an M0 Keeper must
// never be exposed on an untrusted network (see SECURITY.md).
package identity

import "context"

// Principal is the caller identity attached to a request context after
// auth. Subject is opaque in M0 (always "dev"); Colony is empty until
// real tokens carry one.
type Principal struct {
	Subject string
	Colony  string
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Verifier resolves a bearer token (as presented in an Authorization
// header) to the Principal it grants.
type Verifier interface {
	Verify(ctx context.Context, bearerToken string) (Principal, error)
}

// StubVerifier accepts any token, including an empty one, and always
// grants the same dev Principal. It exists so the M0 API surface and its
// interceptor chain are already shaped for real auth; only this type
// needs to be replaced in M1.
type StubVerifier struct{}

func (StubVerifier) Verify(_ context.Context, _ string) (Principal, error) {
	return Principal{Subject: "dev"}, nil
}

var _ Verifier = StubVerifier{}
