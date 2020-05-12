package cli

import (
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestCompilingGoSource(t *testing.T) {
	t.Parallel()

	sourceDir := tmpDir(t)
	defer os.Remove(sourceDir)

	options := mockCmdOptions(t, sourceDir, []string{})
	outputBinary := sourceDir + "/exec"

	err := compileGoProject("../test/fixture-download-source/hello-world-module", outputBinary, options)

	require.NoError(t, err)

	output, err := shell.RunShellCommandWithOutput(options, "", true, false,
		outputBinary)

	require.NoError(t, err)

	require.Equal(t, "Hello World\n", output.Stdout)

}
