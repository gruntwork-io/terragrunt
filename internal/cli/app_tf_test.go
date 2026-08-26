//go:build tf

package cli_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/vexec"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFTerraformHelp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expected string
		args     []string
	}{
		{
			args:     []string{"terragrunt", tf.CommandNamePlan, "--help"},
			expected: "(?s)Usage: terragrunt \\[global options\\] plan.*-detailed-exitcode",
		},
		{
			args:     []string{"terragrunt", tf.CommandNameApply, "-help"},
			expected: "(?s)Usage: terragrunt \\[global options\\] apply.*-destroy",
		},
		{
			args:     []string{"terragrunt", tf.CommandNameApply, "-h"},
			expected: "(?s)Usage: terragrunt \\[global options\\] apply.*-destroy",
		},
	}

	for _, tc := range testCases {
		output := &bytes.Buffer{}
		opts := options.NewTerragruntOptions(vexec.NewOSExec())

		testV := venv.OSVenv()

		testV.Writers = &writer.Writers{Writer: output, ErrWriter: os.Stderr}

		l := logger.CreateLogger()

		app := cli.NewApp(l, opts, testV)
		err := app.Run(l, testV, tc.args)
		require.NoError(t, err)

		assert.Regexp(t, tc.expected, output.String())
	}
}
