// Package locks contains global locks used throughout Terragrunt.
package locks

import "sync"

// EnvLock coordinates process environment reads and writes that must observe
// one stable set of values.
//
// When possible, prefer to spawn a new process with the environment variables
// you want, or avoid setting environment variables instead of using this lock.
var EnvLock sync.RWMutex //nolint:gochecknoglobals
