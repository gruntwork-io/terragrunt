// Package multierror aggregates several errors into a single error whose message
// renders each underlying error as a bulleted list. It is backed entirely by the
// standard library and exists to give run and run --all the human-readable
// aggregation output that plain [errors.Join] does not provide.
package multierror

import (
	"fmt"
	"strings"
)

// Error aggregates multiple errors into one. [Error.Error] renders the underlying
// errors as a bulleted list, and [Error.Unwrap] exposes them so that [errors.Is]
// and [errors.As] continue to traverse the aggregated errors.
type Error struct {
	errs []error
}

// Join collects the non-nil errors into a single [Error], flattening any nested
// aggregates (including errors produced by [errors.Join] and by Join itself) so
// that every error renders as a single top-level bullet. It returns nil when every
// given error is nil, mirroring [errors.Join].
func Join(errs ...error) error {
	flat := flatten(errs)
	if len(flat) == 0 {
		return nil
	}

	return &Error{errs: flat}
}

// Unwrap returns the aggregated errors so that [errors.Is] and [errors.As] can
// traverse them.
func (e *Error) Unwrap() []error {
	return e.errs
}

// Error renders the aggregated errors as a bulleted list.
func (e *Error) Error() string {
	strs := make([]string, len(e.errs))
	for i, err := range e.errs {
		strs[i] = indent(err.Error())
	}

	body := strings.Join(strs, "\n\n")

	if len(strs) == 1 {
		return fmt.Sprintf("error occurred:\n\n%s\n", body)
	}

	return fmt.Sprintf("%d errors occurred:\n\n%s\n", len(strs), body)
}

func flatten(errs []error) []error {
	var flat []error

	for _, err := range errs {
		if err == nil {
			continue
		}

		if joined, ok := err.(interface{ Unwrap() []error }); ok {
			flat = append(flat, flatten(joined.Unwrap())...)
			continue
		}

		flat = append(flat, err)
	}

	return flat
}

func indent(str string) string {
	// Normalize Windows line endings so the bullet formatting is consistent.
	str = strings.ReplaceAll(str, "\r\n", "\n")
	lines := strings.Split(str, "\n")

	for i, line := range lines {
		if i == 0 {
			lines[i] = "* " + line
			continue
		}

		lines[i] = "  " + line
	}

	return strings.Join(lines, "\n")
}
