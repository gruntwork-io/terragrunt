package test_test

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureRegressions                       = "fixtures/regressions"
	testFixtureDependencyGenerate                = "fixtures/regressions/dependency-generate"
	testFixtureDependencyEmptyConfigPath         = "fixtures/regressions/dependency-empty-config-path"
	testFixtureDisabledDependencyEmptyConfigPath = "fixtures/regressions/disabled-dependency-empty-config-path"
	testFixtureParsingDeprecated                 = "fixtures/parsing/exposed-include-with-deprecated-inputs"
	testFixtureSensitiveValues                   = "fixtures/regressions/sensitive-values"
	testFixtureStackDetection                    = "fixtures/regressions/multiple-stacks"
	testFixtureScopeEscape                       = "fixtures/regressions/5195-scope-escape"
	testFixtureNotExistingDependency             = "fixtures/regressions/not-existing-dependency"
	testFixtureDependencyIncludeError            = "fixtures/regressions/dependency-include-error"
	testFixtureReadConfigDependencyStack         = "fixtures/regressions/read-config-dependency-stack"
	testFixtureChainedDepsExposedInclude         = "fixtures/regressions/chained-deps-exposed-include"
	testFixtureExposedIncludePartialParseError   = "fixtures/regressions/exposed-include-partial-parse-error"
	testFixtureDAGQueueDisplay                   = "fixtures/regressions/dag-queue-display"
	testFixtureAutoInitMarkerCachedModules       = "fixtures/regressions/auto-init-marker-with-cached-modules"
	testFixtureDependencyExtraArgsEnv            = "fixtures/regressions/dependency-extra-args-env"
	testFixtureDependencyHookOutput              = "fixtures/regressions/dependency-hook-output"
	testFixtureDependencyExtraArgsOutput         = "fixtures/regressions/dependency-extra-args-output"
	testFixtureDependencyRemoteStateOutput       = "fixtures/regressions/dependency-remote-state-output"
	testFixtureDependencyGenuineError            = "fixtures/regressions/dependency-genuine-error"
	testFixtureDependencyOutputLocalOptimization = "fixtures/regressions/dependency-output-local-optimization"
	testFixtureTerraformSourceRefsDependency     = "fixtures/regressions/terraform-source-references-dependency"
)

func TestIncludeError(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := filepath.Join(tmpEnvPath, testFixtureRegressions, "include-error", "project", "app")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+rootPath,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "include blocks without label")
}

// TestExposedIncludeWithDeprecatedInputsSyntax tests that deprecated dependency.*.inputs.* syntax
// is properly detected even when used in an included config with expose = true.
// This is a regression test for a bug introduced in v0.91.1 where the partial parse path
// did not call DetectDeprecatedConfigurations(), causing cryptic "Could not find Terragrunt
// configuration settings" errors instead of clear deprecation messages.
//
// The bug occurs when:
// 1. An included config (e.g., compcommon.hcl) uses deprecated dependency.*.inputs.* syntax
// 2. The child config includes it with expose = true
// 3. The included config is parsed via PartialParseConfig() which skips deprecation detection
// 4. When evaluating the exposed include, Terragrunt encounters unsupported syntax and fails
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4983
func TestExposedIncludeWithDeprecatedInputsSyntax(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureParsingDeprecated)
	tmpEnvPath := helpers.NewGitServer(t).RenderFixture(testFixtureParsingDeprecated)
	childPath := filepath.Join(tmpEnvPath, testFixtureParsingDeprecated, "child")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --non-interactive --working-dir "+childPath,
	)
	require.Error(t, err)

	// After the fix, we should get a clear error about deprecated syntax
	// instead of the cryptic "Could not find Terragrunt configuration settings" error
	// The error message appears in the error object, not necessarily stderr
	errorMessage := stderr
	if err != nil {
		errorMessage = errorMessage + " " + err.Error()
	}

	assert.Contains(t, errorMessage, "Reading inputs from dependencies is no longer supported")

	// Should NOT get the cryptic error that users were seeing
	assert.NotContains(t, errorMessage, "Could not find Terragrunt configuration settings")
}

