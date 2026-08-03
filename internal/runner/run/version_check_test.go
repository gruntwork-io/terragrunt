//nolint:unparam
package run_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Terraform Version Checking
func TestCheckTerraformVersionMeetsConstraintEqual(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.9.3", ">= v0.9.3", true)
}

func TestCheckTerraformVersionMeetsConstraintGreaterPatch(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.9.4", ">= v0.9.3", true)
}

func TestCheckTerraformVersionMeetsConstraintGreaterMajor(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v1.0.0", ">= v0.9.3", true)
}

func TestCheckTerraformVersionMeetsConstraintLessPatch(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.9.2", ">= v0.9.3", false)
}

func TestCheckTerraformVersionMeetsConstraintLessMajor(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.8.8", ">= v0.9.3", false)
}

func TestParseOpenTofuVersionNormal(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "OpenTofu v1.6.0", "v1.6.0", nil)
}

func TestParseOpenTofuVersionDev(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "OpenTofu v1.6.0-dev", "v1.6.0", nil)
}

func TestParseTerraformVersionNormal(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.9.3", "v0.9.3", nil)
}

func TestParseTerraformVersionWithoutV(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform 0.9.3", "0.9.3", nil)
}

func TestParseTerraformVersionWithDebug(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(
		t,
		"Terraform v0.9.4 cad024a5fe131a546936674ef85445215bbc4226",
		"v0.9.4",
		nil,
	)
}

func TestParseTerraformVersionWithChanges(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(
		t,
		"Terraform v0.9.4-dev (cad024a5fe131a546936674ef85445215bbc4226+CHANGES)",
		"v0.9.4",
		nil,
	)
}

func TestParseTerraformVersionWithDev(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.9.4-dev", "v0.9.4", nil)
}

func TestParseTerraformVersionWithBeta(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.13.0-beta1", "v0.13.0", nil)
}

func TestParseTerraformVersionWithUnexpectedName(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.15.0-rc1", "v0.15.0", nil)
}

func TestParseTerraformVersionInvalidSyntax(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(
		t,
		"invalid-syntax",
		"",
		run.InvalidTerraformVersionSyntax("invalid-syntax"),
	)
}

func testCheckTerraformVersionMeetsConstraint(
	t *testing.T,
	currentVersion string,
	versionConstraint string,
	versionMeetsConstraint bool,
) {
	t.Helper()

	current, err := version.NewVersion(currentVersion)
	if err != nil {
		t.Fatalf("Invalid current version specified in test: %v", err)
	}

	err = run.CheckTerraformVersionMeetsConstraint(current, versionConstraint)
	if versionMeetsConstraint && err != nil {
		assert.NoError(t, err,
			"Expected Terraform version %s to meet constraint %s, but got error: %v",
			currentVersion, versionConstraint, err)
	} else if !versionMeetsConstraint && err == nil {
		assert.Error(t, err,
			"Expected Terraform version %s to NOT meet constraint %s, but got back a nil error",
			currentVersion, versionConstraint)
	}
}

func testParseTerraformVersion(
	t *testing.T,
	versionString string,
	expectedVersion string,
	expectedErr error,
) {
	t.Helper()

	actualVersion, actualErr := run.ParseTerraformVersion(versionString)

	if expectedErr == nil {
		expected, err := version.NewVersion(expectedVersion)
		if err != nil {
			t.Fatalf("Invalid expected version specified in test: %v", err)
		}

		require.NoError(t, actualErr)
		assert.Equal(t, expected, actualVersion)
	} else {
		assert.ErrorIs(t, actualErr, expectedErr)
	}
}

// TODO: Refactor these into a test table.

// Terragrunt Version Checking
func TestCheckTerragruntVersionMeetsConstraintEqual(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.18", ">= v0.23.18", true)
}

func TestCheckTerragruntVersionMeetsConstraintGreaterPatch(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.18", ">= v0.23.9", true)
}

func TestCheckTerragruntVersionMeetsConstraintGreaterMajor(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v1.0.0", ">= v0.23.18", true)
}

func TestCheckTerragruntVersionMeetsConstraintLessPatch(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.17", ">= v0.23.18", false)
}

