package test_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	registryFixturePath                               = "fixtures/tfr"
	registryFixtureRootModulePath                     = "root"
	registryFixtureRootShorthandModulePath            = "root-shorthand"
	registryFixtureSubdirModulePath                   = "subdir"
	registryFixtureSubdirWithReferenceModulePath      = "subdir-with-reference"
	registryFixtureVersionConstraintModulePath        = "version-constraint"
	registryFixtureVersionConstraintNoMatchModulePath = "version-constraint-no-match"
	registryFixtureVersionConstraintInQueryModulePath = "version-constraint-in-query"

	// registryTestModuleSource is the source used by the version-constraint
	// fixtures. The module has exactly two published versions, 0.0.1 and
	// 0.0.2, so constraint resolution against it is deterministic.
	registryTestModuleSource = "tfr://registry.opentofu.org/yorinasub17/terragrunt-registry-test/null"
)

func TestTerraformRegistryFetchingRootModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureRootModulePath, "root_null_resource")
}

func TestRegistryFetchingRootShorthandModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureRootShorthandModulePath, "root_null_resource")
}

func TestTerraformRegistryFetchingSubdirModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureSubdirModulePath, "one_null_resource")
}

func TestTerraformRegistryFetchingSubdirWithReferenceModule(t *testing.T) {
	t.Parallel()
	testTerraformRegistryFetching(t, registryFixtureSubdirWithReferenceModulePath, "two")
}

// TestTerraformRegistryVersionConstraintPinsResolvedVersion runs a unit whose
// terraform block carries a bare tfr:// source plus a version constraint and
// verifies the constraint end-to-end: the unit applies successfully, and the
// download lands in the cache slot keyed by the exact ?version=0.0.2 pin the
// resolver must have produced from "~> 0.0.1".
func TestTerraformRegistryVersionConstraintPinsResolvedVersion(t *testing.T) {
	t.Parallel()

	modPath := filepath.Join(registryFixturePath, registryFixtureVersionConstraintModulePath)
	helpers.CleanupTerraformFolder(t, modPath)
	tmpEnvPath := helpers.CopyEnvironment(t, modPath)
	rootPath := filepath.Join(tmpEnvPath, modPath)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --non-interactive --experiment version-attribute --working-dir "+rootPath+" -- apply -auto-approve",
	)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt run --non-interactive --experiment version-attribute --working-dir "+rootPath+" -- output -no-color -json",
		&stdout,
		&stderr,
	)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Contains(t, outputs, "root_null_resource")

	// Reconstruct the cache slot for the pinned URL the same way the download
	// path does. Its presence, with a version file recording the pinned
	// query, proves the constraint resolved to 0.0.2 rather than some other
	// version (whose slot would live under a different query hash).
	l := logger.CreateLogger()

	pinned, err := tf.NewSource(
		l,
		registryTestModuleSource+"?version=0.0.2",
		filepath.Join(rootPath, ".terragrunt-cache"),
		rootPath,
		false,
	)
	require.NoError(t, err)

	assert.True(t, util.FileExists(filepath.Join(pinned.WorkingDir, "main.tf")))

	wantVersion, err := pinned.EncodeSourceVersion(l)
	require.NoError(t, err)

	// Guard against a vacuous comparison: the version encoding must
	// discriminate between the two published versions, or the assertion
	// below could not tell 0.0.2 apart from 0.0.1.
	otherPin, err := tf.NewSource(
		l,
		registryTestModuleSource+"?version=0.0.1",
		filepath.Join(rootPath, ".terragrunt-cache"),
		rootPath,
		false,
	)
	require.NoError(t, err)

	otherVersion, err := otherPin.EncodeSourceVersion(l)
	require.NoError(t, err)
	require.NotEqual(t, wantVersion, otherVersion)

	gotVersion, err := util.ReadFileAsString(pinned.VersionFile)
	require.NoError(t, err)
	assert.Equal(t, wantVersion, gotVersion)
}

// TestTerraformRegistryVersionConstraintRequiresExperiment pins the typed
// error returned when the terraform block sets the version attribute but the
// version-attribute experiment is not enabled.
func TestTerraformRegistryVersionConstraintRequiresExperiment(t *testing.T) {
	t.Parallel()

	modPath := filepath.Join(registryFixturePath, registryFixtureVersionConstraintModulePath)
	helpers.CleanupTerraformFolder(t, modPath)
	tmpEnvPath := helpers.CopyEnvironment(t, modPath)
	rootPath := filepath.Join(tmpEnvPath, modPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var expectedErr config.VersionAttributeRequiresExperimentError

	assert.ErrorAs(t, err, &expectedErr)
}

// TestTerraformRegistryVersionConstraintNoMatchingVersion pins the typed
// error returned at download time when the registry publishes versions but
// none satisfy the configured constraint.
func TestTerraformRegistryVersionConstraintNoMatchingVersion(t *testing.T) {
	t.Parallel()

	modPath := filepath.Join(registryFixturePath, registryFixtureVersionConstraintNoMatchModulePath)
	helpers.CleanupTerraformFolder(t, modPath)
	tmpEnvPath := helpers.CopyEnvironment(t, modPath)
	rootPath := filepath.Join(tmpEnvPath, modPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --experiment version-attribute --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var expectedErr getter.NoMatchingVersionErr

	assert.ErrorAs(t, err, &expectedErr)
}

// TestTerraformRegistryVersionConstraintInQueryRejected pins the typed error
// returned when a tfr:// source carries a version constraint in its ?version=
// query, which accepts an exact version only. The guard is active without the
// version-attribute experiment, since such a source was never valid.
func TestTerraformRegistryVersionConstraintInQueryRejected(t *testing.T) {
	t.Parallel()

	modPath := filepath.Join(registryFixturePath, registryFixtureVersionConstraintInQueryModulePath)
	helpers.CleanupTerraformFolder(t, modPath)
	tmpEnvPath := helpers.CopyEnvironment(t, modPath)
	rootPath := filepath.Join(tmpEnvPath, modPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt plan --non-interactive --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	var expectedErr run.SourceVersionConstraintErr

	assert.ErrorAs(t, err, &expectedErr)
}

func testTerraformRegistryFetching(t *testing.T, modPath, expectedOutputKey string) {
	t.Helper()

	modFullPath := filepath.Join(registryFixturePath, modPath)
	helpers.CleanupTerraformFolder(t, modFullPath)
	helpers.RunTerragrunt(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+modFullPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := helpers.RunTerragruntCommand(t, "terragrunt output -no-color -json --non-interactive --working-dir "+modFullPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]helpers.TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	_, hasOutput := outputs[expectedOutputKey]
	assert.True(t, hasOutput)
}
