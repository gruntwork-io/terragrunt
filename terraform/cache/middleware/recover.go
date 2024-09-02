package middleware

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

func Recover(logger *logrus.Entry) echo.MiddlewareFunc {
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
