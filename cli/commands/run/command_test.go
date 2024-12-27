package run_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/require"
)

func TestAction(t *testing.T) {
	t.Parallel()

	tt := []struct {
		name        string
		opts        *options.TerragruntOptions
		expectedErr error
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
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fn := run.Action(tc.opts)

			ctx := cli.Context{
				Context: context.Background(),
			}
			err := fn(&ctx)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
