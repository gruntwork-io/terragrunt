package redesign_test

import "errors"

// failingWriter always returns an error from Write, for exercising
// write-failure branches.
type failingWriter struct{}

func (failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}
