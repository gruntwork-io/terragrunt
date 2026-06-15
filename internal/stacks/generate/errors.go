package generate

import "strconv"

// MaxPathLengthExceededError is returned when stack generation discovers a stack
// file whose path length exceeds [generationMaxPath], which indicates stacks
// recursively generating themselves. Unix systems eventually stop such runaway
// recursion with ENAMETOOLONG once PATH_MAX is exceeded, but Windows supports
// extended-length paths, so without this guard a cyclic configuration keeps
// generating until the disk fills.
type MaxPathLengthExceededError struct {
	Path  string
	Limit int
}

func (e *MaxPathLengthExceededError) Error() string {
	return "Cycle detected: maximum path length (" + strconv.Itoa(e.Limit) + ") exceeded at " + e.Path
}

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