// TestParsingWithGenerateAndExpose tests that config parsing works correctly with:
// - Exposed include blocks with generate blocks
// - Dependencies between units
// - Complex inputs with map comparisons
//
// This is a regression test for parsing errors that occurred in v0.90.1+ where
// configs with exposed includes containing generate blocks would fail during
// discovery with "Could not find Terragrunt configuration settings" errors.
//
// Uses `list --dag --format=dot` instead of `run --all plan` since this is a parsing test
// and doesn't need to run terraform operations.
//
// See: https://github.com/gruntwork-io/terragrunt/issues/4983
func TestParsingWithGenerateAndExpose(t *testing.T) {
	t.Parallel()

	testFixture := "fixtures/regressions/parsing-run-all-with-generate"
	helpers.CleanupTerraformFolder(t, testFixture)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixture)
	rootPath := filepath.Join(tmpEnvPath, testFixture, "services-info")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt list --dag --format=dot --non-interactive --working-dir "+rootPath,
	)

	// The command should succeed
	require.NoError(t, err, "list --dag --format=dot should succeed")

	// Should not see parsing errors
	assert.NotContains(t, stderr, "Could not find Terragrunt configuration settings",
		"Should not see parsing errors")
	assert.NotContains(t, stderr, "Unrecoverable parse error",
		"Should not see unrecoverable parse errors")

	// Should not see fmt formatting artifacts from %w (e.g., %!w(...))
	assert.NotContains(t, stderr, "%!w(",
		"Should not see formatting artifacts in error output")

	// Verify both units are discovered in the dependency graph
	// list --dag --format=dot outputs DOT format showing dependencies
	assert.Contains(t, stdout, "test1", "Should discover the service dependency")
}

// TestParsingWithGenerateAndExpose_WithExternalDependencies tests that config parsing
// works correctly when external dependencies exist. This is a variant of TestParsingWithGenerateAndExpose
// that verifies the same parsing behavior works with the full fixture including external dependencies.
//
// Uses `list --dag --format=dot` instead of `run --all plan` since this is a parsing test
// and doesn't need to run terraform operations.
func TestParsingWithGenerateAndExpose_WithExternalDependencies(t *testing.T) {
	t.Parallel()

	testFixture := "fixtures/regressions/parsing-run-all-with-generate"
	helpers.CleanupTerraformFolder(t, testFixture)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixture)
	rootPath := filepath.Join(tmpEnvPath, testFixture, "services-info")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt list --dag --format=dot --non-interactive --working-dir "+rootPath,
	)

	// The command should succeed
	require.NoError(t, err)

	// Should not see parsing errors or formatting artifacts
	assert.NotContains(t, stderr, "Could not find Terragrunt configuration settings")
	assert.NotContains(t, stderr, "Unrecoverable parse error")
	assert.NotContains(t, stderr, "%!w(")

	// Verify units are discovered in the dependency graph
	assert.Contains(t, stdout, "test1", "Should discover the service dependency")
}

func TestMultipleStacksDetection(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStackDetection)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStackDetection)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStackDetection, "live")

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)

	require.NoError(t, err)

	assert.Contains(t, stderr, "terragrunt.stack.hcl")
	assert.Contains(t, stderr, "unit1")
	assert.Contains(t, stderr, "unit2")

	assert.NotContains(t, stderr, "appv2.terragrunt.stack.hcl")
	assert.NotContains(t, stderr, "unit4")
	assert.NotContains(t, stderr, "unit3")
}

// flushTrackingWriter wraps a writer and tracks writes and output size changes (which indicate flushes)
type flushTrackingWriter struct {
	w      io.Writer
	signal chan<- struct{}
	mu     sync.Mutex
	writes int
	once   bool
}

func (ftw *flushTrackingWriter) Write(p []byte) (int, error) {
	ftw.mu.Lock()
	ftw.writes++

	shouldSignal := !ftw.once && ftw.signal != nil
	if shouldSignal {
		ftw.once = true
	}

	ftw.mu.Unlock()

	if shouldSignal {
		select {
		case ftw.signal <- struct{}{}:
		default:
		}
	}

	return ftw.w.Write(p)
}

func (ftw *flushTrackingWriter) getWriteCount() int {
	ftw.mu.Lock()
	defer ftw.mu.Unlock()

	return ftw.writes
}

