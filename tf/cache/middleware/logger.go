package middleware

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func Logger(logger log.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(_ echo.Context, req middleware.RequestLoggerValues) error {
			l := logger.
				WithField(placeholders.CacheServerURLKeyName, req.URI).
				WithField(placeholders.CacheServerStatusKeyName, req.Status)
			if req.Error != nil {
				l.Errorf("Cache server was unable to process the received request, %s", req.Error.Error())
			} else {
				l.Tracef("Cache server received request")
			}
			return nil
		},
	})
}
