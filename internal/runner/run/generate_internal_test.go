package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGeneratePath(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	absolutePath, err := filepath.Abs(filepath.Join(t.TempDir(), "somewhere", "file.tf"))
	require.NoError(t, err)

	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "relative path joined with working dir",
			path:     "tgen_providers.tf",
			expected: filepath.Join(workingDir, "tgen_providers.tf"),
		},
		{
			name:     "absolute path returned unchanged",
			path:     absolutePath,
			expected: absolutePath,
		},
		{
			name:     "relative path with subdirectories",
			path:     filepath.Join("modules", "tgen.tf"),
			expected: filepath.Join(workingDir, "modules", "tgen.tf"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, resolveGeneratePath(workingDir, tc.path))
		})
	}
}

func TestGenerateConfigWritesAndCleansStaleEntries(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:          "tgen_providers.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# providers stub",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
			"backend": {
				Path:          "tgen_backend.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# backend stub",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_backend.tf"))
	assert.FileExists(t, filepath.Join(workingDir, GenerateManifestName))

	// Second run drops the providers block; the manifest must clean up the stale file.
	secondRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"backend": {
				Path:          "tgen_backend.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# backend stub",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &secondRun))
	assert.NoFileExists(t, filepath.Join(workingDir, "tgen_providers.tf"), "stale generate output must be removed")
	assert.FileExists(t, filepath.Join(workingDir, "tgen_backend.tf"), "still-requested generate output must remain")
}

func TestGenerateConfigRenamedPathCleansOldFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:          "tgen_providers.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# providers v1",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"))

	renamedRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:          "tgen_provider_aws.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# providers v2",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &renamedRun))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_provider_aws.tf"), "renamed generate path must produce the new file")
	assert.NoFileExists(t, filepath.Join(workingDir, "tgen_providers.tf"), "previous generate output must be removed when path changes")
}

func TestGenerateConfigDisabledSkipPreservesFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:          "tgen_providers.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# providers stub",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))

	providersPath := filepath.Join(workingDir, "tgen_providers.tf")
	require.FileExists(t, providersPath)

	originalContents, err := os.ReadFile(providersPath)
	require.NoError(t, err)

	// disable = true with if_disabled = "skip" must leave the previously generated file alone.
	disabledRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:       "tgen_providers.tf",
				IfExists:   codegen.ExistsOverwrite,
				IfDisabled: codegen.DisabledSkip,
				Disable:    true,
				Contents:   "# disabled-run contents must not replace the existing file",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &disabledRun))
	assert.FileExists(t, providersPath, "if_disabled = \"skip\" must preserve the existing file across runs")

	actualContents, err := os.ReadFile(providersPath)
	require.NoError(t, err)
	assert.Equal(t, originalContents, actualContents, "if_disabled = \"skip\" must preserve the original file contents")
}

func TestGenerateConfigDisabledRemoveCleansFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:          "tgen_providers.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# providers stub",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))
	require.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"))

	// disable = true with if_disabled = "remove" must drop the file (codegen does this directly,
	// the manifest also marks it as not-current so a future run does not re-target it).
	removeRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:       "tgen_providers.tf",
				IfExists:   codegen.ExistsOverwrite,
				IfDisabled: codegen.DisabledRemove,
				Disable:    true,
				Contents:   "# providers stub",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &removeRun))
	assert.NoFileExists(t, filepath.Join(workingDir, "tgen_providers.tf"), "if_disabled = \"remove\" must remove the file")
}

func TestGenerateConfigIfExistsSkipDoesNotTrackUnownedExistingFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)
	userFile := filepath.Join(workingDir, "tgen_providers.tf")
	userContents := []byte("# user-owned provider config")

	require.NoError(t, os.WriteFile(userFile, userContents, 0o644))

	skipRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsSkip,
				Contents: "# terragrunt-generated provider config",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &skipRun))
	assert.FileExists(t, userFile, "if_exists = \"skip\" must preserve the existing file")

	actualContents, err := os.ReadFile(userFile)
	require.NoError(t, err)
	assert.Equal(t, userContents, actualContents, "if_exists = \"skip\" must leave user-owned contents unchanged")

	emptyRun := runcfg.RunConfig{}
	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &emptyRun))
	assert.FileExists(t, userFile, "unowned skipped files must not be cleaned up after the generate block is removed")

	actualContents, err = os.ReadFile(userFile)
	require.NoError(t, err)
	assert.Equal(t, userContents, actualContents, "cleanup must not rewrite user-owned skipped contents")
}

