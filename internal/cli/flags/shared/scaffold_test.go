package shared_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootFileNameFlag pins that --root-file-name stores a non-empty name and rejects an empty one.
func TestRootFileNameFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		value        string
		wantRootName string
		wantExitErr  bool
	}{
		{name: "custom name", value: "root.hcl", wantRootName: "root.hcl"},
		{name: "unit config name", value: "terragrunt.hcl", wantRootName: "terragrunt.hcl"},
		{name: "empty name", value: "", wantExitErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.TerragruntConfigPath = "terragrunt.hcl"

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
		})
	}
}
