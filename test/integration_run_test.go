package test_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAllHonorsTerraformBinaryWithBothOnPath verifies which binary `run --all`
// executes for each unit when both tofu and terraform are on PATH:
//   - terraform_binary selects the binary, overriding what auto-detection picks
//     (tofu when both are present).
//   - an explicit --tf-path still wins over terraform_binary.
//
// Both tofu and terraform are mocked on PATH so the test is hermetic: it does
// not depend on which real binary is installed, and each expected binary is
// checked against the other so the wrong binary is caught regardless of which
// one auto-detection would otherwise pick.
//
//nolint:paralleltest // Mutates PATH via t.Setenv, which is incompatible with t.Parallel.
func TestRunAllHonorsTerraformBinaryWithBothOnPath(t *testing.T) {
	binDir := t.TempDir()
	writeMockTFBinary(
		t,
		filepath.Join(binDir, helpers.TofuBinary),
		"OpenTofu v1.99.9",
		helpers.TofuBinary,
	)
	writeMockTFBinary(
		t,
		filepath.Join(binDir, helpers.TerraformBinary),
		"Terraform v1.99.9",
		helpers.TerraformBinary,
	)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// A TG_TF_PATH inherited from the environment would override the config
	// value under test, so clear it.
	t.Setenv("TG_TF_PATH", "")
	os.Unsetenv("TG_TF_PATH")

	testCases := []struct {
		name         string
		configBinary string
		tfPathFlag   string
		expected     string
	}{
		{
			name:         "config terraform",
			configBinary: helpers.TerraformBinary,
			expected:     helpers.TerraformBinary,
		},
		{name: "config tofu", configBinary: helpers.TofuBinary, expected: helpers.TofuBinary},
		{
			name:         "tf-path overrides config tofu",
			configBinary: helpers.TofuBinary,
			tfPathFlag:   helpers.TerraformBinary,
			expected:     helpers.TerraformBinary,
		},
		{
			name:         "tf-path overrides config terraform",
			configBinary: helpers.TerraformBinary,
			tfPathFlag:   helpers.TofuBinary,
			expected:     helpers.TofuBinary,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// run --all resolves the binary per unit in the runner pool, the
			// path where terraform_binary was ignored.
			workingDir := t.TempDir()
			for _, unit := range []string{"first", "second"} {
				unitDir := filepath.Join(workingDir, unit)
				require.NoError(t, os.MkdirAll(unitDir, 0755))
				require.NoError(t, os.WriteFile(
					filepath.Join(unitDir, "terragrunt.hcl"),
					[]byte("terraform_binary = \""+tc.configBinary+"\"\n"),
					0644,
				))
				require.NoError(t, os.WriteFile(filepath.Join(unitDir, "main.tf"), []byte{}, 0644))
			}

			cmd := "terragrunt run --all init --non-interactive --working-dir " + workingDir
			if tc.tfPathFlag != "" {
				cmd += " --tf-path " + tc.tfPathFlag
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			output := stdout + stderr

			other := helpers.TofuBinary
			if tc.expected == helpers.TofuBinary {
				other = helpers.TerraformBinary
			}

			assert.Contains(t, output, mockTFMarker(tc.expected),
				"terragrunt should have invoked the %q binary", tc.expected)
			assert.NotContains(t, output, mockTFMarker(other),
				"terragrunt should not have invoked the %q binary", other)
		})
	}
}

// mockTFMarker is the line a mock binary prints when it is invoked for a real
// command (i.e. anything other than a version probe).
func mockTFMarker(impl string) string {
	return "MOCK-TF-IMPL=" + impl
}

// writeMockTFBinary writes an executable bash script that stands in for tofu or
// terraform. It answers `-version` with a parseable version line so Terragrunt's
// version detection succeeds, and prints a distinctive marker for any other
// command so tests can tell which binary was actually invoked.
func writeMockTFBinary(t *testing.T, path, versionLine, impl string) {
	t.Helper()

	script := "#!/usr/bin/env bash\n" +
		"case \"$1\" in\n" +
		"  -version|--version|version)\n" +
		"    echo \"" + versionLine + "\"\n" +
		"    ;;\n" +
		"  *)\n" +
		"    echo \"" + mockTFMarker(impl) + "\"\n" +
		"    ;;\n" +
		"esac\n" +
		"exit 0\n"

	require.NoError(t, os.WriteFile(path, []byte(script), 0755))
}
