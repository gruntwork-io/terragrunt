package providercache_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/stretchr/testify/assert"
)

func TestLockfileReadonlyRequested(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env  map[string]string
		name string
		args []string
		want bool
	}{
		{
			name: "no lockfile flag",
			args: []string{"init"},
			want: false,
		},
		{
			name: "flag with equals form",
			args: []string{"init", "-lockfile=readonly"},
			want: true,
		},
		{
			name: "flag with space form",
			args: []string{"init", "-lockfile", "readonly"},
			want: true,
		},
		{
			name: "double dash equals form",
			args: []string{"init", "--lockfile=readonly"},
			want: true,
		},
		{
			name: "non-readonly mode is ignored",
			args: []string{"init", "-lockfile=something"},
			want: false,
		},
		{
			name: "set via TF_CLI_ARGS_init",
			args: []string{"init"},
			env:  map[string]string{tf.EnvNameTFCLIArgsInit: "-lockfile=readonly"},
			want: true,
		},
		{
			name: "set via TF_CLI_ARGS alongside other args",
			args: []string{"init"},
			env:  map[string]string{tf.EnvNameTFCLIArgs: "-input=false -lockfile=readonly"},
			want: true,
		},
		{
			name: "unrelated env arg",
			args: []string{"init"},
			env:  map[string]string{tf.EnvNameTFCLIArgsInit: "-input=false"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := providercache.LockfileReadonlyRequested(clihelper.Args(tt.args), tt.env)
			assert.Equal(t, tt.want, got)
		})
	}
}
