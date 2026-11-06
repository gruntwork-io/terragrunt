package router_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRouterMiddlewareScope(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		target    string
		wantTrace []string
	}{
		{
			name:      "route under the group runs root and group middleware in order",
			target:    "/v1/items",
			wantTrace: []string{"root-first", "root-second", "v1"},
		},
		{
			name:      "route under a sibling prefix runs only root middleware",
			target:    "/v1x/items",
			wantTrace: []string{"root-first", "root-second"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var trace []string

			record := func(name string) router.MiddlewareFunc {
				return func(w router.ResponseWriter, r *http.Request, next router.HandlerFunc) error {
					trace = append(trace, name)

					return next(w, r)
				}
			}

			ok := func(w router.ResponseWriter, _ *http.Request) error {
				w.WriteHeader(http.StatusNoContent)

				return nil
			}

			root := router.New()
			root.Use(record("root-first"), record("root-second"))

			v1 := root.Group("v1")
			v1.Use(record("v1"))
			v1.GET("/items", ok)

			// v1x shares the v1 prefix as a string but is a different path segment.
			root.Group("v1x").GET("/items", ok)

			rec := httptest.NewRecorder()
			root.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.target, nil))

			assert.Equal(t, http.StatusNoContent, rec.Code)
			assert.Equal(t, tc.wantTrace, trace)
		})
	}
}

func TestRouterHandlerErrorBecomesResponse(t *testing.T) {
	t.Parallel()

	r := router.New()
	r.GET("/fail", func(router.ResponseWriter, *http.Request) error {
		return router.NewHTTPError(http.StatusConflict)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/fail", nil))

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestRouterURLAndRegister(t *testing.T) {
	t.Parallel()

	root := router.New()
	root.Server.Addr = "127.0.0.1:5758"

	controller := &pathController{}
	root.Group("v1").Register(controller)

	require.NotNil(t, controller.registeredAt, "register should hand the router to the controller")
	assert.Equal(t, "http://127.0.0.1:5758/v1", controller.registeredAt.URL().String())
	assert.Equal(t, "http://127.0.0.1:5758/", root.URL().String())
}

// pathController records the router it was registered with.
type pathController struct {
	registeredAt *router.Router
}

func (c *pathController) Register(r *router.Router) {
	c.registeredAt = r
}

func (c *pathController) URL() *url.URL {
	return c.registeredAt.URL()
}
