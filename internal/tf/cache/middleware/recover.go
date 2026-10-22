package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Recover returns middleware that turns a panic in a handler into the error
// the handler would have returned, logging the stack at debug level.
func Recover(l log.Logger) router.MiddlewareFunc {
	return func(w router.ResponseWriter, r *http.Request, next router.HandlerFunc) (er error) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			l.Debug(string(debug.Stack()))

			err, isErr := rec.(error)
			if !isErr {
				err = fmt.Errorf("%v", rec)
			}

			er = err
		}()

		return next(w, r)
	}
}
