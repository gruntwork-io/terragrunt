package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/middleware"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/stretchr/testify/assert"
)

func TestKeyAuth(t *testing.T) {
	t.Parallel()

	const token = "secret-token"

	testCases := []struct {
		name          string
		authorization string
		wantStatus    int
		wantNext      bool
	}{
		{
			name:       "missing header",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:          "other scheme",
			authorization: "Basic " + token,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "bearer scheme without a key",
			authorization: "Bearer ",
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "wrong token",
			authorization: "Bearer wrong-token",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "matching token",
			authorization: "Bearer " + token,
			wantStatus:    http.StatusNoContent,
			wantNext:      true,
		},
		{
			name:          "scheme is case insensitive",
			authorization: "bearer " + token,
			wantStatus:    http.StatusNoContent,
			wantNext:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var nextCalled bool

			r := router.New()
			r.Use(middleware.KeyAuth(token))
			r.GET("/resource", func(w router.ResponseWriter, _ *http.Request) error {
				nextCalled = true

				w.WriteHeader(http.StatusNoContent)

				return nil
			})

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/resource", nil)
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code, "status should reflect the authorization outcome")
			assert.Equal(t, tc.wantNext, nextCalled, "handler should run only for an authorized request")
		})
	}
}
