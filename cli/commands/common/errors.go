// Package common provides common code that are used by many commands.
package common

var _ error = new(AllGraphFlagsError)

type AllGraphFlagsError byte

func (err *AllGraphFlagsError) Error() string {
	return "Using the `--all` and `--graph` flags simultaneously is not supported."
}
