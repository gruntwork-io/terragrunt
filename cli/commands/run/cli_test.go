package run_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

func TestAction(t *testing.T) {
	t.Parallel()

	tt := []struct {
		expectedErr error
		opts        *options.TerragruntOptions
		name        string
	}{
		{
			name: "wrong tofu command",
			opts: &options.TerragruntOptions{
				TerraformCommand: "foo",
				TerraformPath:    "tofu",
			},
			expectedErr: run.WrongTofuCommand("foo"),
		},
		{
			name: "wrong terraform command",
			opts: &options.TerragruntOptions{
				TerraformCommand: "foo",
				TerraformPath:    "terraform",
			},
			expectedErr: run.WrongTerraformCommand("foo"),
		},
	}

	for _, tc := range tt {

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fn := run.Action(logger.CreateLogger(), tc.opts)

			ctx := cli.NewAppContext(t.Context(), cli.NewApp(), nil).
				NewCommandContext(run.NewCommand(logger.CreateLogger(), tc.opts), []string{"bar"})

			err := fn(ctx)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
