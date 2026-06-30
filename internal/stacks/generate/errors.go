package generate

// CanonicalizeWorkingDirError is returned when resolving opts.WorkingDir fails.
type CanonicalizeWorkingDirError struct {
	Err  error
	Path string
}

func (e *CanonicalizeWorkingDirError) Error() string {
	return "canonicalize working dir " + e.Path + ": " + e.Err.Error()
}

func (e *CanonicalizeWorkingDirError) Unwrap() error {
	return e.Err
}
