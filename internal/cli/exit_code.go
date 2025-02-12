package cli

// Constants for exit codes.
const (
	ExitCodeSuccess ExitCode = iota
	ExitCodeGeneralError
	ExitCodeUsageError  // Invalid command usage (2)
	ExitCodeDataError   // Input data error (65)
	ExitCodeConfigError // Configuration error (78)
)

// ExitCode is a number between 0 and 255, which is returned by any Unix command when it returns control to its parent process.
type ExitCode byte

// ExitCoder is the interface checked by `App` and `Command` for a custom exit code.
type ExitCoder interface {
	error
	ExitCode() int
	Unwrap() error
}
