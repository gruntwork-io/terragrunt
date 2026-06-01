package shared_test

import (
	"flag"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/require"
)

func TestParallelismFlag(t *testing.T) {
	t.Parallel()

	for _, it := range []struct {
		name    string
		args    clihelper.Args
		wantErr bool
		wantVal int
	}{
		{"empty", clihelper.Args{}, false, 0},
		{"valid", clihelper.Args{"--parallelism", "1"}, false, 1},
		{"zero", clihelper.Args{"--parallelism", "0"}, true, 0},
		{"negative", clihelper.Args{"--parallelism", "-1"}, true, 0},
		{"invalid", clihelper.Args{"--parallelism", "invalid"}, true, 0},
	} {
		t.Run(it.name, func(t *testing.T) {
			t.Parallel()

			var (
				opts options.TerragruntOptions
				f    = shared.NewParallelismFlag(&opts)
				fs   = new(flag.FlagSet)
			)

			// necessary in order to initialise GenericFlag internals
			fs.SetOutput(io.Discard) // avoid panics on invalid values
			f.Apply(fs)

			err := fs.Parse(it.args)

			if it.wantErr {
				require.Error(t, err)
			}

			if !it.wantErr {
				require.NoError(t, err)
				require.Equal(t, it.wantVal, opts.Parallelism)
			}
		})
	}
}
