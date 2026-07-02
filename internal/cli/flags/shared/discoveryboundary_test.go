package shared_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoveryBoundaryFlagValidation verifies that --discovery-boundary rejects values that
// are not existing directories and accepts an existing directory.
func TestDiscoveryBoundaryFlagValidation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	file := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "existing directory is accepted", value: dir, wantErr: false},
		{name: "non-existent path is rejected", value: filepath.Join(dir, "missing"), wantErr: true},
		{name: "file (not a directory) is rejected", value: file, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			cmdFlags := clihelper.Flags{shared.NewDiscoveryBoundaryFlag(opts)}

			require.NoError(t, cmdFlags.Parse(clihelper.Args{"--discovery-boundary", tt.value}))

			err := cmdFlags.RunActions(context.Background(), &clihelper.Context{})
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.value, opts.DiscoveryBoundary)
		})
	}
}
