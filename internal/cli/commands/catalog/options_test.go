package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func TestOptionsDefaultToTheTUI(t *testing.T) {
	t.Parallel()

	opts := catalog.NewOptions(options.NewTerragruntOptions(vexec.NewOSExec()))

	assert.Equal(t, catalog.FormatTUI, opts.Format)
	require.NoError(t, opts.Validate())
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
