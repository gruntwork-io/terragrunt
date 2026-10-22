// Package errfmt renders errors for display to users.
package errfmt

import (
	"errors"
	"fmt"
	"strings"
)

// maxDepth caps how far Format descends into nested errors, so a deeply nested
// chain cannot exhaust the stack. An error reached at the cap renders as its
// Error method returns.
const maxDepth = 100

// Format renders err for display.
//
// An error built by [errors.Join] renders as a bulleted list headed by the
// number of errors it holds. Nested joins flatten into that one list, and
// continuation lines of a multi-line error are indented under its bullet. A
// wrapper whose message ends with the message of the error it wraps keeps its
// own prefix and renders the wrapped error the same way. Any other error
// renders as its Error method returns.
func Format(err error) string {
	return format(err, 0)
}

func format(err error, depth int) string {
	msg := err.Error()
	if depth >= maxDepth {
		return msg
	}

	if errs, ok := joined(err, msg); ok {
		return formatList(flatten(errs, depth+1))
	}

	inner := errors.Unwrap(err)
	if inner == nil {
		return msg
	}

	prefix, ok := strings.CutSuffix(msg, inner.Error())
	if !ok {
		return msg
	}

	return prefix + format(inner, depth+1)
}

// joined returns the errors err holds when err was built by [errors.Join].
//
// fmt.Errorf with several %w verbs also returns an error that unwraps to a
// slice, but its message interleaves its own text. Comparing msg against the
// newline-joined messages of the slice tells the two apart, so such an error
// renders as its message rather than being split into bullets.
func joined(err error, msg string) ([]error, bool) {
	u, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return nil, false
	}

	errs := u.Unwrap()
	if len(errs) == 0 {
		return nil, false
	}

	msgs := make([]string, len(errs))

	for i, e := range errs {
		if e == nil {
			return nil, false
		}

		msgs[i] = e.Error()
	}

	if strings.Join(msgs, "\n") != msg {
		return nil, false
	}

	return errs, true
}

// listItem is an error in a flattened list, paired with the depth at which
// flatten found it so that formatting it spends only the depth that remains.
type listItem struct {
	err   error
	depth int
}

func flatten(errs []error, depth int) []listItem {
	var flat []listItem

	for _, err := range errs {
		if depth < maxDepth {
			if nested, ok := joined(err, err.Error()); ok {
				flat = append(flat, flatten(nested, depth+1)...)
				continue
			}
		}

		flat = append(flat, listItem{err: err, depth: depth})
	}

	return flat
}

func formatList(items []listItem) string {
	strs := make([]string, len(items))
	for i, item := range items {
		strs[i] = indent(format(item.err, item.depth))
	}

	body := strings.Join(strs, "\n\n")

	if len(strs) == 1 {
		return fmt.Sprintf("error occurred:\n\n%s\n", body)
	}

	return fmt.Sprintf("%d errors occurred:\n\n%s\n", len(strs), body)
}

func indent(str string) string {
	str = strings.ReplaceAll(str, "\r\n", "\n")

	return "* " + strings.ReplaceAll(str, "\n", "\n  ")
}
