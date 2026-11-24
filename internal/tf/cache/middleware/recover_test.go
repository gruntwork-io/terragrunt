package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/middleware"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecover(t *testing.T) {
	t.Parallel()

	errPanic := errors.New("handler panicked")

	testCases := []struct {
		next      router.HandlerFunc
		assertErr func(t *testing.T, err error)
		name      string
	}{
		{
			name: "panic with an error returns that error",
			next: func(router.ResponseWriter, *http.Request) error {
				panic(errPanic)
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errPanic)
			},
		},
		{
			name: "panic with a value returns an error",
			next: func(router.ResponseWriter, *http.Request) error {
				panic("not an error")
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.Error(t, err)
			},
		},
		{
			name: "no panic returns the handler result",
			next: func(router.ResponseWriter, *http.Request) error {
				return nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.NoError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

			tc.assertErr(t, middleware.Recover(logger.CreateLogger())(router.NewResponseWriter(rec), req, tc.next))
		})
	}
}

func TestRecoverPanicBecomesServerError(t *testing.T) {
	t.Parallel()

	r := router.New()
	r.Use(middleware.Recover(logger.CreateLogger()))
	r.GET("/panic", func(router.ResponseWriter, *http.Request) error {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code, "a panic should be answered as a server error")
}
