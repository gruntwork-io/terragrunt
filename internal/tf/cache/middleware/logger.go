package middleware

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Logger returns middleware that logs every request the cache server handles,
// with the download route's secret segment scrubbed from the recorded URI.
func Logger(logger log.Logger, downloadSegment string) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(_ echo.Context, req middleware.RequestLoggerValues) error {
			// Failed requests are logged at error level, and those logs end up in
			// bug reports, where the segment would be a working download URL.
			uri := strings.ReplaceAll(req.URI, downloadSegment, "REDACTED")

			logger := logger.
				WithField(placeholders.CacheServerURLKeyName, uri).
				WithField(placeholders.CacheServerStatusKeyName, req.Status)
			if req.Error != nil {
				logger.Errorf(
					"Cache server was unable to process the received request, %s",
					req.Error.Error(),
				)
			} else {
				logger.Tracef("Cache server received request")
			}

			return nil
		},
	})
}
