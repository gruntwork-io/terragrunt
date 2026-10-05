package run

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProtectedModuleRunCfg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		extraArgs []string
		protected bool
	}{
		{
			name:      "destroy",
			args:      []string{"destroy"},
			protected: true,
		},
		{
			name:      "plan with -destroy",
			args:      []string{"plan", "-destroy"},
			protected: true,
		},
		{
			name:      "plan with -destroy=true",
			args:      []string{"plan", "-destroy=true"},
			protected: true,
		},
		{
			name:      "plan with -destroy=false",
			args:      []string{"plan", "-destroy=false"},
			protected: false,
		},
		{
			name:      "plan with -destroy=true from extra_arguments",
			args:      []string{"plan"},
			extraArgs: []string{"-destroy=true"},
			protected: true,
		},
		{
			name:      "plan",
			args:      []string{"plan"},
			protected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := &Options{
				TerraformCliArgs:     iacargs.New(tt.args...),
				TerragruntConfigPath: "/work/terragrunt.hcl",
			}

			// The arguments of `extra_arguments` are inserted after the command, like this.
			opts.InsertTerraformCliArgs(tt.extraArgs...)

			err := checkProtectedModuleRunCfg(opts, &runcfg.RunConfig{PreventDestroy: true})

			if !tt.protected {
				require.NoError(t, err)

				return
			}

			var target ModuleIsProtected

			require.ErrorAs(t, err, &target)
			assert.Equal(t, opts.TerragruntConfigPath, target.ConfigPath)
		})
	}
}
