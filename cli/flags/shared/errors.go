package shared

// AllGraphFlagsError is returned when both --all and --graph flags are used simultaneously.
type AllGraphFlagsError byte

func (err *AllGraphFlagsError) Error() string {
	return "Using the `--all` and `--graph` flags simultaneously is not supported."
}
