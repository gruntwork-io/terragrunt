package config

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/locks/dynamodb"
)

// lockFactory provides an implementation of Lock with the provided configuration map
type lockFactory func(map[string]string) (locks.Lock, error)

// ErrLockNotFound is the error returned if no Lock implementation could be found
// for the specified name
var ErrLockNotFound = fmt.Errorf("no Lock implementation found")

// lookupLock returns the implementation for the named lock or returns ErrLockNotFound
func lookupLock(name string, conf map[string]string) (locks.Lock, error) {
	factory, containsFactory := builtinLocks[name]
	if !containsFactory {
		return nil, errors.WithStackTrace(ErrLockNotFound)
	}

	return factory(conf)
}

var builtinLocks = map[string]lockFactory{
	"dynamodb": dynamodb.New,
}
