package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCASCloneDepth(t *testing.T) {
	t.Parallel()

	require.NoError(t, cas.ValidateCASCloneDepth(1))
	require.NoError(t, cas.ValidateCASCloneDepth(42))
	require.NoError(t, cas.ValidateCASCloneDepth(-1))
	assert.Error(t, cas.ValidateCASCloneDepth(0))
}
