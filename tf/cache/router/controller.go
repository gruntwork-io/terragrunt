package router

import "net/url"

// Controller is an interface implemented by a REST controller
type Controller interface {
	// Register is the method called by the router, passing the router
	// groups to let the controller register its methods
	Register(router *Router)

	// URL returns the controller fqdn.
	URL() *url.URL
}
