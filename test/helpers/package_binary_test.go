package helpers_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrappedBinary(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	got := helpers.WrappedBinary(ctx)

	assert.Equal(t, got, helpers.WrappedBinary(ctx), "the detected binary is memoized")
	assert.Equal(t, got == helpers.TerraformBinary, helpers.IsTerraform(ctx))

	if value, found := os.LookupEnv("TG_TF_PATH"); found {
		assert.Equal(t, filepath.Base(value), got, "TG_TF_PATH wins")

		return
	}

	if helpers.IsOpenTofuInstalled(ctx) {
		assert.Equal(t, helpers.TofuBinary, got)
	} else {
		assert.Equal(t, helpers.TerraformBinary, got)
	}
}

func TestIsOpenTofuInstalled(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath(helpers.TofuBinary)

	assert.Equal(t, err == nil, helpers.IsOpenTofuInstalled(t.Context()))
}

func TestIsTerraformInstalled(t *testing.T) {
	t.Parallel()

	_, err := exec.LookPath(helpers.TerraformBinary)

	assert.Equal(t, err == nil, helpers.IsTerraformInstalled(t.Context()))
}

func TestIsTerraform110OrHigher(t *testing.T) {
	t.Parallel()

	if !helpers.IsTerraform(t.Context()) {
		assert.False(t, helpers.IsTerraform110OrHigher(t), "OpenTofu is never Terraform 1.10+")

		return
	}

	pkgTRequireOnPath(t, helpers.WrappedBinary(t.Context()))

	major, minor := pkgTBinaryVersion(t, helpers.WrappedBinary(t.Context()), "Terraform")

	assert.Equal(t, pkgTAtLeast(major, minor, 1, 10), helpers.IsTerraform110OrHigher(t))
}

func TestIsOpenTofu112OrHigher(t *testing.T) {
	t.Parallel()

	if helpers.IsTerraform(t.Context()) {
		assert.False(t, helpers.IsOpenTofu112OrHigher(t), "Terraform is never OpenTofu 1.12+")

		return
	}

	pkgTRequireOnPath(t, helpers.TofuBinary)

	major, minor := pkgTBinaryVersion(t, helpers.TofuBinary, "OpenTofu")

	assert.Equal(t, pkgTAtLeast(major, minor, 1, 12), helpers.IsOpenTofu112OrHigher(t))
}

func TestIsNativeS3LockingSupported(t *testing.T) {
	t.Parallel()

	if helpers.IsTerraform(t.Context()) {
		pkgTRequireOnPath(t, helpers.TerraformBinary)

		major, minor := pkgTBinaryVersion(t, helpers.TerraformBinary, "Terraform")

		assert.Equal(t, pkgTAtLeast(major, minor, 1, 11), helpers.IsNativeS3LockingSupported(t))

		return
	}

	pkgTRequireOnPath(t, helpers.TofuBinary)

	major, minor := pkgTBinaryVersion(t, helpers.TofuBinary, "OpenTofu")

	assert.Equal(t, pkgTAtLeast(major, minor, 1, 10), helpers.IsNativeS3LockingSupported(t))
}

// pkgTRequireOnPath skips the test when bin is not on PATH.
func pkgTRequireOnPath(t *testing.T, bin string) {
	t.Helper()

	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("%s is not installed", bin)
	}
}

// pkgTBinaryVersion runs `bin -version` and returns the major and minor
// version it reports for product.
func pkgTBinaryVersion(t *testing.T, bin, product string) (int, int) {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), bin, "-version").Output()
	require.NoError(t, err)

	matches := regexp.MustCompile(product + ` v(\d+)\.(\d+)\.`).FindStringSubmatch(string(out))
	require.Len(t, matches, 3, "unexpected version output: %s", out)

	major, err := strconv.Atoi(matches[1])
	require.NoError(t, err)

	minor, err := strconv.Atoi(matches[2])
	require.NoError(t, err)

	return major, minor
}

// pkgTAtLeast reports whether major.minor is at least wantMajor.wantMinor.
func pkgTAtLeast(major, minor, wantMajor, wantMinor int) bool {
	return major > wantMajor || (major == wantMajor && minor >= wantMinor)
}
