package strict

import "context"

const (
	controlsContextKey ctxKey = iota
)

type ctxKey byte

func ContextWithControls(ctx context.Context, controls Controls) context.Context {
	return context.WithValue(ctx, controlsContextKey, controls)
}

func ControlsFromContext(ctx context.Context) Controls {
	if val := ctx.Value(controlsContextKey); val != nil {
		if val, ok := val.(Controls); ok {
			return val
		}
	}

	return nil
}
