// //go:build parse

// These tests consume so much memory that they cause the CI runner to crash.
// As a result, we have to run them on their own.
//
// In the future, we should make improvements to parsing so that this isn't necessary.

package test_test

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var knownBadFiles = []string{
	"fixtures/disabled/unit-disabled/terragrunt.hcl",
	"fixtures/hclfmt-errors/dangling-attribute/terragrunt.hcl",
	"fixtures/hclfmt-errors/invalid-character/terragrunt.hcl",
	"fixtures/hclfmt-errors/invalid-key/terragrunt.hcl",
	"fixtures/hclvalidate/second/a/terragrunt.hcl",
}

var knownFilesRequiringAuthentication = []string{
	"fixtures/assume-role/external-id-with-comma/terragrunt.hcl",
	"fixtures/assume-role/external-id/terragrunt.hcl",
	"fixtures/auth-provider-cmd/creds-for-dependency/dependency/terragrunt.hcl",
	"fixtures/auth-provider-cmd/creds-for-dependency/dependent/terragrunt.hcl",
	"fixtures/auth-provider-cmd/multiple-apps/app2/terragrunt.hcl",
	"fixtures/auth-provider-cmd/remote-state/terragrunt.hcl",
	"fixtures/auth-provider-cmd/sops/terragrunt.hcl",
	"fixtures/get-aws-caller-identity/terragrunt.hcl",
	"fixtures/get-output/regression-906/a/terragrunt.hcl",
	"fixtures/get-output/regression-906/b/terragrunt.hcl",
	"fixtures/get-output/regression-906/c/terragrunt.hcl",
	"fixtures/get-output/regression-906/d/terragrunt.hcl",
	"fixtures/get-output/regression-906/e/terragrunt.hcl",
	"fixtures/get-output/regression-906/f/terragrunt.hcl",
	"fixtures/get-output/regression-906/g/terragrunt.hcl",
	"fixtures/output-from-remote-state/env1/app2/terragrunt.hcl",
	"fixtures/read-config/iam_role_in_file/terragrunt.hcl",
	"fixtures/sops-kms/terragrunt.hcl",
}

var knownFilesRequiringVersionCheck = []string{
	"fixtures/version-check/a/terragrunt.hcl",
	"fixtures/version-check/b/terragrunt.hcl",
}

func TestParseAllFixtureFiles(t *testing.T) {
	t.Parallel()

	files := helpers.HCLFilesInDir(t, "fixtures")

	for _, file := range files {
		// Skip files in a .terragrunt-cache directory
		if strings.Contains(file, ".terragrunt-cache") {
			continue
		}

		t.Run(file, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Dir(file)

			opts, err := options.NewTerragruntOptionsForTest(dir)
			require.NoError(t, err)

			opts.Experiments.ExperimentMode()

			ctx := config.NewParsingContext(context.Background(), opts)

			cfg, _ := config.ParseConfigFile(ctx, file, nil)

			if slices.Contains(knownBadFiles, file) {
				assert.Nil(t, cfg)

				return
			}

			assert.NotNil(t, cfg)

			// Suggest garbage collection to free up memory.
			// Parsing config files can be memory intensive, and we don't need the config
			// files in memory after we've parsed them.
			runtime.GC()
		})
	}
}

