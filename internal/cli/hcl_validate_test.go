package cli_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/require"
)

// TestHCLValidateReportsUnparsableDependencyConfigPath pins that hcl validate
// returns an error for a dependency whose config_path fails to parse, while
// the dependency's mock outputs still resolve with config_path unknown.
func TestHCLValidateReportsUnparsableDependencyConfigPath(t *testing.T) {
	t.Parallel()

	v := venvtest.New().WithFS(venvtest.NewFS(t, unitRoot, map[string]string{
		"terragrunt.hcl": `
dependency "producer" {
  config_path = ` + "\xb7" + ` "../producer"

  mock_outputs = {
    value = "mock"
  }
}

inputs = {
  value = dependency.producer.outputs.value
}
`,
	}))

	_, err := runCLI(t, v, "hcl", "validate", "--working-dir", unitRoot)
	require.Error(t, err)
}
