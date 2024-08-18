package terraform_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
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
			expectedErr: terraform.WrongTofuCommand("foo"),
		},
		{
			name: "wrong terraform command",
			opts: &options.TerragruntOptions{
				TerraformCommand: "foo",
				TerraformPath:    "terraform",
			},
			expectedErr: terraform.WrongTerraformCommand("foo"),
		},
	}

	for _, tc := range tt {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fn := terraform.Action(tc.opts)

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
