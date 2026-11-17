package log

import "context"

const (
	loggerContextKey ctxKey = iota
)

type ctxKey byte

// ContextWithLogger returns a copy of ctx that carries logger for [LoggerFromContext].
func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// LoggerFromContext returns the logger stored in ctx by [ContextWithLogger], or
// nil when ctx has none.
func LoggerFromContext(ctx context.Context) Logger {
	if val := ctx.Value(loggerContextKey); val != nil {
		if val, ok := val.(Logger); ok {
			return val
		}
	}

	return nil
}
