// Package router registers the cache server's routes on an [http.ServeMux]
// and turns the errors handlers return into responses.
package router

import (
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
)

// HandlerFunc serves one route. An error it returns becomes the response, as
// [WriteError] renders it, unless a response is already under way.
type HandlerFunc func(w ResponseWriter, r *http.Request) error

// MiddlewareFunc runs around a handler. It calls next to run the rest of the
// chain, or returns without calling it to stop the request there.
type MiddlewareFunc func(w ResponseWriter, r *http.Request, next HandlerFunc) error

// Router registers routes under a path prefix. Every Router derived from one
// [New] call shares a mux, a server, and a middleware chain.
type Router struct {
	*shared

	urlPath string
}

// shared is the state every Router derived from one New call has in common.
type shared struct {
	// Server serves the router. Whoever starts it records the bound address
	// in Server.Addr, which URL reports.
	Server *http.Server

	mux         *http.ServeMux
	middlewares []scopedMiddleware
}

type scopedMiddleware struct {
	fn     MiddlewareFunc
	prefix string
}

// New returns a Router mounted at "/". Requests that match no route still run
// the root middleware, so they are logged like any other, and end in a 405 when
// a GET to the same path would match a route, or a 404 otherwise.
func New() *Router {
	router := &Router{
		shared: &shared{
			mux: http.NewServeMux(),
		},
		urlPath: "/",
	}
	router.Server = &http.Server{Handler: router}

	router.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		router.serve(w, r, "/", router.unmatched)
	})

	return router
}

// Group returns a Router for urlPath nested under this one.
func (router *Router) Group(urlPath string) *Router {
	return &Router{
		shared:  router.shared,
		urlPath: path.Join(router.urlPath, urlPath),
	}
}

// URL returns the address this router is served at.
func (router *Router) URL() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   router.Server.Addr,
		Path:   router.urlPath,
	}
}

// Register registers each controller's routes.
func (router *Router) Register(controllers ...Controller) {
	for _, controller := range controllers {
		controller.Register(router)
	}
}

// Use adds middleware to every route under this router's path, whether
// registered before or after the call. Middleware runs in the order it was
// added, so the first added sees the request first and the response last.
func (router *Router) Use(middlewares ...MiddlewareFunc) {
	for _, middleware := range middlewares {
		router.middlewares = append(router.middlewares, scopedMiddleware{
			prefix: router.urlPath,
			fn:     middleware,
		})
	}
}

// GET registers handler for GET requests to urlPath under this router's path.
// urlPath uses [http.ServeMux] pattern syntax, so "{name}" captures a segment
// and a trailing "{rest...}" captures the remainder.
func (router *Router) GET(urlPath string, handler HandlerFunc) {
	routePath := path.Join(router.urlPath, urlPath)

	router.mux.HandleFunc(http.MethodGet+" "+routePath, func(w http.ResponseWriter, r *http.Request) {
		router.serve(w, r, routePath, handler)
	})
}

// ServeHTTP implements [http.Handler].
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router.mux.ServeHTTP(w, r)
}

func (router *Router) serve(w http.ResponseWriter, r *http.Request, routePath string, handler HandlerFunc) {
	for _, m := range slices.Backward(router.middlewares) {
		if m.applies(routePath) {
			handler = m.wrap(handler)
		}
	}

	rw := NewResponseWriter(w)

	if err := handler(rw, r); err != nil {
		WriteError(rw, err)
	}
}

func (m scopedMiddleware) applies(routePath string) bool {
	return m.prefix == "/" ||
		routePath == m.prefix ||
		strings.HasPrefix(routePath, m.prefix+"/")
}

func (m scopedMiddleware) wrap(next HandlerFunc) HandlerFunc {
	return func(w ResponseWriter, r *http.Request) error {
		return m.fn(w, r, next)
	}
}

// unmatched answers a request no route took. The "/" fallback matches every
// method, so the mux never reaches its own 405; routes are only registered
// through GET, so asking the mux how it would route a GET tells the two apart.
func (router *Router) unmatched(w ResponseWriter, r *http.Request) error {
	get := r.Clone(r.Context())
	get.Method = http.MethodGet

	if _, pattern := router.mux.Handler(get); pattern == "/" {
		return NewHTTPError(http.StatusNotFound)
	}

	w.Header().Set("Allow", "GET, HEAD")

	return NewHTTPError(http.StatusMethodNotAllowed)
}