func TestCheckTerragruntVersionMeetsConstraintLessMajor(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.22.15", ">= v0.23.18", false)
}

func TestCheckTerragruntVersionMeetsConstraintPrerelease(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.18-alpha202409013", ">= v0.23.18", true)
}

// TestPopulateTFVersionRespectsTFPath verifies that the run-scoped version
// cache is keyed by the resolved binary path. Two calls with the same working
// directory but different binaries must resolve independently: an early call
// that resolves against `tofu` must not poison the entry a later call reads
// after `terraform_binary` has taken effect. The test feeds distinct --version
// stdout per binary through a vexec.MemExec handler and asserts the second call
// resolves Terraform, not the cached OpenTofu entry.
func TestPopulateTFVersionRespectsTFPath(t *testing.T) {
	t.Parallel()

	const (
		tofuVersionOutput      = "OpenTofu v1.9.0\non darwin_arm64\n"
		terraformVersionOutput = "Terraform v1.15.3\non darwin_arm64\n"
	)

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		switch inv.Name {
		case "tofu":
			return vexec.Result{Stdout: []byte(tofuVersionOutput)}
		case "terraform":
			return vexec.Result{Stdout: []byte(terraformVersionOutput)}
		default:
			return vexec.Result{ExitCode: 1, Stderr: []byte("unexpected binary: " + inv.Name)}
		}
	}
	e := vexec.NewMemExec(handler)

	ctx := run.WithRunVersionCache(t.Context())
	l := logger.CreateLogger()

	tfOpts := func(binary string) *tf.TFOptions {
		return &tf.TFOptions{
			TerraformCliArgs: iacargs.New(),
			ShellOptions: shell.NewShellOptions().
				WithTFPath(binary).
				WithWorkingDir(t.TempDir()),
		}
	}

	// First call mirrors what setupAutoProviderCacheDir does before
	// terraform_binary is read from HCL: TFPath is still the auto-detected
	// "tofu", and the OpenTofu version gets resolved and cached.
	_, tofuVer, tofuImpl, err := run.PopulateTFVersion(
		ctx,
		l,
		venvtest.New().WithExec(e),
		run.PopulateTFVersionInput{
			TFOpts: tfOpts("tofu"),
		},
	)
	require.NoError(t, err)
	assert.Equal(t, tfimpl.OpenTofu, tofuImpl)
	assert.Equal(t, "1.9.0", tofuVer.String())

	// Second call mirrors checkVersionConstraints after `terraform_binary =
	// "terraform"` has been applied. Before the fix, this hit the poisoned
	// cache entry and returned OpenTofu v1.9.0, which then failed any
	// terraform-version-constraint check pinned to a real Terraform release.
	_, terraformVer, terraformImpl, err := run.PopulateTFVersion(
		ctx,
		l,
		venvtest.New().WithExec(e),
		run.PopulateTFVersionInput{
			TFOpts: tfOpts("terraform"),
		},
	)
	require.NoError(t, err)
	assert.Equal(t, tfimpl.Terraform, terraformImpl,
		"expected Terraform after switching TFPath to 'terraform'; got %s"+
			" (version cache likely ignored TFPath)",
		terraformImpl)
	assert.Equal(t, "1.15.3", terraformVer.String(),
		"expected Terraform v1.15.3 after switching TFPath; got %s"+
			" (version cache likely ignored TFPath)",
		terraformVer)
}

func testCheckTerragruntVersionMeetsConstraint(
	t *testing.T,
	currentVersion string,
	versionConstraint string,
	versionMeetsConstraint bool,
) {
	t.Helper()

	current, err := version.NewVersion(currentVersion)
	if err != nil {
		t.Fatalf("Invalid current version specified in test: %v", err)
	}

	err = run.CheckTerragruntVersionMeetsConstraint(current, versionConstraint)
	if versionMeetsConstraint && err != nil {
		t.Fatalf("Expected Terragrunt version %s to meet constraint %s, but got error: %v",
			currentVersion, versionConstraint, err)
	} else if !versionMeetsConstraint && err == nil {
		t.Fatalf(
			"Expected Terragrunt version %s to NOT meet constraint %s, but got back a nil error",
			currentVersion,
			versionConstraint,
		)
	}
}
