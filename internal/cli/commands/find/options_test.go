package find_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewOptionsDefaultsValidate pins that the command's defaults are a valid combination.
func TestNewOptionsDefaultsValidate(t *testing.T) {
	t.Parallel()

	opts := find.NewOptions(options.NewTerragruntOptions())

	assert.Equal(t, find.FormatText, opts.Format)
	assert.Equal(t, find.ModeNormal, opts.Mode)
	require.NoError(t, opts.Validate())
}

// TestOptionsValidate pins which format and mode combinations Run is able to render.
func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		format  string
		mode    string
		wantErr bool
	}{
		{name: "text format in normal mode", format: find.FormatText, mode: find.ModeNormal},
		{name: "text format in dag mode", format: find.FormatText, mode: find.ModeDAG},
		{name: "json format in normal mode", format: find.FormatJSON, mode: find.ModeNormal},
		{name: "json format in dag mode", format: find.FormatJSON, mode: find.ModeDAG},
		{
			name:    "unknown format",
			format:  "yaml",
			mode:    find.ModeNormal,
			wantErr: true,
		},
		{
			name:    "unknown mode",
			format:  find.FormatText,
			mode:    "topological",
			wantErr: true,
		},
		{
			name:    "unknown format and mode",
			format:  "yaml",
			mode:    "topological",
			wantErr: true,
		},
		{name: "empty format and mode", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := find.NewOptions(options.NewTerragruntOptions())
			opts.Format = tc.format
			opts.Mode = tc.mode

			err := opts.Validate()
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
