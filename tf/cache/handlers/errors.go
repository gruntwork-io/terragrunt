package handlers

type NotFoundWellKnownURLError struct {
	url string
}

func (err NotFoundWellKnownURLError) Error() string {
	return err.url + " not found"
}
