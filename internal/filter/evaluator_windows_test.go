//go:build windows

package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWindowsFilterNativeSeparators covers issue #6214: on Windows, path, reading, and
// source filters matched glob patterns built with backslash separators against components
// in forward-slash space, so `--filter` found no affected units. Gated to Windows because
// filepath.ToSlash and filepath.Clean are no-ops on Unix separators.
func TestWindowsFilterNativeSeparators(t *testing.T) {
	t.Parallel()

	t.Run("path filter built from native separators matches forward-slash component", func(t *testing.T) {
		t.Parallel()

		// filepath.Dir on a git diff path yields backslashes on Windows; the resulting
		// path filter must still match the forward-slash component path.
		expr := mustPath(t, `apps\app1`)
		c := component.NewUnit("apps/app1")

		l := logger.CreateLogger()
		result, err := filter.Evaluate(l, expr, []component.Component{c})
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("reading filter matches native read paths", func(t *testing.T) {
		t.Parallel()

		c := component.NewUnit("apps/app1").WithReading(`shared\config.hcl`)
		expr := mustAttr(t, "reading", "shared/config.hcl")

		l := logger.CreateLogger()
		result, err := filter.Evaluate(l, expr, []component.Component{c})
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("source filter matches mixed separators from get_repo_root", func(t *testing.T) {
		t.Parallel()

		c := component.NewUnit("apps/app1").WithConfig(
			&config.TerragruntConfig{
				Terraform: &config.TerraformConfig{
					Source: helpers.PointerTo(`D:\a\1\s/IaC/modules/sns`),
				},
			},
		)
		expr := mustAttr(t, "source", "**/IaC/modules/sns")

		l := logger.CreateLogger()
		result, err := filter.Evaluate(l, expr, []component.Component{c})
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})
}
