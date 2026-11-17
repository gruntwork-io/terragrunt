package list_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/list"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewOptionsDefaultsValidate pins that the command's defaults are a valid combination.
func TestNewOptionsDefaultsValidate(t *testing.T) {
	t.Parallel()

	opts := list.NewOptions(options.NewTerragruntOptions())

	assert.Equal(t, list.FormatText, opts.Format)
	assert.Equal(t, list.ModeNormal, opts.Mode)
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
		{name: "text format in normal mode", format: list.FormatText, mode: list.ModeNormal},
		{name: "tree format in normal mode", format: list.FormatTree, mode: list.ModeNormal},
		{name: "long format in normal mode", format: list.FormatLong, mode: list.ModeNormal},
		{name: "dot format in normal mode", format: list.FormatDot, mode: list.ModeNormal},
		{name: "text format in dag mode", format: list.FormatText, mode: list.ModeDAG},
		{name: "tree format in dag mode", format: list.FormatTree, mode: list.ModeDAG},
		{name: "long format in dag mode", format: list.FormatLong, mode: list.ModeDAG},
		{name: "dot format in dag mode", format: list.FormatDot, mode: list.ModeDAG},
		{
			name:    "unknown format",
			format:  "yaml",
			mode:    list.ModeNormal,
			wantErr: true,
		},
		{
			name:    "unknown mode",
			format:  list.FormatText,
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

			opts := list.NewOptions(options.NewTerragruntOptions())
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
