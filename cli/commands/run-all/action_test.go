package runall_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingRunAllArguments(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.TerraformCommand = ""

	err = runall.Run(context.Background(), tgOptions)
	require.Error(t, err)

	var missingCommand runall.MissingCommand
	ok := errors.As(err, &missingCommand)
	fmt.Println(err, errors.Unwrap(err))
	assert.True(t, ok)
}

func TestRunAllWithIgnoreFile(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for the test
	tempDir := t.TempDir()

	// Create a .terragrunt.ignore file in the temporary directory
	ignoreFilePath := filepath.Join(tempDir, ".terragrunt.ignore")
	err := os.WriteFile(ignoreFilePath, []byte("module-to-ignore\n"), 0644)
	require.NoError(t, err)

	// Create a TerragruntOptions object with the temporary directory as the working directory
	tgOptions, err := options.NewTerragruntOptionsForTest(tempDir)
	require.NoError(t, err)

	tgOptions.TerraformCommand = "apply"

	// Run the RunAll function
	err = runall.Run(context.Background(), tgOptions)
	require.NoError(t, err)

	// Verify that the IgnorePatterns field in TerragruntOptions is populated correctly
	expectedIgnorePatterns := []string{"module-to-ignore"}
	assert.Equal(t, expectedIgnorePatterns, tgOptions.IgnorePatterns)
}
