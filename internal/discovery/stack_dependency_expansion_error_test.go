package discovery_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Direct typed-error contract test: StackDependencyExpansionError must carry the depPath and unwrap cleanly to the original parser error.
func TestStackDependencyExpansionError_Unwrap(t *testing.T) {
	t.Parallel()

	innerErr := hclparse.MalformedDependencyError{
		FilePath: "/some/path/terragrunt.autoinclude.hcl",
		Name:     "vpc",
		Reason:   "missing config_path attribute",
	}

	wrapped := discovery.NewStackDependencyExpansionError("/path/to/dep", innerErr)
	require.Error(t, wrapped)
	assert.Contains(t, wrapped.Error(), "/path/to/dep")
	assert.Contains(t, wrapped.Error(), "missing config_path")

	// errors.As must reach both the wrapper and the underlying typed error.
	var expansion discovery.StackDependencyExpansionError
	require.ErrorAs(t, wrapped, &expansion)
	assert.Equal(t, "/path/to/dep", expansion.DepPath)

	var malformed hclparse.MalformedDependencyError
	require.ErrorAs(t, wrapped, &malformed)
	assert.Equal(t, "vpc", malformed.Name)

	// errors.Is must reach the leaf via Unwrap chain.
	require.ErrorIs(t, wrapped, innerErr)
}
