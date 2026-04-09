package log_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextWithLogger(t *testing.T) {
	t.Parallel()

	logger := log.New()
	ctx := log.ContextWithLogger(t.Context(), logger)

	retrieved := log.LoggerFromContext(ctx)
	require.NotNil(t, retrieved)
	assert.Equal(t, logger, retrieved)
}

func TestLoggerFromContextEmpty(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	retrieved := log.LoggerFromContext(ctx)
	assert.Nil(t, retrieved)
}
