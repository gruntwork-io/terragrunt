// Package services provides the interface for services
// that can be run in the background.
package services

import (
	"context"
)

type Service interface {
	// Init prepares the service to answer requests. The server calls it
	// before it starts serving, so no request observes a half-built service.
	// It is safe to call more than once; only the first call does the work.
	Init() error

	Run(ctx context.Context) error
}
