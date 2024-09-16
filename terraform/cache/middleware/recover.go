package middleware

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/labstack/echo/v4"
)

func Recover(logger log.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) (er error) {
			defer errors.Recover(func(err error) {
				logger.Debugf(errors.PrintErrorWithStackTrace(err))
				er = err
			})

			return next(ctx)
		}
	}
}
