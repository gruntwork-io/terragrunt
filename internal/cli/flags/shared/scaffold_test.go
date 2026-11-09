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

// TestRootFileNameFlag pins that --root-file-name stores a non-empty name and rejects an empty one.
func TestRootFileNameFlag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		value  string
		want   rootFileNameOutcome
		strict bool
	}{
		{name: "custom name", value: "root.hcl"},
		{name: "terragrunt.hcl name", value: "terragrunt.hcl", want: rootFileNameWarned},
		{
			name:   "terragrunt.hcl name under its strict control",
			value:  "terragrunt.hcl",
			strict: true,
			want:   rootFileNameRejected,
		},
		{
			name:   "terragrunt.hcl path under its strict control",
			value:  "./terragrunt.hcl",
			strict: true,
			want:   rootFileNameRejected,
		},
		{
			name:   "terragrunt.hcl in a parent directory under its strict control",
			value:  "../terragrunt.hcl",
			strict: true,
			want:   rootFileNameRejected,
		},
		{
			name:   "terragrunt.hcl in a subdirectory under its strict control",
			value:  "sub/terragrunt.hcl",
			strict: true,
			want:   rootFileNameRejected,
		},
		{name: "differently cased name is stored", value: "TERRAGRUNT.HCL", strict: true},
		{name: "custom name under the strict control", value: "root.hcl", strict: true},
		{name: "empty name", value: "", want: rootFileNameEmpty},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())

			ctrl, ok := opts.StrictControls.Find(controls.RootTerragruntHCL).(*controls.Control)
			require.True(t, ok, "control %s should be a *controls.Control", controls.RootTerragruntHCL)

			if tc.strict {
				require.NoError(t, opts.StrictControls.EnableControl(controls.RootTerragruntHCL))
			}

			err := parseAndRunActions(
				t,
				shared.NewScaffoldingFlags(opts, nil),
				clihelper.Args{"--root-file-name", tc.value},
				map[string]string{},
			)

			if tc.want == rootFileNameStored || tc.want == rootFileNameWarned {
				require.NoError(t, err)
				assert.Equal(t, tc.value, opts.ScaffoldRootFileName, "root file name should be stored")

				warned := true

				ctrl.OnceWarn.Do(func() { warned = false })

				assert.Equal(t, tc.want == rootFileNameWarned, warned, "the disabled control should warn")

				return
			}

			var exitErr clihelper.ExitCoder
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, int(clihelper.ExitCodeGeneralError), exitErr.ExitCode())

			if tc.want == rootFileNameRejected {
				require.ErrorIs(t, err, ctrl.Error, "the enabled control should reject a terragrunt.hcl root")
			}
		})
	}
}

// rootFileNameOutcome is how --root-file-name handles a value.
type rootFileNameOutcome int

const (
	rootFileNameStored rootFileNameOutcome = iota
	rootFileNameWarned
	rootFileNameRejected
	rootFileNameEmpty
)
