package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/middleware"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerAnswersFailedRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		next       router.HandlerFunc
		name       string
		wantStatus int
	}{
		{
			name: "handler error becomes the response",
			next: func(router.ResponseWriter, *http.Request) error {
				return router.NewHTTPError(http.StatusTeapot)
			},
			wantStatus: http.StatusTeapot,
		},
		{
			name: "handler status is kept",
			next: func(w router.ResponseWriter, _ *http.Request) error {
				w.WriteHeader(http.StatusAccepted)

				return nil
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "handler that writes nothing",
			next:       func(router.ResponseWriter, *http.Request) error { return nil },
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/downloads/segment/file.zip", nil)

			err := middleware.Logger(logger.CreateLogger(), "segment")(router.NewResponseWriter(rec), req, tc.next)

			require.NoError(t, err, "logger should answer the request itself and report no error")
			assert.Equal(t, tc.wantStatus, rec.Code)
		})
	}
}
