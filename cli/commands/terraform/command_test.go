package terraform

import (
	"context"
	"testing"

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
			expectedErr: WrongTofuCommand("foo"),
		},
		{
			name: "wrong terraform command",
			opts: &options.TerragruntOptions{
				TerraformCommand: "foo",
				TerraformPath:    "terraform",
			},
			expectedErr: WrongTerraformCommand("foo"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			fn := action(tc.opts)

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
