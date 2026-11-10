package handlers

// NotFoundWellKnownURLError reports a registry that answered its service discovery request with
// 404 or 500. [IsOfflineError] treats it as offline.
type NotFoundWellKnownURLError struct {
	url string
}

func (err NotFoundWellKnownURLError) Error() string {
	return err.url + " not found"
}
