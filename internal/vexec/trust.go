package vexec

import "context"

// trustedCommandKey is the context key [WithTrustedCommand] sets.
type trustedCommandKey struct{}

// WithTrustedCommand returns a copy of ctx that marks the commands started with
// it as Terragrunt's own. An [Exec] that restricts which commands may run can
// let marked commands through by checking [IsTrustedCommand].
//
// Every context derived from the returned one keeps the mark. Never mark a
// context that starts a command named by configuration or other input, since
// the mark exempts that command from the restriction.
func WithTrustedCommand(ctx context.Context) context.Context {
	return context.WithValue(ctx, trustedCommandKey{}, true)
}

// IsTrustedCommand reports whether [WithTrustedCommand] marked ctx.
func IsTrustedCommand(ctx context.Context) bool {
	v, ok := ctx.Value(trustedCommandKey{}).(bool)

	return ok && v
}
