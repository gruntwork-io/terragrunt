// Package services provides the interface for services
// that can be run in the background.
package services

import (
	"context"
)

type Service interface {
	Run(ctx context.Context) error
}
