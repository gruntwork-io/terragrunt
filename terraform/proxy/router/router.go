package router

import (
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

type Router struct {
	*echo.Echo

	// prefix is the router prefix
	prefix string
}

func New() *Router {
	return &Router{
		Echo:   echo.New(),
		prefix: "/",
	}
}

func (router *Router) Group(prefix string) *Router {
	return &Router{
		Echo:   router.Echo,
		prefix: path.Join(router.prefix, prefix),
	}
}

func (router *Router) Prefix() string {
	return router.prefix
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
				if strings.HasPrefix(ctx.Path(), router.prefix) {
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
func (router *Router) GET(prefix string, handle echo.HandlerFunc) {
	router.Echo.GET(path.Join(router.prefix, prefix), handle)
}

// HEAD registers a new HEAD route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) HEAD(prefix string, handle echo.HandlerFunc) {
	router.Echo.HEAD(path.Join(router.prefix, prefix), handle)
}

// OPTIONS registers a new OPTIONS route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) OPTIONS(prefix string, handle echo.HandlerFunc) {
	router.Echo.OPTIONS(path.Join(router.prefix, prefix), handle)
}

// POST registers a new POST route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) POST(prefix string, handle echo.HandlerFunc) {
	router.Echo.POST(path.Join(router.prefix, prefix), handle)
}

// PUT registers a new PUT route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) PUT(prefix string, handle echo.HandlerFunc) {
	router.Echo.PUT(path.Join(router.prefix, prefix), handle)
}

// PATCH registers a new PATCH route for a path with matching handler in the
// router with optional route-level middleware.
func (router *Router) PATCH(prefix string, handle echo.HandlerFunc) {
	router.Echo.PATCH(path.Join(router.prefix, prefix), handle)
}

// DELETE registers a new DELETE route for a path with matching handler in the router
// with optional route-level middleware.
func (router *Router) DELETE(prefix string, handle echo.HandlerFunc) {
	router.Echo.DELETE(path.Join(router.prefix, prefix), handle)
}
