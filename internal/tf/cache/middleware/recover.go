package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/labstack/echo/v4"
)

func Recover(logger log.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) (er error) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				logger.Debug(string(debug.Stack()))

				err, isErr := rec.(error)
				if !isErr {
					err = fmt.Errorf("%v", rec)
				}

				er = err
			}()

			return next(ctx)
		}
	}
}
