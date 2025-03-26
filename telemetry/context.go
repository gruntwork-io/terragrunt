package telemetry

import "context"

const (
	telemeterContextKey ctxKey = iota
)

type ctxKey byte

func ContextWithTelemeter(ctx context.Context, tlm *Telemeter) context.Context {
	return context.WithValue(ctx, telemeterContextKey, tlm)
}

func TelemeterFromContext(ctx context.Context) *Telemeter {
	if val := ctx.Value(telemeterContextKey); val != nil {
		if val, ok := val.(*Telemeter); ok {
			return val
		}
	}

	return nil
}
