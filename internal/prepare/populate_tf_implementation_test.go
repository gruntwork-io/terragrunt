package prepare_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTerraformVersionHandler answers a version probe the way a Terraform binary would;
// tests never assert on the version itself, only on user-visible outcomes.
func fakeTerraformVersionHandler(context.Context, vexec.Invocation) vexec.Result {
	return vexec.Result{Stdout: []byte("Terraform v1.15.3\non linux_amd64\n")}
}

// TestPopulateTFImplementation pins the detection boundary the provider cache server
// relies on: the probed binary's implementation lands on opts before InitServer reads it.
func TestPopulateTFImplementation(t *testing.T) {
	t.Parallel()

	t.Run("probes the binary and stores the result", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithHandler(fakeTerraformVersionHandler)

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TFPath = "terraform"

		require.NoError(t, prepare.PopulateTFImplementation(t.Context(), logger.CreateLogger(), v, opts))
		assert.Equal(t, tfimpl.Terraform, opts.TofuImplementation)
		assert.NotNil(t, opts.TerraformVersion)
	})

	t.Run("skips the probe when already populated", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TofuImplementation = tfimpl.OpenTofu
		opts.TerraformVersion = version.Must(version.NewVersion("1.0.0"))

		// venvtest's fail-closed exec errors on any spawn, so success proves no probe ran.
		require.NoError(t, prepare.PopulateTFImplementation(t.Context(), logger.CreateLogger(), venvtest.New(), opts))
		assert.Equal(t, tfimpl.OpenTofu, opts.TofuImplementation)
	})

	t.Run("returns the probe error and leaves opts untouched", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TFPath = "terraform"

		require.Error(t, prepare.PopulateTFImplementation(t.Context(), logger.CreateLogger(), venvtest.New(), opts))
		assert.Equal(t, tfimpl.Unknown, opts.TofuImplementation)
		assert.Nil(t, opts.TerraformVersion)
	})
}
