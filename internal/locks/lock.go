// Package locks contains global locks used throughout Terragrunt.
package locks

import "sync"

// EnvLock is the lock acquired when writing environment variables in a way
// that is not safe for concurrent access.
//
// When possible, prefer to spawn a new process with the environment variables
// you want, or avoid setting environment variables instead of using this lock.
var EnvLock sync.Mutex //nolint:gochecknoglobals
