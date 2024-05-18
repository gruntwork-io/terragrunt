package services

import (
	"context"
)

type Service interface {
	Run(ctx context.Context) error
}
