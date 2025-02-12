package log

import "context"

const (
	loggerContextKey ctxKey = iota
)

type ctxKey byte

func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

func LoggerFromContext(ctx context.Context) Logger {
	if val := ctx.Value(loggerContextKey); val != nil {
		if val, ok := val.(Logger); ok {
			return val
		}
	}

	return nil
}