func TestGenerateConfigIgnoresUnsafeManifestEntries(t *testing.T) {
	t.Parallel()

	t.Run("absolute outside path", func(t *testing.T) {
		t.Parallel()

		caseWorkingDir := t.TempDir()
		protectedPath := filepath.Join(t.TempDir(), "do-not-delete.tf")

		assertUnsafeManifestEntryIsIgnored(t, caseWorkingDir, protectedPath, protectedPath)
	})

	t.Run("relative traversal outside path", func(t *testing.T) {
		t.Parallel()

		caseWorkingDir := t.TempDir()
		protectedPath := filepath.Clean(filepath.Join(caseWorkingDir, "..", "do-not-delete.tf"))

		assertUnsafeManifestEntryIsIgnored(t, caseWorkingDir, filepath.Join("..", "do-not-delete.tf"), protectedPath)
	})
}

func TestGenerateConfigMalformedManifestIsIgnored(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	require.NoError(t, os.WriteFile(filepath.Join(workingDir, GenerateManifestName), []byte(`not-json`), 0o644))

	cfg := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:          "tgen_providers.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# providers stub",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &cfg))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"))
}

func TestGenerateConfigCleansStaleRemoteStateGenerateBeforeBackendValidation(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)
	opts.TerragruntConfigPath = filepath.Join(workingDir, "terragrunt.hcl")

	staleBackend := filepath.Join(workingDir, "backend.tf")
	require.NoError(t, os.WriteFile(staleBackend, []byte("# "+codegen.TerragruntGeneratedSignature+"\nterraform { backend \"s3\" {} }"), 0o644))
	require.NoError(t, writeGenerateManifest(logger.CreateLogger(), workingDir, map[string]struct{}{"backend.tf": {}}))

	cfg := runcfg.RunConfig{
		RemoteState: remotestate.RemoteState{
			Config: &remotestate.Config{BackendName: "s3"},
		},
	}

	err := GenerateConfig(logger.CreateLogger(), opts, &cfg)
	require.Error(t, err)

	var backendErr BackendNotDefined
	require.ErrorAs(t, err, &backendErr, "expected backend validation to run after stale generated backend cleanup")
	assert.NoFileExists(t, staleBackend, "stale remote_state.generate output must be removed before backend validation")
}

func TestGenerateConfigDoesNotDeleteSourceFileReplacingStaleGeneratedFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)
	backendPath := filepath.Join(workingDir, "backend.tf")
	sourceContents := []byte(`terraform { backend "s3" {} }`)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"backend": {
				Path:          "backend.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# generated backend",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))
	require.FileExists(t, backendPath)
	require.NoError(t, os.WriteFile(backendPath, sourceContents, 0o644))

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &runcfg.RunConfig{}))
	assert.FileExists(t, backendPath, "source files copied over a previously generated path must survive stale cleanup")

	actualContents, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Equal(t, sourceContents, actualContents)
}

func TestGenerateConfigIfExistsSkipAdoptsSignedGeneratedFileAfterUpgrade(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)
	generatedPath := filepath.Join(workingDir, "tgen_providers.tf")

	require.NoError(t, os.WriteFile(generatedPath, []byte("# "+codegen.TerragruntGeneratedSignature+"\n# generated before manifest existed"), 0o644))

	skipRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsSkip,
				Contents: "# would be skipped",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &skipRun))
	require.FileExists(t, generatedPath)

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &runcfg.RunConfig{}))
	assert.NoFileExists(t, generatedPath, "signed generated files from before manifest tracking should be adopted and cleaned when removed")
}

func TestGenerateConfigDoesNotReplaceManifestWhenLaterGenerationFails(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"old": {
				Path:          "old.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# old generated file",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))

	failingRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"new": {
				Path:          "new.tf",
				IfExists:      codegen.ExistsOverwrite,
				Contents:      "# new generated file",
				CommentPrefix: codegen.DefaultCommentPrefix,
			},
		},
		RemoteState: *remotestate.New(
			&remotestate.Config{
				BackendName: "s3",
				Generate: &remotestate.ConfigGenerate{
					Path:     "backend.tf",
					IfExists: "not-a-valid-if-exists-value",
				},
			},
		),
	}

	require.Error(t, GenerateConfig(logger.CreateLogger(), opts, &failingRun))

	manifestPaths, err := readGenerateManifest(logger.CreateLogger(), workingDir)
	require.NoError(t, err)
	assert.Contains(t, manifestPaths, "old.tf")
	assert.NotContains(t, manifestPaths, "new.tf")
}

func assertUnsafeManifestEntryIsIgnored(t *testing.T, workingDir string, manifestPath string, protectedPath string) {
	t.Helper()

	require.NoError(t, os.WriteFile(protectedPath, []byte("# outside cache"), 0o644))

	manifestContents, err := json.Marshal([]string{manifestPath})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, GenerateManifestName), manifestContents, 0o644))

	require.NoError(t, GenerateConfig(logger.CreateLogger(), newGenerateTestOptions(workingDir), &runcfg.RunConfig{}))
	assert.FileExists(t, protectedPath, "manifest entries outside the working dir must never be removed")
}

func newGenerateTestOptions(workingDir string) *Options {
	return &Options{
		WorkingDir:       workingDir,
		DownloadDir:      workingDir,
		TerraformCliArgs: iacargs.New(),
	}
}
