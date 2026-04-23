package generate

// CanonicalizeWorkingDirError is returned when GenerateStacks cannot resolve
// opts.WorkingDir to a cleaned, absolute, symlink-resolved identity. The
// Path field carries the original value for caller diagnostics; Err wraps
// the underlying failure from util.CanonicalResolvedPath.
type CanonicalizeWorkingDirError struct {
	Err  error
	Path string
}

// Error renders the path and wrapped cause for logs.
func (e *CanonicalizeWorkingDirError) Error() string {
	return "canonicalize working dir " + e.Path + ": " + e.Err.Error()
}

// Unwrap exposes the underlying filesystem error so callers can use
// errors.Is / errors.As against lower-level error values.
func (e *CanonicalizeWorkingDirError) Unwrap() error {
	return e.Err
}
