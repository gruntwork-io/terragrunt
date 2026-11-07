package shared_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootFileNameFlag pins that --root-file-name stores a non-empty name, rejects an empty one, and rejects a terragrunt.hcl name under its strict control.
func TestRootFileNameFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		value        string
		wantRootName string
		strict       bool
		wantExitErr  bool
	}{
		{name: "custom name", value: "root.hcl", wantRootName: "root.hcl"},
		{name: "terragrunt.hcl name", value: "terragrunt.hcl", wantRootName: "terragrunt.hcl"},
		{name: "terragrunt.hcl name under its strict control", value: "terragrunt.hcl", strict: true, wantExitErr: true},
		{name: "terragrunt.hcl path under its strict control", value: "./terragrunt.hcl", strict: true, wantExitErr: true},
		{name: "custom name under the strict control", value: "root.hcl", strict: true, wantRootName: "root.hcl"},
		{name: "empty name", value: "", wantExitErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			if tc.strict {
				require.NoError(t, opts.StrictControls.EnableControl(controls.RootTerragruntHCL))
			}

			err := parseAndRunActions(
				t,
				shared.NewScaffoldingFlags(opts, nil),
				clihelper.Args{"--root-file-name", tc.value},
				map[string]string{},
			)

			if !tc.wantExitErr {
				require.NoError(t, err)
				assert.Equal(t, tc.wantRootName, opts.ScaffoldRootFileName, "root file name should be stored")

				return
			}

			var exitErr clihelper.ExitCoder
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())

			if !tc.strict {
				return
			}

			ctrl, ok := opts.StrictControls.Find(controls.RootTerragruntHCL).(*controls.Control)
			require.True(t, ok, "control %s should be a *controls.Control", controls.RootTerragruntHCL)
			require.ErrorIs(t, err, ctrl.Error, "the enabled control should reject a terragrunt.hcl root")
		})
	}
}
