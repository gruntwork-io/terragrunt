package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunStacksGenerate verifies that stack generation works correctly when running terragrunt with --all flag.
// It ensures that:
// 1. The stack directory is created
// 2. The stack is properly applied
// 3. The expected number of test.txt files are generated
func TestRunStacksGenerate(t *testing.T) {
	t.Parallel()

	// Set up test environment
	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksBasic, "live")

	// Run terragrunt with --all flag to trigger stack generation
	helpers.RunTerragrunt(t, "terragrunt run apply --all --non-interactive --working-dir "+rootPath)

	// Verify stack directory exists and validate its contents
	path := filepath.Join(rootPath, ".terragrunt-stack")
	validateStackDir(t, path)

	// Collect all test.txt files in the stack directory to verify correct generation
	var txtFiles []string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.Name() == "test.txt" {
			txtFiles = append(txtFiles, filePath)
		}

		return nil
	})

	require.NoError(t, err)
	// Verify that exactly 4 test.txt files were generated
	assert.Len(t, txtFiles, 4)
}

// TestRunNoStacksGenerate verifies that stack generation is skipped in appropriate scenarios:
// 1. When running without --all flag on directory which contains only terragrunt.stack.hcl
// 2. When running with --all but --no-stack-generate flag is set on directory which contains only terragrunt.stack.hcl
// 3. When running without --all flag on standard terragrunt directory
// 4. When running with --all but --no-stack-generate on directory without terragrunt.stack.hcl
func TestRunNoStacksGenerate(t *testing.T) {
	t.Parallel()

	// Define test cases for different scenarios where stack generation should be skipped
	testdata := []struct {
		name       string
		cmd        string
		subfolder  string
		shouldFail bool
	}{
		{
			name:       "NoAll",
			cmd:        "terragrunt run apply --non-interactive",
			subfolder:  "live",
			shouldFail: true,
		},
		{
			name:       "AllNoGenerate",
			cmd:        "terragrunt run apply --all --no-stack-generate --non-interactive",
			subfolder:  "live",
			shouldFail: false,
		},
		{
			name:       "Standard",
			cmd:        "terragrunt run apply --non-interactive",
			subfolder:  "units/chicken",
			shouldFail: false,
		},
		{
			name:       "AllNoStackToGenerate",
			cmd:        "terragrunt run apply --all --no-stack-generate --non-interactive",
			subfolder:  "units",
			shouldFail: false,
		},
	}

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)

	// Run each test case and verify stack generation is skipped
	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up test environment
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
			path := filepath.Join(tmpEnvPath, testFixtureStacksBasic, tt.subfolder)
			cmd := tt.cmd + " --working-dir " + path + " -- -auto-approve"

			// Execute terragrunt command and verify no output
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			if tt.shouldFail {
				require.Error(t, err)
				assert.Empty(t, stdout)
				// We should explicitly avoid asserting on stderr, because information
				// might be logged to stderr, even if the command succeeds.
				//
				// e.g. Usage of the provider cache server.
				//
				// assert.Empty(t, stderr)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, stderr)
			}

			// Verify that stack directory was not created
			genPath := filepath.Join(path, ".terragrunt-stack")
			assert.NoDirExists(t, genPath)
		})
	}
}

func TestRunVersionFilesCacheKey(t *testing.T) {
	t.Parallel()

	// The cache key incorporates the resolved binary path, so the expected
	// hash differs depending on whether tofu or terraform is wrapped.
	testdata := []struct {
		expect       map[string]string
		name         string
		versionFiles []string
	}{
		{
			name: "use default",
			expect: map[string]string{
				helpers.TofuBinary:      "H2PcpB8dh-BE5Dz-LiSD1hykSfk",
				helpers.TerraformBinary: "rdE9RkSvu7WDQly2KzL2HxpcB3Q",
			},
			versionFiles: nil,
		},
		{
			name: "custom files provided",
			expect: map[string]string{
				helpers.TofuBinary:      "trAjGOdUv3IcX1lU50dck_mqlUs",
				helpers.TerraformBinary: "kLXlcfwganOp8AOV4_ErIlU3g6o",
			},
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
			},
		},
	}

	helpers.CleanupTerraformFolder(t, testFixtureVersionFilesCacheKey)

	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wrapped := helpers.WrappedBinary(t.Context())
			expect, ok := tt.expect[wrapped]
			require.Truef(t, ok, "no expected cache key recorded for wrapped binary %q", wrapped)

			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureVersionFilesCacheKey, tt.versionFiles...)
			path := filepath.Join(tmpEnvPath, testFixtureVersionFilesCacheKey)
			flags := make([]string, 0, 4+2*len(tt.versionFiles))
			flags = append(flags,
				"-non-interactive",
				"--log-level debug",
				"--working-dir",
				path,
			)

			for _, file := range tt.versionFiles {
				flags = append(
					flags,
					"--version-manager-file-name",
					file,
				)
			}

			cmd := "terragrunt run apply " + strings.Join(flags, " ") + " -- -auto-approve"

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)
			assert.NotEmpty(t, stdout)
			assert.NotEmpty(t, stderr)
			assert.Contains(t, stderr, "using cache key for version files: "+expect)
		})
	}
}

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
	writeMockTFBinary(t, filepath.Join(binDir, helpers.TofuBinary), "OpenTofu v1.99.9", helpers.TofuBinary)
	writeMockTFBinary(t, filepath.Join(binDir, helpers.TerraformBinary), "Terraform v1.99.9", helpers.TerraformBinary)

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
		{name: "config terraform", configBinary: helpers.TerraformBinary, expected: helpers.TerraformBinary},
		{name: "config tofu", configBinary: helpers.TofuBinary, expected: helpers.TofuBinary},
		{name: "tf-path overrides config tofu", configBinary: helpers.TofuBinary, tfPathFlag: helpers.TerraformBinary, expected: helpers.TerraformBinary},
		{name: "tf-path overrides config terraform", configBinary: helpers.TerraformBinary, tfPathFlag: helpers.TofuBinary, expected: helpers.TofuBinary},
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
