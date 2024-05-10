package middleware

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func Logger() echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log := log.WithField("uri", v.URI).WithField("status", v.Status)
			if v.Error != nil {
				log.Errorf("Cache server was unable to process the received request, %s", v.Error.Error())
			} else {
				log.Tracef("Cache server received request")
			}
			return nil
		},
	})
}
