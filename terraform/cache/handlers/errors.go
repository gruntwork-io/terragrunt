package handlers

import "fmt"

type NotFoundWellKnownURL struct {
	url string
}

func (err NotFoundWellKnownURL) Error() string {
	return fmt.Sprintf("%s not found", err.url)
}
