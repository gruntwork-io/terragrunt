package router

import (
	"net/url"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

type Router struct {
	*echo.Echo

	// urlPath is the router urlPath
	urlPath string
}

func New() *Router {
	return &Router{
		Echo:    echo.New(),
		urlPath: "/",
	}
}

func (router *Router) Group(urlPath string) *Router {
	return &Router{
		Echo:    router.Echo,
		urlPath: path.Join(router.urlPath, urlPath),
	}
}

func (router *Router) URL() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   router.Server.Addr,
		Path:   router.urlPath,
	}
}

// Register registers controller's endpoints
func (router *Router) Register(controllers ...Controller) {
	for _, controller := range controllers {
		controller.Register(router)
	}
}

// Use adds middleware to the chain which is run after router.
func (router *Router) Use(middlewares ...echo.MiddlewareFunc) {
	for _, middleware := range middlewares {
		middleware := func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(ctx echo.Context) error {
				if strings.HasPrefix(strings.Trim(ctx.Path(), "/"), strings.Trim(router.urlPath, "/")) {
					return middleware(next)(ctx)
				}
				return next(ctx)
			}
		}
		router.Echo.Use(middleware)
	}
}

// GET registers a new GET route for a path with matching handler in the router
// with optional route-level middleware.
func (router *Router) GET(urlPath string, handle echo.HandlerFunc) {
	router.Echo.GET(path.Join(router.urlPath, urlPath), handle)
}

// HEAD registers a new HEAD route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) HEAD(urlPath string, handle echo.HandlerFunc) {
	router.Echo.HEAD(path.Join(router.urlPath, urlPath), handle)
}

// OPTIONS registers a new OPTIONS route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) OPTIONS(urlPath string, handle echo.HandlerFunc) {
	router.Echo.OPTIONS(path.Join(router.urlPath, urlPath), handle)
}

// POST registers a new POST route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) POST(urlPath string, handle echo.HandlerFunc) {
	router.Echo.POST(path.Join(router.urlPath, urlPath), handle)
}

// PUT registers a new PUT route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) PUT(urlPath string, handle echo.HandlerFunc) {
	router.Echo.PUT(path.Join(router.urlPath, urlPath), handle)
}

// PATCH registers a new PATCH route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) PATCH(urlPath string, handle echo.HandlerFunc) {
	router.Echo.PATCH(path.Join(router.urlPath, urlPath), handle)
}

// DELETE registers a new DELETE route for a path with matching handler in the router
// with optional route-level middleware.
func (router *Router) DELETE(urlPath string, handle echo.HandlerFunc) {
	router.Echo.DELETE(path.Join(router.urlPath, urlPath), handle)
}
