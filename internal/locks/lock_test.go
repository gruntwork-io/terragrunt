package locks_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/locks"
	"github.com/stretchr/testify/assert"
)

func TestEnvLockAllowsConcurrentReadersWithRacing(t *testing.T) {
	t.Parallel()

	locks.EnvLock.RLock()
	defer locks.EnvLock.RUnlock()

	assert.True(t, locks.EnvLock.TryRLock(), "environment readers must remain concurrent")
	locks.EnvLock.RUnlock()

	assert.False(t, locks.EnvLock.TryLock(), "an environment writer must not overlap a reader")
}
