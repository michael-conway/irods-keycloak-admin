package authz

import (
	"context"
	"errors"
)

type Scope string

const (
	ScopeEvents      Scope = "events"
	ScopeProvision   Scope = "provision"
	ScopeMutate      Scope = "mutate"
	ScopePlan        Scope = "plan"
	ScopeApply       Scope = "apply"
	ScopeBootstrap   Scope = "bootstrap"
	ScopeRepair      Scope = "repair"
	ScopeDiagnostics Scope = "diagnostics"
)

var ErrPermissionDenied = errors.New("permission denied")

type Principal struct {
	Subject  string
	ClientID string
	Scopes   []Scope
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func RequireScope(ctx context.Context, required Scope) error {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return ErrPermissionDenied
	}
	for _, scope := range principal.Scopes {
		if scope == required {
			return nil
		}
	}
	return ErrPermissionDenied
}