// TestExposedIncludePartialParseSucceeds verifies that partial parsing (used during module discovery)
// succeeds when an included config has an unresolved dependency, because the include resolution error
// is gracefully swallowed during partial parse.
func TestExposedIncludePartialParseSucceeds(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExposedIncludePartialParseError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureExposedIncludePartialParseError)

	helpers.CleanupTerraformFolder(t, rootPath)

	// list --dag triggers partial parsing during discovery.
	// The child includes root.hcl with expose=true, and root.hcl has a dependency
	// whose outputs aren't available. During partial parse, this should be tolerated.
	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt list --dag --format=dot --non-interactive --working-dir "+rootPath,
	)
	require.NoError(
		t,
		err,
		"Partial parsing should succeed even when exposed include has unresolved dependency",
	)
	assert.Contains(t, stdout, "child")
	assert.Contains(t, stdout, "unreachable-dep")
}

// TestExposedIncludeFullParseReturnsError verifies that full parsing surfaces an error when an
// included config (with expose=true) has a dependency whose outputs cannot be resolved.
// This ensures we only swallow include resolution errors during partial parse, not during full parse.
func TestExposedIncludeFullParseReturnsError(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureExposedIncludePartialParseError)
	rootPath := filepath.Join(tmpEnvPath, testFixtureExposedIncludePartialParseError)

	helpers.CleanupTerraformFolder(t, rootPath)

	childPath := filepath.Join(rootPath, "child")
	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --non-interactive --working-dir "+childPath+" -- plan",
	)
	require.Error(t, err, "Full parsing should fail when exposed include has unresolved dependency")
}

// encodeForgedManifest builds a gob-encoded .terragrunt-module-manifest with one file entry per supplied path so tests can plant adversarial inputs.
func encodeForgedManifest(t *testing.T, entries ...forgedManifestEntry) []byte {
	t.Helper()

	var buf bytes.Buffer

	enc := gob.NewEncoder(&buf)

	for _, entry := range entries {
		require.NoError(t, enc.Encode(entry))
	}

	return buf.Bytes()
}

func readForgedManifestEntries(t *testing.T, path string) []forgedManifestEntry {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, file.Close())
	}()

	decoder := gob.NewDecoder(file)
	entries := []forgedManifestEntry{}

	for {
		var entry forgedManifestEntry
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				return entries
			}

			require.NoError(t, err)
		}

		entries = append(entries, entry)
	}
}

// forgedManifestEntry mirrors the gob manifest entry layout for adversarial fixtures.
type forgedManifestEntry struct {
	Path  string
	IsDir bool
}

func forgedFile(path string) forgedManifestEntry {
	return forgedManifestEntry{Path: path}
}

func forgedDir(path string) forgedManifestEntry {
	return forgedManifestEntry{Path: path, IsDir: true}
}

// findCachedFile returns the first path under cacheRoot whose basename matches name, or "" if not found.
func findCachedFile(t *testing.T, cacheRoot, name string) string {
	t.Helper()

	var found string

	walkErr := vfs.WalkDir(
		vfs.NewOSFS(),
		cacheRoot,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !d.IsDir() && filepath.Base(path) == name {
				found = path
				return filepath.SkipAll
			}

			return nil
		},
	)
	require.NoError(t, walkErr, "walking %s", cacheRoot)

	return found
}

// TestTerraformSourceReferencesDependencyIsRejected pins that a unit whose terraform.source references a dependency
// output is rejected with a clear, actionable error instead of a misleading downstream cascade. The module source is
// consumed during discovery and queue construction, before any dependency output exists, so it can never be resolved
// there. module-b's source consumes module-a's output; the run must fail up front naming module-b's source, not fail
// later on module-c with a "detected no outputs" that sends the user chasing the wrong unit.
func TestTerraformSourceReferencesDependencyIsRejected(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTerraformSourceRefsDependency)
	rootPath := filepath.Join(tmpEnvPath, testFixtureTerraformSourceRefsDependency)
	livePath := filepath.Join(rootPath, "live")
	modulesPath := filepath.Join(rootPath, "modules")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --all --non-interactive --working-dir "+livePath+" --source "+modulesPath+" -- apply",
	)

	// The error must name the offending terraform.source, not misattribute the failure to a downstream unit. The typed
	// error is asserted in TestPartialParseTerraformSourceReferencingDependencyReturnsClearError; here it flows through
	// the CLI as text, so match on the message and confirm the misleading downstream cascade is absent.
	require.ErrorContains(t, err, "cannot reference dependency outputs")
	assert.NotContains(t, stdout+stderr, "detected no outputs")
}
