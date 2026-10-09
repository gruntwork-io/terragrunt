package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/stretchr/testify/assert"
)

func TestRouterUnmatchedStatus(t *testing.T) {
	t.Parallel()

	r := router.New()
	r.Group("v1").GET("/providers/{rest...}", func(w router.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)

		return nil
	})

	testCases := []struct {
		name       string
		method     string
		target     string
		wantAllow  string
		wantStatus int
	}{
		{
			name:       "get on route",
			method:     http.MethodGet,
			target:     "/v1/providers/a/b",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "head on route",
			method:     http.MethodHead,
			target:     "/v1/providers/a/b",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "post on route",
			method:     http.MethodPost,
			target:     "/v1/providers/a/b",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  "GET, HEAD",
		},
		{
			name:       "get off route",
			method:     http.MethodGet,
			target:     "/v2/providers/a/b",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "post off route",
			method:     http.MethodPost,
			target:     "/v2/providers/a/b",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tc.method, tc.target, nil))

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantAllow, rec.Header().Get("Allow"))
		})
	}
}
