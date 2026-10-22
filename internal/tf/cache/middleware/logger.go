package middleware

import (
	"cmp"
	"net/http"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

// Logger returns middleware that logs every request the cache server handles,
// with the download route's secret segment scrubbed from the recorded URI.
// It answers a failed request itself so the entry carries the status the
// client saw.
func Logger(l log.Logger, downloadSegment string) router.MiddlewareFunc {
	return func(w router.ResponseWriter, r *http.Request, next router.HandlerFunc) error {
		err := next(w, r)
		if err != nil {
			router.WriteError(w, err)
		}

		// Failed requests are logged at error level, and those logs end up in
		// bug reports, where the segment would be a working download URL.
		uri := strings.ReplaceAll(r.RequestURI, downloadSegment, "REDACTED")

		// A handler that returns without writing leaves net/http to send a 200.
		status := cmp.Or(w.Status(), http.StatusOK)

		entry := l.
			WithField(placeholders.CacheServerURLKeyName, uri).
			WithField(placeholders.CacheServerStatusKeyName, status)
		if err != nil {
			entry.Errorf(
				"Cache server was unable to process the received request, %s",
				err.Error(),
			)

			return nil
		}

		entry.Tracef("Cache server received request")

		return nil
	}
}
