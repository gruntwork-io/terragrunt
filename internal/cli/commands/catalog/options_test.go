package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func TestDefaultFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		want   string
		stdin  bool
		stdout bool
	}{
		{name: "terminal", want: catalog.FormatTUI, stdin: true, stdout: true},
		{name: "stdout piped", want: catalog.FormatJSONL, stdin: true},
		{name: "stdin redirected", want: catalog.FormatJSONL, stdout: true},
		{name: "no terminal", want: catalog.FormatJSONL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			terminal := &venv.Terminal{
				StdinIsTTY:  func() bool { return tc.stdin },
				StdoutIsTTY: func() bool { return tc.stdout },
			}

			assert.Equal(t, tc.want, catalog.DefaultFormat(terminal))
		})
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "tui", format: catalog.FormatTUI},
		{name: "jsonl", format: catalog.FormatJSONL},
		{name: "md", format: catalog.FormatMD},
		{name: "unknown format", format: "yaml", wantErr: true},
		{name: "empty format", format: "", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := catalog.NewOptions(options.NewTerragruntOptions(vexec.NewOSExec()))
			opts.Format = tc.format

			err := opts.Validate()

			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