func TestRenderAllFixtureFiles(t *testing.T) {
	t.Parallel()

	files := helpers.HCLFilesInDir(t, "fixtures")

	for _, file := range files {
		if !strings.HasSuffix(file, "terragrunt.hcl") {
			continue
		}

		if strings.Contains(file, ".terragrunt-cache") {
			continue
		}

		if slices.Contains(knownBadFiles, file) {
			continue
		}

		if slices.Contains(knownFilesRequiringAuthentication, file) {
			continue
		}

		if slices.Contains(knownFilesRequiringVersionCheck, file) {
			continue
		}

		t.Run(file, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(file)
			require.NoError(t, err)

			parsingCtx := config.NewParsingContext(context.Background(), opts)
			cfg, err := config.ParseConfigFile(parsingCtx, file, nil)
			require.NoError(t, err)

			dir := filepath.Dir(file)

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt render --experiment cli-redesign --working-dir "+dir,
			)
			require.NoError(t, err)

			assert.NotEmpty(t, stderr)

			renderedCfg, err := config.ParseConfigString(parsingCtx, file, stdout, nil)
			require.NoError(t, err)

			assert.Equal(t, cfg.Locals, renderedCfg.Locals)

			if cfg.Terraform != nil {
				assert.Equal(t, cfg.Terraform.Source, renderedCfg.Terraform.Source)
				assert.Equal(t, cfg.Terraform.ExtraArgs, renderedCfg.Terraform.ExtraArgs)
				assert.Equal(t, cfg.Terraform.BeforeHooks, renderedCfg.Terraform.BeforeHooks)
				assert.Equal(t, cfg.Terraform.AfterHooks, renderedCfg.Terraform.AfterHooks)
				assert.Equal(t, cfg.Terraform.ErrorHooks, renderedCfg.Terraform.ErrorHooks)
			}

			if cfg.RemoteState != nil {
				assert.Equal(t, cfg.RemoteState.BackendName, renderedCfg.RemoteState.BackendName)
				assert.Equal(t, cfg.RemoteState.Config.DisableInit, renderedCfg.RemoteState.Config.DisableInit)
				assert.Equal(t, cfg.RemoteState.Config.DisableDependencyOptimization, renderedCfg.RemoteState.Config.DisableDependencyOptimization)
				assert.Equal(t, cfg.RemoteState.Config.BackendConfig, renderedCfg.RemoteState.Config.BackendConfig)
			}

			if cfg.Dependencies != nil {
				assert.Equal(t, cfg.Dependencies.Paths, renderedCfg.Dependencies.Paths)
			}

			if len(cfg.TerragruntDependencies) > 0 {
				assert.Equal(t, cfg.TerragruntDependencies, renderedCfg.TerragruntDependencies)
			}

			assert.Equal(t, cfg.GenerateConfigs, renderedCfg.GenerateConfigs)
			assert.Equal(t, cfg.FeatureFlags, renderedCfg.FeatureFlags)
			assert.Equal(t, cfg.TerraformBinary, renderedCfg.TerraformBinary)
			assert.Equal(t, cfg.TerraformVersionConstraint, renderedCfg.TerraformVersionConstraint)
			assert.Equal(t, cfg.TerragruntVersionConstraint, renderedCfg.TerragruntVersionConstraint)
			assert.Equal(t, cfg.DownloadDir, renderedCfg.DownloadDir)
			assert.Equal(t, cfg.PreventDestroy, renderedCfg.PreventDestroy)
			assert.Equal(t, cfg.Skip, renderedCfg.Skip)
			assert.Equal(t, cfg.IamRole, renderedCfg.IamRole)
			assert.Equal(t, cfg.IamAssumeRoleDuration, renderedCfg.IamAssumeRoleDuration)
			assert.Equal(t, cfg.IamAssumeRoleSessionName, renderedCfg.IamAssumeRoleSessionName)
			assert.Equal(t, cfg.RetryMaxAttempts, renderedCfg.RetryMaxAttempts)
			assert.Equal(t, cfg.RetrySleepIntervalSec, renderedCfg.RetrySleepIntervalSec)
			assert.Equal(t, cfg.RetryableErrors, renderedCfg.RetryableErrors)
			assert.Equal(t, cfg.Inputs, renderedCfg.Inputs)

			// Suggest garbage collection to free up memory.
			// Parsing config files can be memory intensive, and we don't need the config
			// files in memory after we've parsed them.
			runtime.GC()
		})
	}
}

func TestParseFindListAllComponents(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --experiment cli-redesign --no-color"},
		{name: "list", command: "terragrunt list --experiment cli-redesign --no-color"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			assert.NotEmpty(t, stdout)

			fields := strings.Fields(stdout)

			aDepLine := 0
			bDepLine := 0

			for i, field := range fields {
				if field == "fixtures/find/dag/a-dependent" {
					aDepLine = i
				}

				if field == "fixtures/find/dag/b-dependency" {
					bDepLine = i
				}
			}

			assert.Less(t, aDepLine, bDepLine)
		})
	}
}

func TestParseFindListAllComponentsWithDAG(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --experiment cli-redesign --no-color --dag"},
		{name: "list", command: "terragrunt list --experiment cli-redesign --no-color --dag"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			assert.NotEmpty(t, stderr)
			assert.NotEmpty(t, stdout)

			fields := strings.Fields(stdout)

			aDepLine := 0
			bDepLine := 0

			for i, field := range fields {
				if field == "fixtures/find/dag/a-dependent" {
					aDepLine = i
				}

				if field == "fixtures/find/dag/b-dependency" {
					bDepLine = i
				}
			}

			assert.Greater(t, aDepLine, bDepLine)
		})
	}
}

func TestParseFindListAllComponentsWithDAGAndExternal(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name    string
		command string
	}{
		{name: "find", command: "terragrunt find --experiment cli-redesign --no-color --dag --external"},
		{name: "list", command: "terragrunt list --experiment cli-redesign --no-color --dag --external"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				tt.command,
			)
			require.NoError(t, err)

			assert.NotEmpty(t, stderr)
			assert.NotEmpty(t, stdout)

			fields := strings.Fields(stdout)

			aDepLine := 0
			bDepLine := 0

			for i, field := range fields {
				if field == "fixtures/find/dag/a-dependent" {
					aDepLine = i
				}

				if field == "fixtures/find/dag/b-dependency" {
					bDepLine = i
				}
			}

			assert.Greater(t, aDepLine, bDepLine)
		})
	}
}
