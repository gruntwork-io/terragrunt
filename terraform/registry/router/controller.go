package router

// Controller is an interface implemented by a REST controller
type Controller interface {
	// Register is the method called by the router, passing the router
	// groups to let the controller register its methods
	// The length of the groups received is equal with the length of the
	// relative paths returned by Paths
	Register(router *Router)
}
