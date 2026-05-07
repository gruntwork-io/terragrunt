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

	testCases := []struct {
		name       string
		workingDir string
		path       string
		expected   string
	}{
		{
			name:       "relative path joined with working dir",
			workingDir: "/cache/working",
			path:       "tgen_providers.tf",
			expected:   "/cache/working/tgen_providers.tf",
		},
		{
			name:       "absolute path returned unchanged",
			workingDir: "/cache/working",
			path:       "/etc/somewhere/file.tf",
			expected:   "/etc/somewhere/file.tf",
		},
		{
			name:       "relative path with subdirectories",
			workingDir: "/cache/working",
			path:       "modules/tgen.tf",
			expected:   "/cache/working/modules/tgen.tf",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, resolveGeneratePath(tc.workingDir, tc.path))
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
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# providers stub",
			},
			"backend": {
				Path:     "tgen_backend.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# backend stub",
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
				Path:     "tgen_backend.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# backend stub",
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
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# providers v1",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"))

	renamedRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:     "tgen_provider_aws.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# providers v2",
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
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# providers stub",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &firstRun))
	require.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"))

	// disable = true with if_disabled = "skip" must leave the previously generated file alone.
	disabledRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:       "tgen_providers.tf",
				IfExists:   codegen.ExistsOverwrite,
				IfDisabled: codegen.DisabledSkip,
				Disable:    true,
				Contents:   "# providers stub",
			},
		},
	}

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &disabledRun))
	assert.FileExists(t, filepath.Join(workingDir, "tgen_providers.tf"), "if_disabled = \"skip\" must preserve the existing file across runs")
}

func TestGenerateConfigDisabledRemoveCleansFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	firstRun := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# providers stub",
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

	require.NoError(t, os.WriteFile(userFile, []byte("# user-owned provider config"), 0o644))

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

	emptyRun := runcfg.RunConfig{}
	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &emptyRun))
	assert.FileExists(t, userFile, "unowned skipped files must not be cleaned up after the generate block is removed")
}

func TestGenerateConfigIgnoresUnsafeManifestEntries(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "do-not-delete.tf")

	require.NoError(t, os.WriteFile(outsideFile, []byte("# outside cache"), 0o644))

	manifestContents, err := json.Marshal([]string{outsideFile})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, GenerateManifestName), manifestContents, 0o644))

	require.NoError(t, GenerateConfig(logger.CreateLogger(), opts, &runcfg.RunConfig{}))
	assert.FileExists(t, outsideFile, "manifest entries outside the working dir must never be removed")
}

func TestGenerateConfigMalformedManifestIsIgnored(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newGenerateTestOptions(workingDir)

	require.NoError(t, os.WriteFile(filepath.Join(workingDir, GenerateManifestName), []byte(`not-json`), 0o644))

	cfg := runcfg.RunConfig{
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"providers": {
				Path:     "tgen_providers.tf",
				IfExists: codegen.ExistsOverwrite,
				Contents: "# providers stub",
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
	require.NoError(t, os.WriteFile(staleBackend, []byte(`terraform { backend "s3" {} }`), 0o644))
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

func newGenerateTestOptions(workingDir string) *Options {
	return &Options{
		WorkingDir:       workingDir,
		DownloadDir:      workingDir,
		TerraformCliArgs: iacargs.New(),
	}
}
