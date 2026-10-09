package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
)

const bearerScheme = "Bearer "

// KeyAuth returns middleware that admits only requests carrying token as a
// bearer token in the Authorization header. A request whose header is missing
// or uses another scheme gets a 400, and one with the wrong token a 401.
//
// The token guards the cache server against connections from other processes
// on the same host. Its value can be any text.
func KeyAuth(token string) router.MiddlewareFunc {
	return func(w router.ResponseWriter, r *http.Request, next router.HandlerFunc) error {
		if err := authorize(r, token); err != nil {
			return err
		}

		return next(w, r)
	}
}

func authorize(r *http.Request, token string) error {
	header := r.Header.Get("Authorization")
	if len(header) <= len(bearerScheme) || !strings.EqualFold(header[:len(bearerScheme)], bearerScheme) {
		return &router.HTTPError{
			Code:    http.StatusBadRequest,
			Message: "missing key in request header",
		}
	}

	key := header[len(bearerScheme):]
	if subtle.ConstantTimeCompare([]byte(key), []byte(token)) != 1 {
		return router.NewHTTPError(http.StatusUnauthorized)
	}

	return nil
}
