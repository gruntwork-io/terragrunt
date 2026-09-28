package strict_test

import (
	"bytes"
	"testing"

	infostrict "github.com/gruntwork-io/terragrunt/internal/cli/commands/info/strict"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListActionFiltersCompletedControlsAndSubcontrols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        clihelper.Args
		contains    []string
		notContains []string
	}{
		{
			name:        "list without --all",
			args:        clihelper.Args{"list"},
			contains:    []string{"active-parent"},
			notContains: []string{"done-parent"},
		},
		{
			name:     "list with --all",
			args:     clihelper.Args{"list", "--all"},
			contains: []string{"active-parent", "done-parent"},
		},
		{
			name:        "detail without --all",
			args:        clihelper.Args{"list", "active-parent"},
			contains:    []string{"active-sub"},
			notContains: []string{"done-sub"},
		},
		{
			name:     "detail with --all",
			args:     clihelper.Args{"list", "--all", "active-parent"},
			contains: []string{"active-sub", "done-sub"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.StrictControls = strict.Controls{
				&controls.Control{
					Name:        "active-parent",
					Description: "active parent description",
					Subcontrols: strict.Controls{
						&controls.Control{Name: "active-sub", Description: "active sub description"},
						&controls.Control{
							Name:        "done-sub",
							Description: "done sub description",
							Status:      strict.CompletedStatus,
						},
					},
				},
				&controls.Control{
					Name:        "done-parent",
					Description: "done parent description",
					Status:      strict.CompletedStatus,
				},
			}

			out := new(bytes.Buffer)

			app := clihelper.NewApp(map[string]string{})
			app.Writer = out

			cmd := infostrict.NewCommand(logger.CreateLogger(), opts)
			cliCtx := clihelper.NewAppContext(app, tc.args)

			require.NoError(t, cmd.Run(t.Context(), cliCtx, tc.args))

			for _, fragment := range tc.contains {
				assert.Contains(t, out.String(), fragment)
			}

			for _, fragment := range tc.notContains {
				assert.NotContains(t, out.String(), fragment)
			}
		})
	}
}
