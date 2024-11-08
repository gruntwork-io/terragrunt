// Package test_test contains integration tests for Terragrunt.
package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/cli"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hard-code this to match the test fixture for now
const (
	testCommandsThatNeedInput                 = "fixtures/commands-that-need-input"
	testFixtureAuthProviderCmd                = "fixtures/auth-provider-cmd"
	testFixtureAutoInit                       = "fixtures/download/init-on-source-change"
	testFixtureBrokenDependency               = "fixtures/broken-dependency"
	testFixtureCodegenPath                    = "fixtures/codegen"
	testFixtureConfigSingleJSONPath           = "fixtures/config-files/single-json-config"
	testFixtureConfigWithNonDefaultNames      = "fixtures/config-files/with-non-default-names"
	testFixtureDependencyOutput               = "fixtures/dependency-output"
	testFixtureDirsPath                       = "fixtures/dirs"
	testFixtureDisabledModule                 = "fixtures/disabled/"
	testFixtureDisabledPath                   = "fixtures/disabled-path/"
	testFixtureDisjoint                       = "fixtures/stack/disjoint"
	testFixtureDownload                       = "fixtures/download"
	testFixtureEmptyState                     = "fixtures/empty-state/"
	testFixtureEnvVarsBlockPath               = "fixtures/env-vars-block/"
	testFixtureExcludesFile                   = "fixtures/excludes-file"
	testFixtureExitCode                       = "fixtures/exit-code"
	testFixtureExternalDependence             = "fixtures/external-dependencies"
	testFixtureExternalDependency             = "fixtures/external-dependency/"
	testFixtureExtraArgsPath                  = "fixtures/extra-args/"
	testFixtureFailedTerraform                = "fixtures/failure"
	testFixtureGetOutput                      = "fixtures/get-output"
	testFixtureGetTerragruntSourceCli         = "fixtures/get-terragrunt-source-cli"
	testFixtureGraphDependencies              = "fixtures/graph-dependencies"
	testFixtureHclfmtDiff                     = "fixtures/hclfmt-diff"
	testFixtureHclvalidate                    = "fixtures/hclvalidate"
	testFixtureIamRolesMultipleModules        = "fixtures/read-config/iam_roles_multiple_modules"
	testFixtureIncludeParent                  = "fixtures/include-parent"
	testFixtureInfoError                      = "fixtures/terragrunt-info-error"
	testFixtureInitCache                      = "fixtures/init-cache"
	testFixtureInitError                      = "fixtures/init-error"
	testFixtureInitOnce                       = "fixtures/init-once"
	testFixtureInputs                         = "fixtures/inputs"
	testFixtureInputsFromDependency           = "fixtures/inputs-from-dependency"
	testFixtureLogFormatter                   = "fixtures/log/formatter"
	testFixtureLogRelPaths                    = "fixtures/log/rel-paths"
	testFixtureMissingDependence              = "fixtures/missing-dependencies/main"
	testFixtureModulePathError                = "fixtures/module-path-in-error"
	testFixtureNoColor                        = "fixtures/no-color"
	testFixtureNoSubmodules                   = "fixtures/no-submodules/"
	testFixtureNullValue                      = "fixtures/null-values"
	testFixtureOutDir                         = "fixtures/out-dir"
	testFixtureOutputAll                      = "fixtures/output-all"
	testFixtureOutputModuleGroups             = "fixtures/output-module-groups"
	testFixtureParallelRun                    = "fixtures/parallel-run"
	testFixtureParallelStateInit              = "fixtures/parallel-state-init"
	testFixtureParallelism                    = "fixtures/parallelism"
	testFixturePath                           = "fixtures/terragrunt/"
	testFixturePlanfileOrder                  = "fixtures/planfile-order-test"
	testFixtureProviderCacheDirect            = "fixtures/provider-cache/direct"
	testFixtureProviderCacheFilesystemMirror  = "fixtures/provider-cache/filesystem-mirror"
	testFixtureProviderCacheMultiplePlatforms = "fixtures/provider-cache/multiple-platforms"
	testFixtureProviderCacheNetworkMirror     = "fixtures/provider-cache/network-mirror"
	testFixtureReadConfig                     = "fixtures/read-config"
	testFixtureRefSource                      = "fixtures/download/remote-ref"
	testFixtureSkip                           = "fixtures/skip/"
	testFixtureSkipDependencies               = "fixtures/skip-dependencies"
	testFixtureSourceMapSlashes               = "fixtures/source-map/slashes-in-ref"
	testFixtureStack                          = "fixtures/stack/"
	testFixtureStdout                         = "fixtures/download/stdout-test"
	testFixtureTfTest                         = "fixtures/tftest/"
	testFixtureErrorPrint                     = "fixtures/error-print"
	testFixtureBufferModuleOutput             = "fixtures/buffer-module-output"
	testFixtureDependenciesOptimisation       = "fixtures/dependency-optimisation"

	terraformFolder = ".terraform"

	terraformState = "terraform.tfstate"

	terraformRemoteStateS3Region = "us-west-2"

	terraformStateBackup = "terraform.tfstate.backup"
	terragruntCache      = ".terragrunt-cache"

	terraformBinary = "terraform"
	tofuBinary      = "tofu"
)

func TestLogCustomFormatOutput(t *testing.T) {
	t.Parallel()

	var (
		absPathReg = `(?:/[^/]+)*/` + regexp.QuoteMeta(testFixtureLogFormatter)
	)

	testCases := []struct {
		logCustomFormat    string
		expectedOutputRegs []*regexp.Regexp
	}{
		{
			logCustomFormat: "%time %level %prefix %msg",
			expectedOutputRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{2}:\d{2}:\d{2}\.\d{3} debug ` + absPathReg + regexp.QuoteMeta(" Terragrunt Version: 0.0.0")),
				regexp.MustCompile(`\d{2}:\d{2}:\d{2}\.\d{3} debug ` + absPathReg + `/dep Module ` + absPathReg + `/dep must wait for 0 dependencies to finish`),
				regexp.MustCompile(`\d{2}:\d{2}:\d{2}\.\d{3} debug ` + absPathReg + `/app Module ` + absPathReg + `/app must wait for 1 dependencies to finish`),
			},
		},
		{
			logCustomFormat: "%interval %level(case=upper) %prefix(path=relative-module,prefix='[',suffix='] ')%msg(path=relative)",
			expectedOutputRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" DEBUG Terragrunt Version: 0.0.0")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" DEBUG [dep] Module ./dep must wait for 0 dependencies to finish")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" DEBUG [app] Module ./app must wait for 1 dependencies to finish")),
			},
		},
		{
			logCustomFormat: "%interval %level(case=upper,width=6) %prefix(path=relative-module,suffix=' ')%tfpath(suffix=': ')%msg(path=relative)",
			expectedOutputRegs: []*regexp.Regexp{
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" DEBUG  Terragrunt Version: 0.0.0")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" STDOUT dep "+wrappedBinary()+": Initializing the backend...")),
				regexp.MustCompile(`\d{4}` + regexp.QuoteMeta(" STDOUT app "+wrappedBinary()+": Initializing the backend...")),
			},
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			cleanupTerraformFolder(t, testFixtureLogFormatter)
			tmpEnvPath := copyEnvironment(t, testFixtureLogFormatter)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

			_, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all init --terragrunt-log-level debug --terragrunt-non-interactive -no-color --terragrunt-no-color --terragrunt-log-custom-format=%q --terragrunt-working-dir %s", testCase.logCustomFormat, rootPath))
			require.NoError(t, err)

			for _, reg := range testCase.expectedOutputRegs {
				assert.Regexp(t, reg, stderr)
			}
		})
	}
}

func TestBufferModuleOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureBufferModuleOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureBufferModuleOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureBufferModuleOutput)

	_, _, err := runTerragruntCommandWithOutput(t, "terragrunt run-all plan -out planfile --terragrunt-log-disable --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt run-all show -json planfile --terragrunt-non-interactive --terragrunt-log-disable --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	for _, stdout := range strings.Split(stdout, "\n") {
		if stdout == "" {
			continue
		}

		var objmap map[string]json.RawMessage
		err = json.Unmarshal([]byte(stdout), &objmap)
		require.NoError(t, err)
	}
}

func TestDisableLogging(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := copyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	stdout, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt run-all init --terragrunt-log-level debug --terragrunt-log-disable --terragrunt-non-interactive -no-color --terragrunt-no-color --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stdout, "Initializing provider plugins...")
	assert.Empty(t, stderr)
}

func TestLogWithAbsPath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := copyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt run-all init --terragrunt-log-level debug --terragrunt-log-show-abs-paths --terragrunt-non-interactive -no-color --terragrunt-no-color --terragrunt-log-format=pretty --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		prefixName = filepath.Join(rootPath, prefixName)
		assert.Contains(t, stderr, "STDOUT ["+prefixName+"] "+wrappedBinary()+": Initializing provider plugins...")
		assert.Contains(t, stderr, "DEBUG  ["+prefixName+"] Reading Terragrunt config file at "+prefixName+"/terragrunt.hcl")
	}
}

func TestLogWithRelPath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLogRelPaths)
	tmpEnvPath := copyEnvironment(t, testFixtureLogRelPaths)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogRelPaths)

	testCases := []struct {
		workingDir string
		assertFn   func(t *testing.T, stdout, stderr string)
	}{
		{
			workingDir: "duplicate-dir-names/workspace/one/two/aaa", // dir `workspace` duplicated twice in path
			assertFn: func(t *testing.T, _, stderr string) {
				t.Helper()

				assert.Contains(t, stderr, "Module ./bbb/ccc/workspace")
				assert.Contains(t, stderr, "Module ./bbb/ccc/module-b")
				assert.Contains(t, stderr, "Downloading Terraform configurations from .. into ./bbb/ccc/workspace/.terragrunt-cache")
				assert.Contains(t, stderr, "[bbb/ccc/workspace]")
				assert.Contains(t, stderr, "[bbb/ccc/module-b]")
			},
		},
	}

	for i, testCase := range testCases {
		workingDir := filepath.Join(rootPath, testCase.workingDir)

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			stdout, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt run-all init --terragrunt-log-level debug --terragrunt-non-interactive -no-color --terragrunt-no-color --terragrunt-log-format=pretty --terragrunt-working-dir "+workingDir)
			require.NoError(t, err)

			testCase.assertFn(t, stdout, stderr)
		})
	}
}

func TestLogFormatPrettyOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := copyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	stdout, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt run-all init --terragrunt-log-level debug --terragrunt-non-interactive -no-color --terragrunt-no-color --terragrunt-log-format=pretty  --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		assert.Contains(t, stderr, "STDOUT ["+prefixName+"] "+wrappedBinary()+": Initializing provider plugins...")
		assert.Contains(t, stderr, "DEBUG  ["+prefixName+"] Reading Terragrunt config file at ./"+prefixName+"/terragrunt.hcl")
	}

	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "DEBUG  Terragrunt Version:")
}

func TestLogFormatKeyValueOutput(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--terragrunt-log-format=key-value", "--terragrunt-disable-log-formatting"} {
		t.Run("testCase-flag-"+flag, func(t *testing.T) {
			t.Parallel()

			cleanupTerraformFolder(t, testFixtureLogFormatter)
			tmpEnvPath := copyEnvironment(t, testFixtureLogFormatter)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

			_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt run-all init -no-color --terragrunt-log-level debug --terragrunt-non-interactive "+flag+" --terragrunt-working-dir "+rootPath)
			require.NoError(t, err)

			for _, prefixName := range []string{"app", "dep"} {
				assert.Contains(t, stderr, "level=stdout prefix="+prefixName+" tfpath="+wrappedBinary()+" msg=Initializing provider plugins...\n")
				assert.Contains(t, stderr, "level=debug prefix="+prefixName+" msg=Reading Terragrunt config file at ./"+prefixName+"/terragrunt.hcl\n")
			}
		})
	}
}

func TestLogRawModuleOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := copyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt run-all init -no-color --terragrunt-log-level debug --terragrunt-non-interactive  --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	stdoutInline := strings.ReplaceAll(stdout, "\n", "")
	assert.Contains(t, stdoutInline, "Initializing the backend...Initializing provider plugins...")
	assert.NotRegexp(t, regexp.MustCompile(`(?i)(`+strings.Join(log.AllLevels.Names(), "|")+`)+`), stdoutInline)
}

func TestTerragruntExcludesFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureExcludesFile, ".terragrunt-excludes")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureExcludesFile)

	tc := []struct {
		flags          string
		expectedOutput []string
	}{
		{
			"",
			[]string{`value = "b"`, `value = "d"`},
		},
		{
			"--terragrunt-excludes-file ./excludes-file-pass-as-flag",
			[]string{`value = "a"`, `value = "c"`},
		},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("tt-%d", i), func(t *testing.T) {
			t.Parallel()

			cleanupTerraformFolder(t, testFixtureExcludesFile)

			runTerragrunt(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s %s", rootPath, tt.flags))

			stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all output --terragrunt-non-interactive --terragrunt-working-dir %s %s", rootPath, tt.flags))
			require.NoError(t, err)

			actualOutput := strings.Split(strings.TrimSpace(stdout), "\n")
			assert.ElementsMatch(t, tt.expectedOutput, actualOutput)
		})
	}
}

func TestHclvalidateDiagnostic(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := copyEnvironment(t, testFixtureHclvalidate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

	expectedDiags := diagnostic.Diagnostics{
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Invalid expression",
			Detail:   "Expected the start of an expression, but found an invalid expression token.",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/a/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 2, Column: 6, Byte: 14},
				End:      diagnostic.Pos{Line: 3, Column: 1, Byte: 15},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "  t =\n}",
				StartLine:            2,
				HighlightStartOffset: 5,
				HighlightEndOffset:   6,
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Unsupported attribute",
			Detail:   "This object does not have an attribute named \"outputs\".",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 6, Column: 19, Byte: 86},
				End:      diagnostic.Pos{Line: 6, Column: 27, Byte: 94},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "",
				Code:                 "  c = dependency.a.outputs.z",
				StartLine:            6,
				HighlightStartOffset: 18,
				HighlightEndOffset:   26,
				Values:               []diagnostic.ExpressionValue{{Traversal: "dependency.a", Statement: "is object with no attributes"}},
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Missing required argument",
			Detail:   "The argument \"config_path\" is required, but no definition was found.",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 16, Column: 16, Byte: 219},
				End:      diagnostic.Pos{Line: 16, Column: 17, Byte: 220},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "dependency \"iam\"",
				Code:                 "dependency iam {",
				StartLine:            16,
				HighlightStartOffset: 15,
				HighlightEndOffset:   16,
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Can't evaluate expression",
			Detail:   "You can only reference to other local variables here, but it looks like you're referencing something else (\"dependency\" is not defined)",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 12, Column: 9, Byte: 149},
				End:      diagnostic.Pos{Line: 12, Column: 21, Byte: 161},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "  ddd = dependency.d",
				StartLine:            12,
				HighlightStartOffset: 8,
				HighlightEndOffset:   20,
			},
		},
		&diagnostic.Diagnostic{
			Severity: diagnostic.DiagnosticSeverity(hcl.DiagError),
			Summary:  "Can't evaluate expression",
			Detail:   "You can only reference to other local variables here, but it looks like you're referencing something else (\"dependency\" is not defined)",
			Range: &diagnostic.Range{
				Filename: filepath.Join(rootPath, "second/c/terragrunt.hcl"),
				Start:    diagnostic.Pos{Line: 10, Column: 9, Byte: 117},
				End:      diagnostic.Pos{Line: 10, Column: 31, Byte: 139},
			},
			Snippet: &diagnostic.Snippet{
				Context:              "locals",
				Code:                 "  vvv = dependency.a.outputs.z",
				StartLine:            10,
				HighlightStartOffset: 8,
				HighlightEndOffset:   30,
			},
		},
	}

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt hclvalidate --terragrunt-working-dir %s --terragrunt-hclvalidate-json", rootPath))
	require.NoError(t, err)

	var actualDiags diagnostic.Diagnostics

	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &actualDiags)
	require.NoError(t, err)

	assert.ElementsMatch(t, expectedDiags, actualDiags)
}

func TestHclvalidateInvalidConfigPath(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHclvalidate)
	tmpEnvPath := copyEnvironment(t, testFixtureHclvalidate)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclvalidate)

	expectedPaths := []string{
		filepath.Join(rootPath, "second/a/terragrunt.hcl"),
		filepath.Join(rootPath, "second/c/terragrunt.hcl"),
	}

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt hclvalidate --terragrunt-working-dir %s --terragrunt-hclvalidate-json --terragrunt-hclvalidate-show-config-path", rootPath))
	require.NoError(t, err)

	var actualPaths []string

	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &actualPaths)
	require.NoError(t, err)

	assert.ElementsMatch(t, expectedPaths, actualPaths)
}

func TestTerragruntProviderCacheMultiplePlatforms(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureProviderCacheMultiplePlatforms)
	tmpEnvPath := copyEnvironment(t, testFixtureProviderCacheMultiplePlatforms)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureProviderCacheMultiplePlatforms)

	providerCacheDir := t.TempDir()

	var (
		platforms     = []string{"linux_amd64", "darwin_arm64"}
		platformsArgs = make([]string, 0, len(platforms))
	)

	for _, platform := range platforms {
		platformsArgs = append(platformsArgs, "-platform="+platform)
	}

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all providers lock %s  --terragrunt-no-auto-init --terragrunt-provider-cache --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", strings.Join(platformsArgs, " "), providerCacheDir, rootPath))

	providers := []string{
		"hashicorp/aws/5.36.0",
		"hashicorp/azurerm/3.95.0",
	}

	registryName := "registry.opentofu.org"
	if isTerraform() {
		registryName = "registry.terraform.io"
	}

	for _, appName := range []string{"app1", "app2", "app3"} {
		appPath := filepath.Join(rootPath, appName)
		assert.True(t, util.FileExists(appPath))

		lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
		lockfileContent, err := os.ReadFile(lockfilePath)
		require.NoError(t, err)

		lockfile, diags := hclwrite.ParseConfig(lockfileContent, lockfilePath, hcl.Pos{Line: 1, Column: 1})
		assert.False(t, diags.HasErrors())
		assert.NotNil(t, lockfile)

		for _, provider := range providers {
			provider := path.Join(registryName, provider)

			providerBlock := lockfile.Body().FirstMatchingBlock("provider", []string{filepath.Dir(provider)})
			assert.NotNil(t, providerBlock)

			providerPath := filepath.Join(providerCacheDir, provider)
			assert.True(t, util.FileExists(providerPath))

			for _, platform := range platforms {
				platformPath := filepath.Join(providerPath, platform)
				assert.True(t, util.FileExists(platformPath))
			}
		}
	}
}

func TestTerragruntInitOnce(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureInitOnce)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInitOnce)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Initializing modules")

	// update the config creation time without changing content
	cfgPath := filepath.Join(rootPath, "terragrunt.hcl")
	bytes, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	err = os.WriteFile(cfgPath, bytes, 0644)
	require.NoError(t, err)

	stdout, _, err = runTerragruntCommandWithOutput(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)
	assert.NotContains(t, stdout, "Initializing modules", "init command executed more than once")
}

func TestTerragruntWorksWithSingleJsonConfig(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureConfigSingleJSONPath)
	tmpEnvPath := copyEnvironment(t, testFixtureConfigSingleJSONPath)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureConfigSingleJSONPath)

	runTerragrunt(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+rootTerragruntConfigPath)
}

func TestTerragruntWorksWithNonDefaultConfigNamesAndRunAllCommand(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureConfigWithNonDefaultNames)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureConfigWithNonDefaultNames)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt run-all apply --terragrunt-config main.hcl --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnvPath, &stdout, &stderr)
	require.NoError(t, err)

	out := stdout.String()
	assert.Equal(t, 1, strings.Count(out, "parent_hcl_file"))
	assert.Equal(t, 1, strings.Count(out, "dependency_hcl"))
	assert.Equal(t, 1, strings.Count(out, "common_hcl"))
}

func TestTerragruntWorksWithNonDefaultConfigNames(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureConfigWithNonDefaultNames)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureConfigWithNonDefaultNames)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply --terragrunt-config main.hcl --terragrunt-non-interactive --terragrunt-working-dir "+filepath.Join(tmpEnvPath, "app"), &stdout, &stderr)
	require.NoError(t, err)

	out := stdout.String()
	assert.Equal(t, 1, strings.Count(out, "parent_hcl_file"))
	assert.Equal(t, 1, strings.Count(out, "dependency_hcl"))
	assert.Equal(t, 1, strings.Count(out, "common_hcl"))
}

func TestTerragruntReportsTerraformErrorsWithPlanAll(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureFailedTerraform)
	tmpEnvPath := copyEnvironment(t, testFixtureFailedTerraform)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, "fixtures/failure")

	cmd := "terragrunt plan-all --terragrunt-non-interactive --terragrunt-working-dir " + rootTerragruntConfigPath
	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
	err := runTerragruntCommand(t, cmd, &stdout, &stderr)
	require.Error(t, err, "Failed to properly fail command: %v. The terraform should be bad", cmd)

	output := stdout.String()
	errOutput := stderr.String()
	fmt.Printf("STDERR is %s.\n STDOUT is %s", errOutput, output)

	require.ErrorContains(t, err, "missingvar1")
	require.ErrorContains(t, err, "missingvar2")
}

func TestTerragruntGraphDependenciesCommand(t *testing.T) {
	t.Parallel()

	// this test doesn't even run plan, it exits right after the stack was created
	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())

	tmpEnvPath := copyEnvironment(t, testFixtureGraphDependencies)

	rootTerragruntConfigPath := util.JoinPath(tmpEnvPath, testFixtureGraphDependencies, config.DefaultTerragruntConfigPath)
	copyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", "not-used")

	environmentPath := fmt.Sprintf("%s/%s/root", tmpEnvPath, testFixtureGraphDependencies)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)
	runTerragruntRedirectOutput(t, "terragrunt graph-dependencies --terragrunt-working-dir "+environmentPath, &stdout, &stderr)
	output := stdout.String()
	assert.True(t, strings.Contains(output, strings.TrimSpace(`
digraph {
	"backend-app" ;
	"backend-app" -> "mysql";
	"backend-app" -> "redis";
	"backend-app" -> "vpc";
	"frontend-app" ;
	"frontend-app" -> "backend-app";
	"frontend-app" -> "vpc";
	"mysql" ;
	"mysql" -> "vpc";
	"redis" ;
	"redis" -> "vpc";
	"vpc" ;
}
	`)))
}

// Check that Terragrunt does not pollute stdout with anything
func TestTerragruntStdOut(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureStdout)
	runTerragruntRedirectOutput(t, "terragrunt output foo --terragrunt-non-interactive --terragrunt-working-dir "+testFixtureStdout, &stdout, &stderr)

	output := stdout.String()
	assert.Equal(t, "\"foo\"\n", output)
}

func TestTerragruntStackCommandsWithPlanFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := filepath.EvalSymlinks(copyEnvironment(t, testFixtureDisjoint))
	require.NoError(t, err)
	disjointEnvironmentPath := util.JoinPath(tmpEnvPath, testFixtureDisjoint)

	cleanupTerraformFolder(t, disjointEnvironmentPath)
	runTerragrunt(t, "terragrunt plan-all -out=plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir "+disjointEnvironmentPath)
	runTerragrunt(t, "terragrunt apply-all plan.tfplan --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir "+disjointEnvironmentPath)
}

func TestInvalidSource(t *testing.T) {
	t.Parallel()

	generateTestCase := testFixtureNotExistingSource
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)

	var workingDirNotFoundErr terraform.WorkingDirNotFound

	ok := errors.As(err, &workingDirNotFoundErr)
	assert.True(t, ok)
}

// Run terragrunt plan -detailed-exitcode on a folder with some uncreated resources and make sure that you get an exit
// code of "2", which means there are changes to apply.
func TestExitCode(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixtureExitCode)
	modulePath := util.JoinPath(rootPath, testFixtureExitCode)
	err := runTerragruntCommand(t, "terragrunt plan -detailed-exitcode --terragrunt-non-interactive --terragrunt-working-dir "+modulePath, os.Stdout, os.Stderr)

	exitCode, exitCodeErr := util.GetExitCode(err)
	require.NoError(t, exitCodeErr)
	assert.Equal(t, 2, exitCode)
}

func TestPlanfileOrder(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixturePlanfileOrder)
	modulePath := util.JoinPath(rootPath, testFixturePlanfileOrder)

	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-working-dir "+modulePath, os.Stdout, os.Stderr)
	require.NoError(t, err)

	err = runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-working-dir "+modulePath, os.Stdout, os.Stderr)
	require.NoError(t, err)
}

// This tests terragrunt properly passes through terraform commands and any number of specified args
func TestTerraformCommandCliArgs(t *testing.T) {
	t.Parallel()

	tc := []struct {
		command     []string
		expected    string
		expectedErr error
	}{
		{
			[]string{"version"},
			wrappedBinary() + " version",
			nil,
		},
		{
			[]string{"version", "foo"},
			wrappedBinary() + " version foo",
			nil,
		},
		{
			[]string{"version", "foo", "bar", "baz"},
			wrappedBinary() + " version foo bar baz",
			nil,
		},
		{
			[]string{"version", "foo", "bar", "baz", "foobar"},
			wrappedBinary() + " version foo bar baz foobar",
			nil,
		},
		{
			[]string{"paln"}, //codespell:ignore
			"",
			expectedWrongCommandErr("paln"), //codespell:ignore
		},
		{
			[]string{"paln", "--terragrunt-disable-command-validation"}, //codespell:ignore
			wrappedBinary() + " invocation failed",                      // error caused by running terraform with the wrong command
			nil,
		},
	}

	for _, tt := range tc {
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", strings.Join(tt.command, " "), testFixtureExtraArgsPath)

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		err := runTerragruntCommand(t, cmd, &stdout, &stderr)
		if tt.expectedErr != nil {
			require.ErrorIs(t, err, tt.expectedErr)
		}

		output := stdout.String()
		errOutput := stderr.String()
		assert.True(t, strings.Contains(errOutput, tt.expected) || strings.Contains(output, tt.expected))
	}
}

// This tests terragrunt properly passes through terraform commands with sub commands
// and any number of specified args
func TestTerraformSubcommandCliArgs(t *testing.T) {
	t.Parallel()

	tc := []struct {
		command  []string
		expected string
	}{
		{
			[]string{"force-unlock"},
			wrappedBinary() + " force-unlock",
		},
		{
			[]string{"force-unlock", "foo"},
			wrappedBinary() + " force-unlock foo",
		},
		{
			[]string{"force-unlock", "foo", "bar", "baz"},
			wrappedBinary() + " force-unlock foo bar baz",
		},
		{
			[]string{"force-unlock", "foo", "bar", "baz", "foobar"},
			wrappedBinary() + " force-unlock foo bar baz foobar",
		},
	}

	for _, tt := range tc {
		cmd := fmt.Sprintf("terragrunt %s --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", strings.Join(tt.command, " "), testFixtureExtraArgsPath)

		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)
		// Call runTerragruntCommand directly because this command contains failures (which causes runTerragruntRedirectOutput to abort) but we don't care.
		if err := runTerragruntCommand(t, cmd, &stdout, &stderr); err == nil {
			t.Fatalf("Failed to properly fail command: %v.", cmd)
		}
		output := stdout.String()
		errOutput := stderr.String()
		assert.True(t, strings.Contains(errOutput, tt.expected) || strings.Contains(output, tt.expected))
	}
}

func validateInputs(t *testing.T, outputs map[string]TerraformOutput) {
	t.Helper()

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []interface{}{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []interface{}{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []interface{}{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(t, map[string]interface{}{"foo": true, "bar": false, "baz": true}, outputs["map_bool"].Value)
	assert.Equal(t, map[string]interface{}{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value, 0.0000000001)
	assert.Equal(t, map[string]interface{}{"list": []interface{}{1.0, 2.0, 3.0}, "map": map[string]interface{}{"foo": "bar"}, "num": 42.0, "str": "string"}, outputs["object"].Value)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

func TestInputsPassedThroughCorrectly(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := copyEnvironment(t, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	validateInputs(t, outputs)
}

type TerraformOutput struct {
	Sensitive bool        `json:"Sensitive"`
	Type      interface{} `json:"Type"`
	Value     interface{} `json:"Value"`
}

func TestTerragruntMissingDependenciesFail(t *testing.T) {
	t.Parallel()

	generateTestCase := testFixtureMissingDependence
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)
	var parsedError config.DependencyDirNotFoundError
	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.Dir, 1)
	assert.Contains(t, parsedError.Dir[0], "hl3-release")
}

func TestTerragruntExcludeExternalDependencies(t *testing.T) {
	t.Parallel()

	excludedModule := "module-a"
	includedModule := "module-b"

	modules := []string{
		excludedModule,
		includedModule,
	}

	cleanupTerraformFolder(t, testFixtureExternalDependence)
	for _, module := range modules {
		cleanupTerraformFolder(t, util.JoinPath(testFixtureExternalDependence, module))
	}

	var (
		applyAllStdout bytes.Buffer
		applyAllStderr bytes.Buffer
	)

	rootPath := copyEnvironment(t, testFixtureExternalDependence)
	modulePath := util.JoinPath(rootPath, testFixtureExternalDependence, includedModule)

	err := runTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-ignore-external-dependencies --terragrunt-forward-tf-stdout --terragrunt-working-dir "+modulePath, &applyAllStdout, &applyAllStderr)
	logBufferContentsLineByLine(t, applyAllStdout, "apply-all stdout")
	logBufferContentsLineByLine(t, applyAllStderr, "apply-all stderr")
	applyAllStdoutString := applyAllStdout.String()

	if err != nil {
		t.Errorf("Did not expect to get error: %s", err.Error())
	}

	assert.Contains(t, applyAllStdoutString, "Hello World, "+includedModule)
	assert.NotContains(t, applyAllStdoutString, "Hello World, "+excludedModule)
}

func TestApplySkipTrue(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixtureSkip)
	rootPath = util.JoinPath(rootPath, testFixtureSkip, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-log-level info --terragrunt-non-interactive --terragrunt-working-dir %s --var person=Hobbs", rootPath), &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	require.NoError(t, err)
	assert.Contains(t, stderr, "Skipping terragrunt module ./terragrunt.hcl due to skip = true.")
	assert.NotContains(t, stdout, "hello, Hobbs")
}

func TestApplySkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixtureSkip)
	rootPath = util.JoinPath(rootPath, testFixtureSkip, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stderr := showStderr.String()
	stdout := showStdout.String()

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello, Hobbs")
	assert.NotContains(t, stderr, "Skipping terragrunt module")
}

func TestApplyAllSkipTrue(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixtureSkip)
	rootPath = util.JoinPath(rootPath, testFixtureSkip, "skip-true")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir %s --terragrunt-log-level info", rootPath), &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	// this test is now prepared to handle the case where skip is inherited from the included terragrunt file
	// meaning the skip-true/resource2 module will be skipped as well and only the skip-true/resource1 module will be applied

	require.NoError(t, err)
	assert.Contains(t, stderr, "Skipping terragrunt module ./terragrunt.hcl due to skip = true.")
	assert.Contains(t, stdout, "hello, Ernie")
	assert.NotContains(t, stdout, "hello, Bert")
}

func TestApplyAllSkipFalse(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, testFixtureSkip)
	rootPath = util.JoinPath(rootPath, testFixtureSkip, "skip-false")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	stdout := showStdout.String()
	stderr := showStderr.String()

	require.NoError(t, err)
	assert.Contains(t, stdout, "hello, Hobbs")
	assert.Contains(t, stdout, "hello, Ernie")
	assert.Contains(t, stdout, "hello, Bert")
	assert.NotContains(t, stderr, "Skipping terragrunt module")
}

func TestDependencyOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "integration")

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := util.JoinPath(rootPath, "app3")
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+app3Path, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, 42, int(outputs["z"].Value.(float64)))
}

func TestDependencyOutputErrorBeforeApply(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")
	app3Path := filepath.Join(rootPath, "app3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+app3Path, &showStdout, &showStderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputSkipOutputs(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "integration")
	emptyPath := filepath.Join(rootPath, "empty")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	// Test that even if the dependency (app1) is not applied, using skip_outputs will skip pulling the outputs so there
	// will be no errors.
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+emptyPath, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputSkipOutputsWithMockOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent3Path := filepath.Join(rootPath, "dependent3")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+dependent3Path, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+dependent3Path, &stdout, &stderr),
	)
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)

	// Now apply-all so that the dependency is applied, and verify it still uses the mock output
	err = runTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+dependent3Path, &stdout, &stderr),
	)
	outputs = map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out.
func TestDependencyMockOutput(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent1Path := filepath.Join(rootPath, "dependent1")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+dependent1Path, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+dependent1Path, &stdout, &stderr),
	)
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 0", outputs["truth"].Value)

	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// Now apply-all so that the dependency is applied, and verify it uses the dependency output
	err = runTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// verify expected output when mocks are used: The answer is 0
	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+dependent1Path, &stdout, &stderr),
	)
	outputs = map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, "The answer is 42", outputs["truth"].Value)
}

// Test default behavior when mock_outputs_merge_with_state is not set. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it already has an output
// but not the newly added output.
func TestDependencyMockOutputMergeWithStateDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-default", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is not set
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribute is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

}

// Test when mock_outputs_merge_with_state is explicitly set to false. It should behave, as before this parameter was added
// It will fail on any command if the parent state is not applied, because the state of the parent exists and it already has an output
// but not the newly added output.
func TestDependencyMockOutputMergeWithStateFalse(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-false", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify we have the default behavior if mock_outputs_merge_with_state is set to false
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet, and the new attribute is not available and in
	// this case, mocked outputs are not used.
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output2\"")

	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// It will mock the newly added output from the parent as it was not already applied to the state.
func TestDependencyMockOutputMergeWithStateTrue(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-true", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true and some output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "fake-data2", outputs["test_output2_from_parent"].Value)

	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")
}

// Test when mock_outputs_merge_with_state is explicitly set to true, but using an unallowed command. It should ignore
// the mock output.
func TestDependencyMockOutputMergeWithStateTrueNotAllowed(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-true-validate-only", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "plan stdout")
	logBufferContentsLineByLine(t, stderr, "plan stderr")

	// Verify mocked outputs are used if mock_outputs_merge_with_state is set to true with an allowed command and some
	// output in the parent are not applied yet.
	stdout.Reset()
	stderr.Reset()
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr),
	)

	// ... but not when an unallowed command is used
	require.Error(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr),
	)
}

// Test when mock_outputs_merge_with_state is explicitly set to true.
// Mock should not be used as the parent state was already fully applied.
func TestDependencyMockOutputMergeWithStateNoOverride(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-with-state", "merge-with-state-no-override", "live")
	parentPath := filepath.Join(rootPath, "parent")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+parentPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "show stdout")
	logBufferContentsLineByLine(t, stderr, "show stderr")

	// Verify mocked outputs are not used if mock_outputs_merge_with_state is set to true and all outputs in the parent have been applied.
	stdout.Reset()
	stderr.Reset()
	err = runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)

	// Now check the outputs to make sure they are as expected
	stdout.Reset()
	stderr.Reset()

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "value2", outputs["test_output2_from_parent"].Value)

	logBufferContentsLineByLine(t, stdout, "show stdout")
	logBufferContentsLineByLine(t, stderr, "show stderr")
}

// Test when mock_outputs_merge_strategy_with_state or mock_outputs_merge_with_state is not set, the default is no_merge
func TestDependencyMockOutputMergeStrategyWithStateDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-default", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "false" that MergeStrategyType is set to no_merge
func TestDependencyMockOutputMergeStrategyWithStateCompatFalse(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-false", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_with_state = "true" that MergeStrategyType is set to shallow
func TestDependencyMockOutputMergeStrategyWithStateCompatTrue(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-true", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when both mock_outputs_merge_with_state and mock_outputs_merge_strategy_with_state are set, mock_outputs_merge_strategy_with_state is used
func TestDependencyMockOutputMergeStrategyWithStateCompatConflict(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-compat-true", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "no_merge" that mocks are not merged into the current state
func TestDependencyMockOutputMergeStrategyWithStateNoMerge(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-no-merge", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "This object does not have an attribute named \"test_output_list_string\"")
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")
}

// Test when mock_outputs_merge_strategy_with_state = "shallow" that only top level outputs are merged.
// Lists or keys in existing maps will not be merged
func TestDependencyMockOutputMergeStrategyWithStateShallow(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-shallow", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "fake-list-data", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test when mock_outputs_merge_strategy_with_state = "deep" that the existing state is deeply merged into the mocks
// so that the existing state overwrites the mocks. This allows child modules to use new dependency outputs before the
// dependency has been applied
func TestDependencyMockOutputMergeStrategyWithStateDeepMapOnly(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "mock-outputs-merge-strategy-with-state", "merge-strategy-with-state-deep-map-only", "live")
	childPath := filepath.Join(rootPath, "child")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "apply stdout")
	logBufferContentsLineByLine(t, stderr, "apply stderr")

	stdout.Reset()
	stderr.Reset()

	require.NoError(t, runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+childPath, &stdout, &stderr))
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	logBufferContentsLineByLine(t, stdout, "output stdout")
	logBufferContentsLineByLine(t, stderr, "output stderr")

	assert.Equal(t, "value1", outputs["test_output1_from_parent"].Value)
	assert.Equal(t, "fake-abc", outputs["test_output2_from_parent"].Value)
	assert.Equal(t, "map_root1_sub1_value", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "map_root1", "map_root1_sub1", "value"))
	assert.Equal(t, "fake-abc", util.MustWalkTerraformOutput(outputs["test_output_map_map_string_from_parent"].Value, "not_in_state", "abc", "value"))
	assert.Equal(t, "a", util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "0"))
	assert.Nil(t, util.MustWalkTerraformOutput(outputs["test_output_list_string"].Value, "1"))
}

// Test that when you have a mock_output on a dependency, the dependency will use the mock as the output instead
// of erroring out when running an allowed command.
func TestDependencyMockOutputRestricted(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "mock-outputs")
	dependent2Path := filepath.Join(rootPath, "dependent2")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+dependent2Path, &showStdout, &showStderr)
	require.Error(t, err)
	// Verify that we fail because the dependency is not applied yet
	assert.Contains(t, err.Error(), "has not been applied yet")

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify we can run when using one of the allowed commands
	showStdout.Reset()
	showStderr.Reset()
	err = runTerragruntCommand(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+dependent2Path, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Verify that validate-all works as well.
	showStdout.Reset()
	showStderr.Reset()
	err = runTerragruntCommand(t, "terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir "+dependent2Path, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	showStdout.Reset()
	showStderr.Reset()
	err = runTerragruntCommand(t, "terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr)
	require.NoError(t, err)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")
}

func TestDependencyOutputTypeConversion(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	cleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := copyEnvironment(t, ".")

	inputsPath := util.JoinPath(tmpEnvPath, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "type-conversion")

	// First apply the inputs module
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+inputsPath)

	// Then apply the outputs module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr),
	)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []interface{}{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []interface{}{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []interface{}{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(t, map[string]interface{}{"foo": true, "bar": false, "baz": true}, outputs["map_bool"].Value)
	assert.Equal(t, map[string]interface{}{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value.(float64), 0.0000001)
	assert.Equal(t, map[string]interface{}{"list": []interface{}{1.0, 2.0, 3.0}, "map": map[string]interface{}{"foo": "bar"}, "num": 42.0, "str": "string"}, outputs["object"].Value)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/1102: Ordering keys from
// maps to avoid random placements when terraform file is generated.
func TestOrderedMapOutputRegressions1102(t *testing.T) {
	t.Parallel()
	generateTestCase := filepath.Join(testFixtureGetOutput, "regression-1102")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	command := "terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir " + generateTestCase
	path := filepath.Join(generateTestCase, "backend.tf")

	// runs terragrunt for the first time and checks the output "backend.tf" file.
	require.NoError(
		t,
		runTerragruntCommand(t, command, &stdout, &stderr),
	)
	expected, _ := os.ReadFile(path)
	assert.Contains(t, string(expected), "local")

	// runs terragrunt again. All the outputs must be
	// equal to the first run.
	for i := 0; i < 20; i++ {
		require.NoError(
			t,
			runTerragruntCommand(t, command, &stdout, &stderr),
		)
		actual, _ := os.ReadFile(path)
		assert.Equal(t, expected, actual)
	}
}

// Test that we get the expected error message about dependency cycles when there is a cycle in the dependency chain
func TestDependencyOutputCycleHandling(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)

	tc := []string{
		"aa",
		"aba",
		"abca",
		"abcda",
	}

	for _, tt := range tc {
		// Capture range variable into forloop so that the binding is consistent across runs.
		tt := tt

		t.Run(tt, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
			rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "cycle", tt)
			fooPath := util.JoinPath(rootPath, "foo")

			planStdout := bytes.Buffer{}
			planStderr := bytes.Buffer{}
			err := runTerragruntCommand(
				t,
				"terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+fooPath,
				&planStdout,
				&planStderr,
			)
			logBufferContentsLineByLine(t, planStdout, "plan stdout")
			logBufferContentsLineByLine(t, planStderr, "plan stderr")
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), "Found a dependency cycle between modules"))
		})
	}
}

// Regression testing for https://github.com/gruntwork-io/terragrunt/issues/854: Referencing a dependency that is a
// subdirectory of the current config, which includes an `include` block has problems resolving the correct relative
// path.
func TestDependencyOutputRegression854(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-854", "root")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		"terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

// Regression testing for bug where terragrunt output runs on dependency blocks are done in the terragrunt-cache for the
// child, not the parent.
func TestDependencyOutputCachePathBug(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "localstate", "live")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		"terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestDependencyOutputWithTerragruntSource(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-1124", "live")
	modulePath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-1124", "modules")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		fmt.Sprintf("terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-source %s", rootPath, modulePath),
		&stdout,
		&stderr,
	)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestDependencyOutputWithHooks(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "regression-1273")
	depPathFileOut := util.JoinPath(rootPath, "dep", "file.out")
	mainPath := util.JoinPath(rootPath, "main")
	mainPathFileOut := util.JoinPath(mainPath, "file.out")

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	// We need to bust the output cache that stores the dependency outputs so that the second run pulls the outputs.
	// This is only a problem during testing, where the process is shared across terragrunt runs.
	config.ClearOutputCache()

	// The file should exist in the first run.
	assert.True(t, util.FileExists(depPathFileOut))
	assert.False(t, util.FileExists(mainPathFileOut))

	// Now delete file and run just main again. It should NOT create file.out.
	require.NoError(t, os.Remove(depPathFileOut))
	runTerragrunt(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+mainPath)
	assert.False(t, util.FileExists(depPathFileOut))
	assert.False(t, util.FileExists(mainPathFileOut))
}

func TestDeepDependencyOutputWithMock(t *testing.T) {
	// Test that the terraform command flows through for mock output retrieval to deeper dependencies. Previously the
	// terraform command was being overwritten, so by the time the deep dependency retrieval runs, it was replaced with
	// "output" instead of the original one.

	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureGetOutput, "nested-mocks", "live")

	// Since we haven't applied anything, this should only succeed if mock outputs are used.
	runTerragrunt(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestDataDir(t *testing.T) {
	// Cannot be run in parallel with other tests as it modifies process' environment.

	cleanupTerraformFolder(t, testFixtureDirsPath)
	tmpEnvPath := copyEnvironment(t, testFixtureDirsPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDirsPath)

	t.Setenv("TF_DATA_DIR", util.JoinPath(tmpEnvPath, "data_dir"))

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Initializing provider plugins")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stdout.String(), "Initializing provider plugins")
}

func TestReadTerragruntConfigWithDependency(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureReadConfig)
	cleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := copyEnvironment(t, ".")

	inputsPath := util.JoinPath(tmpEnvPath, testFixtureInputs)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReadConfig, "with_dependency")

	// First apply the inputs module
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+inputsPath)

	// Then apply the read config module
	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr),
	)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, true, outputs["bool"].Value)
	assert.Equal(t, []interface{}{true, false}, outputs["list_bool"].Value)
	assert.Equal(t, []interface{}{1.0, 2.0, 3.0}, outputs["list_number"].Value)
	assert.Equal(t, []interface{}{"a", "b", "c"}, outputs["list_string"].Value)
	assert.Equal(t, map[string]interface{}{"foo": true, "bar": false, "baz": true}, outputs["map_bool"].Value)
	assert.Equal(t, map[string]interface{}{"foo": 42.0, "bar": 12345.0}, outputs["map_number"].Value)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, outputs["map_string"].Value)
	assert.InEpsilon(t, 42.0, outputs["number"].Value.(float64), 0.0000001)
	assert.Equal(t, map[string]interface{}{"list": []interface{}{1.0, 2.0, 3.0}, "map": map[string]interface{}{"foo": "bar"}, "num": 42.0, "str": "string"}, outputs["object"].Value)
	assert.Equal(t, "string", outputs["string"].Value)
	assert.Equal(t, "default", outputs["from_env"].Value)
}

func TestReadTerragruntConfigFromDependency(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureReadConfig)
	tmpEnvPath := copyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureReadConfig, "from_dependency")

	showStdout := bytes.Buffer{}
	showStderr := bytes.Buffer{}
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &showStdout, &showStderr),
	)

	logBufferContentsLineByLine(t, showStdout, "show stdout")
	logBufferContentsLineByLine(t, showStderr, "show stderr")

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "hello world", outputs["bar"].Value)
}

func TestReadTerragruntConfigWithDefault(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureReadConfig)
	rootPath := util.JoinPath(testFixtureReadConfig, "with_default")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "default value", outputs["data"].Value)
}

func TestReadTerragruntConfigWithOriginalTerragruntDir(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureReadConfig)
	rootPath := util.JoinPath(testFixtureReadConfig, "with_original_terragrunt_dir")

	rootPathAbs, err := filepath.Abs(rootPath)
	require.NoError(t, err)
	fooPathAbs := filepath.Join(rootPathAbs, "foo")
	depPathAbs := filepath.Join(rootPathAbs, "dep")

	// Run apply on the dependency module and make sure we get the outputs we expect
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+depPathAbs)

	depStdout := bytes.Buffer{}
	depStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+depPathAbs, &depStdout, &depStderr),
	)

	depOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(depStdout.Bytes(), &depOutputs))

	assert.Equal(t, depPathAbs, depOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, depOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, depOutputs["bar_original_terragrunt_dir"].Value)

	// Run apply on the root module and make sure we get the expected outputs
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	rootStdout := bytes.Buffer{}
	rootStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &rootStdout, &rootStderr),
	)

	rootOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(rootStdout.Bytes(), &rootOutputs))

	assert.Equal(t, fooPathAbs, rootOutputs["terragrunt_dir"].Value)
	assert.Equal(t, rootPathAbs, rootOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, rootOutputs["dep_bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, rootOutputs["dep_bar_original_terragrunt_dir"].Value)

	// Run 'run-all apply' and make sure all the outputs are identical in the root module and the dependency module
	runTerragrunt(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	runAllRootStdout := bytes.Buffer{}
	runAllRootStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &runAllRootStdout, &runAllRootStderr),
	)

	runAllRootOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(runAllRootStdout.Bytes(), &runAllRootOutputs))

	runAllDepStdout := bytes.Buffer{}
	runAllDepStderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+depPathAbs, &runAllDepStdout, &runAllDepStderr),
	)

	runAllDepOutputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(runAllDepStdout.Bytes(), &runAllDepOutputs))

	assert.Equal(t, fooPathAbs, runAllRootOutputs["terragrunt_dir"].Value)
	assert.Equal(t, rootPathAbs, runAllRootOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, runAllRootOutputs["dep_bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllRootOutputs["dep_bar_original_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["original_terragrunt_dir"].Value)
	assert.Equal(t, fooPathAbs, runAllDepOutputs["bar_terragrunt_dir"].Value)
	assert.Equal(t, depPathAbs, runAllDepOutputs["bar_original_terragrunt_dir"].Value)
}

func TestReadTerragruntConfigFull(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureReadConfig)
	rootPath := util.JoinPath(testFixtureReadConfig, "full")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "terragrunt", outputs["terraform_binary"].Value)
	assert.Equal(t, "= 0.12.20", outputs["terraform_version_constraint"].Value)
	assert.Equal(t, "= 0.23.18", outputs["terragrunt_version_constraint"].Value)
	assert.Equal(t, ".terragrunt-cache", outputs["download_dir"].Value)
	assert.Equal(t, "TerragruntIAMRole", outputs["iam_role"].Value)
	assert.Equal(t, "true", outputs["skip"].Value)
	assert.Equal(t, "true", outputs["prevent_destroy"].Value)

	// Simple maps
	localstgOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["localstg"].Value.(string)), &localstgOut))
	assert.Equal(t, map[string]interface{}{"the_answer": float64(42)}, localstgOut)
	inputsOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["inputs"].Value.(string)), &inputsOut))
	assert.Equal(t, map[string]interface{}{"doc": "Emmett Brown"}, inputsOut)

	// Complex blocks
	depsOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["dependencies"].Value.(string)), &depsOut))
	assert.Equal(
		t,
		map[string]interface{}{
			"paths": []interface{}{"../../terragrunt"},
		},
		depsOut,
	)

	generateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["generate"].Value.(string)), &generateOut))
	assert.Equal(
		t,
		map[string]interface{}{
			"provider": map[string]interface{}{
				"path":              "provider.tf",
				"if_exists":         "overwrite_terragrunt",
				"if_disabled":       "skip",
				"comment_prefix":    "# ",
				"disable_signature": false,
				"disable":           false,
				"contents": `provider "aws" {
  region = "us-east-1"
}
`,
			},
		},
		generateOut,
	)
	remoteStateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["remote_state"].Value.(string)), &remoteStateOut))
	assert.Equal(
		t,
		map[string]interface{}{
			"backend":                         "local",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        map[string]interface{}{"path": "backend.tf", "if_exists": "overwrite_terragrunt"},
			"config":                          map[string]interface{}{"path": "foo.tfstate"},
		},
		remoteStateOut,
	)
	terraformOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["terraformtg"].Value.(string)), &terraformOut))
	assert.Equal(
		t,
		map[string]interface{}{
			"source":                   "./delorean",
			"include_in_copy":          []interface{}{"time_machine.*"},
			"copy_terraform_lock_file": true,
			"extra_arguments": map[string]interface{}{
				"var-files": map[string]interface{}{
					"name":               "var-files",
					"commands":           []interface{}{"apply", "plan"},
					"arguments":          nil,
					"required_var_files": []interface{}{"extra.tfvars"},
					"optional_var_files": []interface{}{"optional.tfvars"},
					"env_vars": map[string]interface{}{
						"TF_VAR_custom_var": "I'm set in extra_arguments env_vars",
					},
				},
			},
			"before_hook": map[string]interface{}{
				"before_hook_1": map[string]interface{}{
					"name":            "before_hook_1",
					"commands":        []interface{}{"apply", "plan"},
					"execute":         []interface{}{"touch", "before.out"},
					"working_dir":     nil,
					"run_on_error":    true,
					"suppress_stdout": nil,
				},
			},
			"after_hook": map[string]interface{}{
				"after_hook_1": map[string]interface{}{
					"name":            "after_hook_1",
					"commands":        []interface{}{"apply", "plan"},
					"execute":         []interface{}{"touch", "after.out"},
					"working_dir":     nil,
					"run_on_error":    true,
					"suppress_stdout": nil,
				},
			},
			"error_hook": map[string]interface{}{},
		},
		terraformOut,
	)
}

func logBufferContentsLineByLine(t *testing.T, out bytes.Buffer, label string) {
	t.Helper()
	t.Logf("[%s] Full contents of %s:", t.Name(), label)
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		t.Logf("[%s] %s", t.Name(), line)
	}
}

func TestTerragruntGenerateBlockSkipRemove(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "skip")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	assert.FileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockRemove(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	assert.NoFileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockRemoveTerragruntSuccess(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove_terragrunt")

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	assert.NoFileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockRemoveTerragruntFail(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureCodegenPath)
	generateTestCase := util.JoinPath(tmpEnvPath, testFixtureCodegenPath, "remove-file", "remove_terragrunt_error")

	_, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	require.Error(t, err)

	var generateFileRemoveError codegen.GenerateFileRemoveError
	ok := errors.As(err, &generateFileRemoveError)
	assert.True(t, ok)

	assert.FileExists(t, filepath.Join(generateTestCase, "backend.tf"))
}

func TestTerragruntGenerateBlockSkip(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "skip")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)
	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	assert.False(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntGenerateBlockOverwrite(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "overwrite")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntGenerateAttr(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-attr")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	text := "test-terragrunt-generate-attr-hello-world"

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir %s -var text=\"%s\"", generateTestCase, text))
	require.NoError(t, err)
	assert.Contains(t, stdout, text)
}

func TestTerragruntGenerateBlockOverwriteTerragruntSuccess(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "overwrite_terragrunt")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntGenerateBlockOverwriteTerragruntFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "overwrite_terragrunt_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)
	var generateFileExistsError codegen.GenerateFileExistsError
	ok := errors.As(err, &generateFileExistsError)
	assert.True(t, ok)
}

func TestTerragruntGenerateBlockNestedInherit(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "nested", "child_inherit")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it inherited the config
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, fileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTerragruntGenerateBlockNestedOverwrite(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "nested", "child_overwrite")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	// If the state file was written as bar.tfstate, that means it overwrite the parent config
	assert.False(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.True(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
	// Also check to make sure the child config generate block was included
	assert.True(t, fileIsInFolder(t, "random_file.txt", generateTestCase))
}

func TestTerragruntGenerateBlockDisableSignature(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "disable-signature")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)

	// Now check the outputs to make sure they are as expected
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase, &stdout, &stderr),
	)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	assert.Equal(t, "Hello, World!", outputs["text"].Value)
}

func TestTerragruntGenerateBlockSameNameFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "same_name_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)
	var parsedError config.DuplicatedGenerateBlocksError
	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTerragruntGenerateBlockSameNameIncludeFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "same_name_includes_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)
	var parsedError config.DuplicatedGenerateBlocksError
	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 1)
	assert.Contains(t, parsedError.BlockName, "backend")
}

func TestTerragruntGenerateBlockMultipleSameNameFail(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "same_name_pair_error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)
	var parsedError config.DuplicatedGenerateBlocksError
	ok := errors.As(err, &parsedError)
	assert.True(t, ok)
	assert.Len(t, parsedError.BlockName, 2)
	assert.Contains(t, parsedError.BlockName, "backend")
	assert.Contains(t, parsedError.BlockName, "backend2")
}

func TestTerragruntGenerateBlockDisable(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "disable")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, fileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTerragruntGenerateBlockEnable(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "generate-block", "enable")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, fileIsInFolder(t, "data.txt", generateTestCase))
}

func TestTerragruntRemoteStateCodegenGeneratesBackendBlock(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "base")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it wrote out the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
}

func TestTerragruntRemoteStateCodegenOverwrites(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "overwrite")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	// If the state file was written as foo.tfstate, that means it overwrote the local backend config.
	assert.True(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
	assert.False(t, fileIsInFolder(t, "bar.tfstate", generateTestCase))
}

func TestTerragruntRemoteStateCodegenErrorsIfExists(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "error")
	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase, &stdout, &stderr)
	require.Error(t, err)
	var generateFileExistsError codegen.GenerateFileExistsError
	ok := errors.As(err, &generateFileExistsError)
	assert.True(t, ok)
}

func TestTerragruntRemoteStateCodegenDoesNotGenerateWithSkip(t *testing.T) {
	t.Parallel()

	generateTestCase := filepath.Join(testFixtureCodegenPath, "remote-state", "skip")

	cleanupTerraformFolder(t, generateTestCase)
	cleanupTerragruntFolder(t, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+generateTestCase)
	assert.False(t, fileIsInFolder(t, "foo.tfstate", generateTestCase))
}

// This function cannot be parallelized as it changes the global version.Version
//
//nolint:paralleltest
func TestTerragruntValidateAllWithVersionChecks(t *testing.T) {
	tmpEnvPath := copyEnvironment(t, "fixtures/version-check")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntVersionCommand(t, "v0.23.21", "terragrunt validate-all --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnvPath, &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	require.NoError(t, err)
}

func TestTerragruntIncludeParentHclFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureIncludeParent)
	tmpEnvPath = path.Join(tmpEnvPath, testFixtureIncludeParent)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt run-all apply --terragrunt-modules-that-include parent.hcl --terragrunt-modules-that-include common.hcl --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnvPath, &stdout, &stderr)
	require.NoError(t, err)

	out := stdout.String()
	assert.Equal(t, 1, strings.Count(out, "parent_hcl_file"))
	assert.Equal(t, 1, strings.Count(out, "dependency_hcl"))
	assert.Equal(t, 1, strings.Count(out, "common_hcl"))
}

// The tests here cannot be parallelized.
// This is due to a race condition brought about by overriding `version.Version` in
// runTerragruntVersionCommand
//
//nolint:paralleltest,funlen
func TestTerragruntVersionConstraints(t *testing.T) {
	tc := []struct {
		name                 string
		terragruntVersion    string
		terragruntConstraint string
		shouldSucceed        bool
	}{
		{
			"version meets constraint equal",
			"v0.23.18",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constraint greater patch",
			"v0.23.19",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version meets constraint greater major",
			"v1.0.0",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version fails constraint less patch",
			"v0.23.17",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			false,
		},
		{
			"version fails constraint less major",
			"v0.22.18",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			false,
		},
		{
			"version meets constraint pre-release",
			"v0.23.18-alpha2024091301",
			"terragrunt_version_constraint = \">= v0.23.18\"",
			true,
		},
		{
			"version fails constraint pre-release",
			"v0.23.18-alpha2024091301",
			"terragrunt_version_constraint = \"< v0.23.18\"",
			false,
		},
	}

	for _, tt := range tc { //nolint:paralleltest
		t.Run(tt.name, func(t *testing.T) {
			tmpEnvPath := copyEnvironment(t, testFixtureReadConfig)
			rootPath := filepath.Join(tmpEnvPath, testFixtureReadConfig, "with_constraints")

			tmpTerragruntConfigPath := createTmpTerragruntConfigContent(t, tt.terragruntConstraint, config.DefaultTerragruntConfigPath)

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			err := runTerragruntVersionCommand(
				t,
				tt.terragruntVersion,
				fmt.Sprintf(
					"terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s",
					tmpTerragruntConfigPath,
					rootPath,
				),
				&stdout,
				&stderr,
			)

			logBufferContentsLineByLine(t, stdout, "stdout")
			logBufferContentsLineByLine(t, stderr, "stderr")

			if tt.shouldSucceed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestReadTerragruntAuthProviderCmd(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := copyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "multiple-apps")
	appPath := util.JoinPath(rootPath, "app1")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	runTerragrunt(t, fmt.Sprintf(`terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s`, rootPath, mockAuthCmd))

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt output -json --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", appPath, mockAuthCmd))
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "app1-bar", outputs["foo-app1"].Value)
	assert.Equal(t, "app2-bar", outputs["foo-app2"].Value)
	assert.Equal(t, "app3-bar", outputs["foo-app3"].Value)
}

func TestIamRolesLoadingFromDifferentModules(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureIamRolesMultipleModules)

	// Execution outputs to be verified
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	// Invoke terragrunt and verify used IAM roles for each dependency
	err := runTerragruntCommand(t, "terragrunt init --terragrunt-log-level debug --terragrunt-debugreset --terragrunt-working-dir "+testFixtureIamRolesMultipleModules, &stdout, &stderr)

	// Taking all outputs in one string
	output := fmt.Sprintf("%v %v %v", stderr.String(), stdout.String(), err.Error())

	component1 := ""
	component2 := ""

	// scan each output line and get lines for component1 and component2
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Assuming IAM role arn:aws:iam::component1:role/terragrunt") {
			component1 = line
			continue
		}
		if strings.Contains(line, "Assuming IAM role arn:aws:iam::component2:role/terragrunt") {
			component2 = line
			continue
		}
	}
	assert.NotEmptyf(t, component1, "Missing role for component 1")
	assert.NotEmptyf(t, component2, "Missing role for component 2")
}

// This function cannot be parallelized as it changes the global version.Version
//
//nolint:paralleltest
func TestTerragruntVersionConstraintsPartialParse(t *testing.T) {
	fixturePath := "fixtures/partial-parse/terragrunt-version-constraint"
	cleanupTerragruntFolder(t, fixturePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntVersionCommand(t, "0.21.23", "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+fixturePath, &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")

	require.Error(t, err)

	var invalidVersionError terraform.InvalidTerragruntVersion
	ok := errors.As(err, &invalidVersionError)
	assert.True(t, ok)
}

func TestLogFailingDependencies(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	path := filepath.Join(testFixtureBrokenDependency, "app")

	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-log-level debug", path), &stdout, &stderr)
	require.Error(t, err)

	testdataDir, err := filepath.Abs(testFixtureBrokenDependency)
	require.NoError(t, err)

	output := stderr.String()
	assert.Contains(t, output, fmt.Sprintf("%s invocation failed in %s", wrappedBinary(), getPathRelativeTo(t, testdataDir, path)))
}

func TestDependenciesOptimisation(t *testing.T) {
	tmpEnvPath := copyEnvironment(t, testFixtureDependenciesOptimisation)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureDependenciesOptimisation)

	_, _, err := runTerragruntCommandWithOutput(t, "terragrunt run-all apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	config.ClearOutputCache()

	moduleC := util.JoinPath(tmpEnvPath, testFixtureDependenciesOptimisation, "module-c")
	t.Setenv("TERRAGRUNT_STRICT_CONTROL", "skip-dependencies-inputs")
	_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir "+moduleC)
	require.NoError(t, err)

	// checking that dependencies optimisation is working and outputs from module-a are not retrieved
	assert.NotContains(t, stderr, "Retrieved output from ../module-a/terragrunt.hcl")
}

func cleanupTerraformFolder(t *testing.T, templatesPath string) {
	t.Helper()

	removeFile(t, util.JoinPath(templatesPath, terraformState))
	removeFile(t, util.JoinPath(templatesPath, terraformStateBackup))
	removeFile(t, util.JoinPath(templatesPath, terragruntDebugFile))
	removeFolder(t, util.JoinPath(templatesPath, terraformFolder))
}

func cleanupTerragruntFolder(t *testing.T, templatesPath string) {
	t.Helper()

	removeFolder(t, util.JoinPath(templatesPath, terragruntCache))
}

func removeFile(t *testing.T, path string) {
	t.Helper()

	if util.FileExists(path) {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func removeFolder(t *testing.T, path string) {
	t.Helper()

	if util.FileExists(path) {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func runTerragruntCommand(t *testing.T, command string, writer io.Writer, errwriter io.Writer) error {
	t.Helper()

	args := splitCommand(command)

	if !strings.Contains(command, "-terragrunt-log-format") && !strings.Contains(command, "-terragrunt-log-custom-format") {
		args = append(args, "--terragrunt-log-format=key-value")
	}

	t.Log(args)

	opts := options.NewTerragruntOptionsWithWriters(writer, errwriter)
	app := cli.NewApp(opts)
	return app.Run(args)
}

func runTerragruntVersionCommand(t *testing.T, ver string, command string, writer io.Writer, errwriter io.Writer) error {
	t.Helper()

	version.Version = ver
	return runTerragruntCommand(t, command, writer, errwriter)
}

func runTerragrunt(t *testing.T, command string) {
	t.Helper()

	runTerragruntRedirectOutput(t, command, os.Stdout, os.Stderr)
}

func runTerragruntCommandWithOutput(t *testing.T, command string) (string, string, error) {
	t.Helper()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, command, &stdout, &stderr)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")
	return stdout.String(), stderr.String(), err
}

func runTerragruntRedirectOutput(t *testing.T, command string, writer io.Writer, errwriter io.Writer) {
	t.Helper()

	if err := runTerragruntCommand(t, command, writer, errwriter); err != nil {
		stdout := "(see log output above)"
		if stdoutAsBuffer, stdoutIsBuffer := writer.(*bytes.Buffer); stdoutIsBuffer {
			stdout = stdoutAsBuffer.String()
		}

		stderr := "(see log output above)"
		if stderrAsBuffer, stderrIsBuffer := errwriter.(*bytes.Buffer); stderrIsBuffer {
			stderr = stderrAsBuffer.String()
		}

		t.Fatalf("Failed to run Terragrunt command '%s' due to error: %s\n\nStdout: %s\n\nStderr: %s", command, errors.ErrorStack(err), stdout, stderr)
	}
}

func copyEnvironment(t *testing.T, environmentPath string, includeInCopy ...string) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(createLogger(), environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", includeInCopy))

	return tmpDir
}

func createTmpTerragruntConfig(t *testing.T, templatesPath string, s3BucketName string, lockTableName string, configFileName string) string {
	t.Helper()

	tmpFolder, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	copyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "not-used")

	return tmpTerragruntConfigFile
}

func createTmpTerragruntConfigContent(t *testing.T, contents string, configFileName string) string {
	t.Helper()

	tmpFolder, err := os.MkdirTemp("", "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp folder due to error: %v", err)
	}

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)

	if err := os.WriteFile(tmpTerragruntConfigFile, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", tmpTerragruntConfigFile, err)
	}

	return tmpTerragruntConfigFile
}

func copyTerragruntConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, s3BucketName string, lockTableName string, region string) {
	t.Helper()

	copyAndFillMapPlaceholders(t, configSrcPath, configDestPath, map[string]string{
		"__FILL_IN_BUCKET_NAME__":      s3BucketName,
		"__FILL_IN_LOCK_TABLE_NAME__":  lockTableName,
		"__FILL_IN_REGION__":           region,
		"__FILL_IN_LOGS_BUCKET_NAME__": s3BucketName + "-tf-state-logs",
	})
}

func copyAndFillMapPlaceholders(t *testing.T, srcPath string, destPath string, placeholders map[string]string) {
	t.Helper()

	contents, err := util.ReadFileAsString(srcPath)
	if err != nil {
		t.Fatalf("Error reading file at %s: %v", srcPath, err)
	}

	// iterate over placeholders and replace placeholders
	for k, v := range placeholders {
		contents = strings.ReplaceAll(contents, k, v)
	}
	if err := os.WriteFile(destPath, []byte(contents), 0444); err != nil {
		t.Fatalf("Error writing temp file to %s: %v", destPath, err)
	}
}

// Returns a unique (ish) id we can attach to resources and tfstate files so they don't conflict with each other
// Uses base 62 to generate a 6 character string that's unlikely to collide with the handful of tests we run in
// parallel. Based on code here: http://stackoverflow.com/a/9543797/483528
func uniqueID() string {
	const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const uniqueIDLength = 6 // Should be good for 62^6 = 56+ billion combinations

	var out bytes.Buffer

	for i := 0; i < uniqueIDLength; i++ {
		out.WriteByte(base62Chars[rand.Intn(len(base62Chars))])
	}

	return out.String()
}

// Delete the specified S3 bucket to clean up after a test
func deleteS3Bucket(t *testing.T, awsRegion string, bucketName string, opts ...options.TerragruntOptionsFunc) {
	t.Helper()

	require.NoError(t, deleteS3BucketE(t, awsRegion, bucketName, opts...))
}
func deleteS3BucketE(t *testing.T, awsRegion string, bucketName string, opts ...options.TerragruntOptionsFunc) error {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("integration_test", opts...)
	if err != nil {
		t.Logf("Error creating mockOptions: %v", err)
		return err
	}

	sessionConfig := &awshelper.AwsSessionConfig{
		Region: awsRegion,
	}

	s3Client, err := remote.CreateS3Client(sessionConfig, mockOptions)
	if err != nil {
		t.Logf("Error creating S3 client: %v", err)
		return err
	}

	t.Logf("Deleting test s3 bucket %s", bucketName)

	out, err := s3Client.ListObjectVersions(&s3.ListObjectVersionsInput{Bucket: aws.String(bucketName)})
	if err != nil {
		t.Logf("Failed to list object versions in s3 bucket %s: %v", bucketName, err)
		return err
	}

	objectIdentifiers := []*s3.ObjectIdentifier{}
	for _, version := range out.Versions {
		objectIdentifiers = append(objectIdentifiers, &s3.ObjectIdentifier{
			Key:       version.Key,
			VersionId: version.VersionId,
		})
	}

	if len(objectIdentifiers) > 0 {
		deleteInput := &s3.DeleteObjectsInput{
			Bucket: aws.String(bucketName),
			Delete: &s3.Delete{Objects: objectIdentifiers},
		}
		if _, err := s3Client.DeleteObjects(deleteInput); err != nil {
			t.Logf("Error deleting all versions of all objects in bucket %s: %v", bucketName, err)
			return err
		}
	}

	if _, err := s3Client.DeleteBucket(&s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		t.Logf("Failed to delete S3 bucket %s: %v", bucketName, err)
		return err
	}
	return nil
}

func fileIsInFolder(t *testing.T, name string, path string) bool {
	t.Helper()

	found := false
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		require.NoError(t, err)
		if filepath.Base(path) == name {
			found = true
		}
		return nil
	})
	require.NoError(t, err)
	return found
}

func runValidateAllWithIncludeAndGetIncludedModules(t *testing.T, rootModulePath string, includeModulePaths []string, strictInclude bool) []string {
	t.Helper()

	cmdParts := []string{
		"terragrunt", "run-all", "validate",
		"--terragrunt-non-interactive",
		"--terragrunt-log-level", "debug",
		"--terragrunt-working-dir", rootModulePath,
	}

	for _, module := range includeModulePaths {
		cmdParts = append(cmdParts, "--terragrunt-include-dir", module)
	}

	if strictInclude {
		cmdParts = append(cmdParts, "--terragrunt-strict-include")
	}

	cmd := strings.Join(cmdParts, " ")

	validateAllStdout := bytes.Buffer{}
	validateAllStderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		cmd,
		&validateAllStdout,
		&validateAllStderr,
	)
	logBufferContentsLineByLine(t, validateAllStdout, "validate-all stdout")
	logBufferContentsLineByLine(t, validateAllStderr, "validate-all stderr")
	require.NoError(t, err)

	require.NoError(t, err)

	includedModulesRegexp, err := regexp.Compile(`=> Module (.+) \(excluded: (true|false)`)
	require.NoError(t, err)

	matches := includedModulesRegexp.FindAllStringSubmatch(validateAllStderr.String(), -1)
	includedModules := []string{}
	for _, match := range matches {
		if match[2] == "false" {
			includedModules = append(includedModules, getPathRelativeTo(t, match[1], rootModulePath))
		}
	}
	sort.Strings(includedModules)
	return includedModules
}

func TestShowErrorWhenRunAllInvokedWithoutArguments(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureStack)
	appPath := util.JoinPath(tmpEnvPath, testFixtureStack)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, "terragrunt run-all --terragrunt-non-interactive --terragrunt-working-dir "+appPath, &stdout, &stderr)
	require.Error(t, err)
	var missingCommandError runall.MissingCommand
	ok := errors.As(err, &missingCommandError)
	assert.True(t, ok)
}

func TestNoMultipleInitsWithoutSourceChange(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureDownload)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureStdout)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// no initialization expected for second plan run
	// https://github.com/gruntwork-io/terragrunt/issues/1921
	assert.Equal(t, 0, strings.Count(stdout.String(), "has been successfully initialized!"))
}

func TestAutoInitWhenSourceIsChanged(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureDownload)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureAutoInit)

	terragruntHcl := util.JoinPath(testPath, "terragrunt.hcl")
	contents, err := util.ReadFileAsString(terragruntHcl)
	if err != nil {
		require.NoError(t, err)
	}
	updatedHcl := strings.ReplaceAll(contents, "__TAG_VALUE__", "v0.35.1")
	require.NoError(t, os.WriteFile(terragruntHcl, []byte(updatedHcl), 0444))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	updatedHcl = strings.ReplaceAll(contents, "__TAG_VALUE__", "v0.35.2")
	require.NoError(t, os.WriteFile(terragruntHcl, []byte(updatedHcl), 0444))

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	err = runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// auto initialization when source is changed
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))
}

func TestNoColor(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureNoColor)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNoColor)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt plan -no-color --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	// providers initialization during first plan
	assert.Equal(t, 1, strings.Count(stdout.String(), "has been successfully initialized!"))

	assert.NotContains(t, stdout.String(), "\x1b")
}

func TestOutputModuleGroups(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureOutputModuleGroups)
	cleanupTerraformFolder(t, tmpEnvPath)
	environmentPath := fmt.Sprintf("%s/%s", tmpEnvPath, testFixtureOutputModuleGroups)

	expectedApplyOutput := fmt.Sprintf(`
	{
	  "Group 1": [
		"%[1]s/root/vpc"
	  ],
	  "Group 2": [
		"%[1]s/root/mysql",
		"%[1]s/root/redis"
	  ],
	  "Group 3": [
		"%[1]s/root/backend-app"
	  ],
	  "Group 4": [
		"%[1]s/root/frontend-app"
	  ]
	}`, environmentPath)

	expectedDestroyOutput := fmt.Sprintf(`
	{
	  "Group 1": [
	    "%[1]s/root/frontend-app"
	  ],
	  "Group 2": [
		"%[1]s/root/backend-app"
	  ],
	  "Group 3": [
		"%[1]s/root/mysql",
		"%[1]s/root/redis"
	  ],
	  "Group 4": [
		"%[1]s/root/vpc"
	  ]
	}`, environmentPath)

	tests := map[string]struct {
		subCommand     string
		expectedOutput string
	}{
		"output-module-groups with no subcommand": {
			subCommand:     "",
			expectedOutput: expectedApplyOutput,
		},
		"output-module-groups with apply subcommand": {
			subCommand:     "apply",
			expectedOutput: expectedApplyOutput,
		},
		"output-module-groups with destroy subcommand": {
			subCommand:     "destroy",
			expectedOutput: expectedDestroyOutput,
		},
	}

	for name, tt := range tests {
		tt := tt

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)
			runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output-module-groups --terragrunt-working-dir %s %s", environmentPath, tt.subCommand), &stdout, &stderr)
			output := strings.ReplaceAll(stdout.String(), " ", "")
			expectedOutput := strings.ReplaceAll(strings.ReplaceAll(tt.expectedOutput, "\t", ""), " ", "")
			assert.True(t, strings.Contains(strings.TrimSpace(output), strings.TrimSpace(expectedOutput)))
		})
	}
}

func TestTerragruntValidateModulePrefix(t *testing.T) {
	t.Parallel()

	fixturePath := testFixtureIncludeParent
	cleanupTerraformFolder(t, fixturePath)
	tmpEnvPath := copyEnvironment(t, fixturePath)
	rootPath := util.JoinPath(tmpEnvPath, fixturePath)

	runTerragrunt(t, "terragrunt run-all validate --terragrunt-forward-tf-stdout --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestInitFailureModulePrefix(t *testing.T) {
	t.Parallel()

	initTestCase := testFixtureInitError

	cleanupTerraformFolder(t, initTestCase)
	cleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	require.Error(
		t,
		runTerragruntCommand(t, "terragrunt init -no-color --terragrunt-non-interactive --terragrunt-working-dir "+initTestCase, &stdout, &stderr),
	)
	logBufferContentsLineByLine(t, stderr, "init")
	assert.Contains(t, stderr.String(), "fixtures/init-error")
}

func TestDependencyOutputModulePrefix(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureGetOutput)
	tmpEnvPath := copyEnvironment(t, testFixtureGetOutput)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureGetOutput, "integration")

	runTerragrunt(t, "terragrunt apply-all --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)

	// verify expected output 42
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	app3Path := util.JoinPath(rootPath, "app3")
	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+app3Path, &stdout, &stderr),
	)
	// validate that output is valid json
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	assert.Equal(t, 42, int(outputs["z"].Value.(float64)))
}

func TestErrorExplaining(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureInitError)
	initTestCase := util.JoinPath(tmpEnvPath, testFixtureInitError)

	cleanupTerraformFolder(t, initTestCase)
	cleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt init -no-color --terragrunt-forward-tf-stdout --terragrunt-non-interactive --terragrunt-working-dir "+initTestCase, &stdout, &stderr)
	require.Error(t, err)

	explanation := shell.ExplainError(err)
	assert.Contains(t, explanation, "Check your credentials and permissions")
}

func TestExplainingMissingCredentials(t *testing.T) {
	// no parallel because we need to set env vars
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/not-existing-creds-46521694")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	tmpEnvPath := copyEnvironment(t, testFixtureInitError)
	initTestCase := util.JoinPath(tmpEnvPath, testFixtureInitError)

	cleanupTerraformFolder(t, initTestCase)
	cleanupTerragruntFolder(t, initTestCase)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt init -no-color --terragrunt-forward-tf-stdout --terragrunt-non-interactive --terragrunt-working-dir "+initTestCase, &stdout, &stderr)
	explanation := shell.ExplainError(err)
	assert.Contains(t, explanation, "Missing AWS credentials")
}

func TestModulePathInPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureModulePathError)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureModulePathError, "app")

	stdout, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt plan -no-color --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.Error(t, err)
	output := stdout + "\n" + stderr + "\n" + err.Error() + "\n"

	assert.Contains(t, output, "error occurred")
}

func TestModulePathInRunAllPlanErrorMessage(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureModulePathError)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureModulePathError)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt run-all plan -no-color --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr)
	require.Error(t, err)
	output := fmt.Sprintf("%s\n%s\n%v\n", stdout.String(), stderr.String(), err.Error())
	assert.Contains(t, output, "finished with an error")
	assert.Contains(t, output, "Module ./d1", output)
}

func TestHclFmtDiff(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureHclfmtDiff)
	tmpEnvPath := copyEnvironment(t, testFixtureHclfmtDiff)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHclfmtDiff)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt hclfmt --terragrunt-diff --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	output := stdout.String()

	expectedDiff, err := os.ReadFile(util.JoinPath(rootPath, "expected.diff"))
	require.NoError(t, err)

	logBufferContentsLineByLine(t, stdout, "output")
	assert.Contains(t, output, string(expectedDiff))
}

func TestDownloadSourceWithRef(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureRefSource)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureRefSource)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
}

func TestSourceMapWithSlashInRef(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureSourceMapSlashes)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSourceMapSlashes)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt plan --terragrunt-non-interactive --terragrunt-source-map git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=git::git@github.com:gruntwork-io/terragrunt.git?ref=fixture/test-fixtures --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
}

func TestInitSkipCache(t *testing.T) {
	t.Parallel()

	cleanupTerraformFolder(t, testFixtureInitCache)
	tmpEnvPath := copyEnvironment(t, testFixtureInitCache)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInitCache, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	// verify that init was invoked
	assert.Contains(t, stdout.String(), "has been successfully initialized!")
	assert.Contains(t, stderr.String(), "Running command: "+wrappedBinary()+" init")

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	// verify that init wasn't invoked second time since cache directories are ignored
	assert.NotContains(t, stdout.String(), "has been successfully initialized!")
	assert.NotContains(t, stderr.String(), "Running command: "+wrappedBinary()+" init")

	// verify that after adding new file, init is executed
	tfFile := util.JoinPath(tmpEnvPath, testFixtureInitCache, "app", "project.tf")
	if err := os.WriteFile(tfFile, []byte(""), 0644); err != nil {
		t.Fatalf("Error writing new Terraform file to %s: %v", tfFile, err)
	}

	stdout = bytes.Buffer{}
	stderr = bytes.Buffer{}

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt plan --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)

	// verify that init was invoked
	assert.Contains(t, stdout.String(), "has been successfully initialized!")
	assert.Contains(t, stderr.String(), "Running command: "+wrappedBinary()+" init")
}

func TestTerragruntFailIfBucketCreationIsrequired(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, testFixturePath)
	cleanupTerraformFolder(t, rootPath)

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(uniqueID())
	lockTableName := "terragrunt-test-locks-" + strings.ToLower(uniqueID())

	tmpTerragruntConfigPath := createTmpTerragruntConfig(t, rootPath, s3BucketName, lockTableName, config.DefaultTerragruntConfigPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt apply --terragrunt-fail-on-state-bucket-creation --terragrunt-non-interactive --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, rootPath), &stdout, &stderr)
	require.Error(t, err)
}

func TestTerragruntPassNullValues(t *testing.T) {
	t.Parallel()

	generateTestCase := testFixtureNullValue
	tmpEnv := copyEnvironment(t, generateTestCase)
	cleanupTerraformFolder(t, tmpEnv)
	cleanupTerragruntFolder(t, tmpEnv)
	tmpEnv = util.JoinPath(tmpEnv, generateTestCase)

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnv)

	// Now check the outputs to make sure they are as expected
	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnv)

	require.NoError(t, err)
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// check that the null values are passed correctly
	assert.Nil(t, outputs["output1"].Value)
	assert.Equal(t, "variable 2", outputs["output2"].Value)

	// check that file with null values is removed
	cachePath := filepath.Join(tmpEnv, terragruntCache)
	foundNullValuesFile := false
	err = filepath.Walk(cachePath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if strings.HasPrefix(path, terraform.NullTFVarsFile) {
				foundNullValuesFile = true
			}
			return nil
		})
	assert.Falsef(t, foundNullValuesFile, "Found %s file in cache directory", terraform.NullTFVarsFile)
	require.NoError(t, err)
}

func TestTerragruntHandleLegacyNullValues(t *testing.T) {
	// no parallel since we need to set env vars
	t.Setenv("TERRAGRUNT_TEMP_QUOTE_NULL", "1")
	generateTestCase := testFixtureNullValue
	tmpEnv := copyEnvironment(t, generateTestCase)
	cleanupTerraformFolder(t, tmpEnv)
	cleanupTerragruntFolder(t, tmpEnv)
	tmpEnv = util.JoinPath(tmpEnv, generateTestCase)

	_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnv)
	require.NoError(t, err)
	assert.Contains(t, stderr, "Input `var1` has value `null`. Quoting due to TERRAGRUNT_TEMP_QUOTE_NULL")

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-working-dir "+tmpEnv)
	require.NoError(t, err)
	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	// check that null value is passed as "null"
	assert.Equal(t, "null", outputs["output1"].Value)
	assert.Equal(t, "variable 2", outputs["output2"].Value)
}

func TestTerragruntNoWarningLocalPath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureDisabledPath)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDisabledPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt apply --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "No double-slash (//) found in source URL")
}

func TestTerragruntNoWarningRemotePath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureNoSubmodules)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureNoSubmodules)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt init --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "No double-slash (//) found in source URL")
}

func TestTerragruntDisabledDependency(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureDisabledModule)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "app")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt run-all plan --terragrunt-non-interactive  --terragrunt-log-level debug --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := stderr.String()

	// check that only enabled dependencies are evaluated

	for _, path := range []string{
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "app"),
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "m1"),
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "m3"),
	} {
		relPath, err := filepath.Rel(testPath, path)
		require.NoError(t, err)
		assert.Contains(t, output, relPath, output)
	}

	for _, path := range []string{
		util.JoinPath(tmpEnvPath, testFixtureDisabledModule, "m2"),
	} {
		relPath, err := filepath.Rel(testPath, path)
		require.NoError(t, err)
		assert.NotContains(t, output, relPath, output)
	}
}

func TestTerragruntHandleEmptyStateFile(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureEmptyState)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureEmptyState)

	// create empty terraform.tfstate file
	file, err := os.Create(util.JoinPath(testPath, terraformState))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	runTerragrunt(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+testPath)
}

func TestTerragruntInvokeTerraformTests(t *testing.T) {
	t.Parallel()
	if isTerraform() {
		t.Skip("Not compatible with Terraform 1.5.x")
		return
	}

	tmpEnvPath := copyEnvironment(t, testFixtureTfTest)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureTfTest)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt test --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "1 passed, 0 failed")
}

func TestTerragruntCommandsThatNeedInput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testCommandsThatNeedInput)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testCommandsThatNeedInput)

	stdout, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Apply complete")
}

func TestTerragruntSkipDependenciesWithSkipFlag(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureSkipDependencies)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureSkipDependencies)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt run-all apply --no-color --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.NoError(t, err)

	output := fmt.Sprintf("%s %s", stderr.String(), stdout.String())

	assert.NotContains(t, output, "Error reading partial config for dependency")
	assert.NotContains(t, output, "Call to function \"find_in_parent_folders\" failed")
	assert.NotContains(t, output, "ParentFileNotFoundError")

	assert.Contains(t, output, "first/terragrunt.hcl due to skip = true")
	assert.Contains(t, output, "second/terragrunt.hcl due to skip = true")
	// check that no test_file.txt was created in module directory
	_, err = os.Stat(util.JoinPath(tmpEnvPath, testFixtureSkipDependencies, "first", "test_file.txt"))
	require.Error(t, err)
	_, err = os.Stat(util.JoinPath(tmpEnvPath, testFixtureSkipDependencies, "second", "test_file.txt"))
	require.Error(t, err)
}

func TestTerragruntInfoError(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureInfoError)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureInfoError, "module-b")

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := runTerragruntCommand(t, "terragrunt terragrunt-info --terragrunt-non-interactive --terragrunt-working-dir "+testPath, &stdout, &stderr)
	require.Error(t, err)

	// parse stdout json as TerragruntInfoGroup
	var output terragruntinfo.TerragruntInfoGroup
	err = json.Unmarshal(stdout.Bytes(), &output)
	require.NoError(t, err)
}

func TestStorePlanFilesRunAllPlanApply(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	tmpEnvPath := copyEnvironment(t, testFixtureOutDir)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// run plan with output directory
	_, output, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	assert.Contains(t, output, "Using output file "+getPathRelativeTo(t, tmpDir, testPath))

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err := findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)
}

func TestStorePlanFilesRunAllPlanApplyRelativePath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureOutDir)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// run plan with output directory
	_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, "test"))
	require.NoError(t, err)

	outDir := util.JoinPath(testPath, "test")

	// verify that tfplan files are created in the tmpDir, 2 files
	list, err := findFilesWithExtension(outDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir test", testPath))
	require.NoError(t, err)
}

func TestStorePlanFilesJsonRelativePath(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureOutDir)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// run plan with output directory
	_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir test --terragrunt-json-out-dir json", testPath))
	require.NoError(t, err)

	// verify that tfplan files are created in the tmpDir, 2 files
	outDir := util.JoinPath(testPath, "test")
	list, err := findFilesWithExtension(outDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// verify that json files are create
	jsonDir := util.JoinPath(testPath, "json")
	listJSON, err := findFilesWithExtension(jsonDir, ".json")
	require.NoError(t, err)
	assert.Len(t, listJSON, 2)
}

func TestPlanJsonFilesRunAll(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	_, _, _, err := testRunAllPlan(t, "--terragrunt-json-out-dir "+tmpDir)
	require.NoError(t, err)

	// verify that was generated json files with plan data
	list, err := findFilesWithExtension(tmpDir, ".json")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.json", filepath.Base(file))
		// verify that file is not empty
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
		// check that produced json is valid and can be unmarshalled
		var plan map[string]interface{}
		err = json.Unmarshal(content, &plan)
		require.NoError(t, err)
		// check that plan is not empty
		assert.NotEmpty(t, plan)
	}

}

func TestPlanJsonPlanBinaryRunAll(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	tmpEnvPath := copyEnvironment(t, testFixtureOutDir)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// run plan with output directory
	_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-json-out-dir %s --terragrunt-out-dir %s", testPath, tmpDir, tmpDir))
	require.NoError(t, err)

	// verify that was generated json files with plan data
	list, err := findFilesWithExtension(tmpDir, ".json")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.json", filepath.Base(file))
		// verify that file is not empty
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.NotEmpty(t, content)
	}

	// verify that was generated binary plan files
	list, err = findFilesWithExtension(tmpDir, ".tfplan")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	for _, file := range list {
		assert.Equal(t, "tfplan.tfplan", filepath.Base(file))
	}

}

func TestTerragruntRunAllPlanAndShow(t *testing.T) {
	t.Parallel()

	// create temporary directory for plan files
	tmpDir := t.TempDir()
	tmpEnvPath := copyEnvironment(t, testFixtureOutDir)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// run plan and apply
	_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	// run new plan and show
	_, _, err = runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-out-dir %s", testPath, tmpDir))
	require.NoError(t, err)

	stdout, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run-all show --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-forward-tf-stdout --terragrunt-working-dir %s --terragrunt-out-dir %s -no-color", testPath, tmpDir))
	require.NoError(t, err)

	// Verify that output contains the plan and not just the actual state output
	assert.Contains(t, stdout, "No changes. Your infrastructure matches the configuration.")
}

func createTmpTerragruntConfigWithParentAndChild(t *testing.T, parentPath string, childRelPath string, s3BucketName string, parentConfigFileName string, childConfigFileName string) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "terragrunt-parent-child-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	childDestPath := util.JoinPath(tmpDir, childRelPath)

	if err := os.MkdirAll(childDestPath, 0777); err != nil {
		t.Fatalf("Failed to create temp dir %s due to error %v", childDestPath, err)
	}

	parentTerragruntSrcPath := util.JoinPath(parentPath, parentConfigFileName)
	parentTerragruntDestPath := util.JoinPath(tmpDir, parentConfigFileName)
	copyTerragruntConfigAndFillPlaceholders(t, parentTerragruntSrcPath, parentTerragruntDestPath, s3BucketName, "not-used", "not-used")

	childTerragruntSrcPath := util.JoinPath(util.JoinPath(parentPath, childRelPath), childConfigFileName)
	childTerragruntDestPath := util.JoinPath(childDestPath, childConfigFileName)
	copyTerragruntConfigAndFillPlaceholders(t, childTerragruntSrcPath, childTerragruntDestPath, s3BucketName, "not-used", "not-used")

	return childTerragruntDestPath
}

func TestLogFormatJSONOutput(t *testing.T) {
	t.Parallel()

	for _, flag := range []string{"--terragrunt-log-format=json", "--terragrunt-json-log"} {
		t.Run("testCase-flag-"+flag, func(t *testing.T) {
			t.Parallel()

			tmpEnvPath := copyEnvironment(t, testFixtureNotExistingSource)
			cleanupTerraformFolder(t, tmpEnvPath)
			testPath := util.JoinPath(tmpEnvPath, testFixtureNotExistingSource)

			_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt apply "+flag+" --terragrunt-non-interactive --terragrunt-working-dir "+testPath)
			require.Error(t, err)

			// for windows OS
			output := bytes.ReplaceAll([]byte(stderr), []byte("\r\n"), []byte("\n"))

			multipeJSONs := bytes.Split(output, []byte("\n"))

			var msgs = make([]string, 0, len(multipeJSONs))

			for _, jsonBytes := range multipeJSONs {
				if len(jsonBytes) == 0 {
					continue
				}

				var output map[string]interface{}

				err = json.Unmarshal(jsonBytes, &output)
				require.NoError(t, err)

				msg, ok := output["msg"].(string)
				assert.True(t, ok)
				msgs = append(msgs, msg)
			}

			assert.Contains(t, strings.Join(msgs, ""), "Downloading Terraform configurations from git::https://github.com/gruntwork-io/terragrunt.git?ref=v0.9.9")
		})
	}
}

func TestTerragruntTerraformOutputJson(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureInitError)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureInitError)

	_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt apply --no-color --terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-forward-tf-stdout --terragrunt-non-interactive --terragrunt-working-dir "+testPath)
	require.Error(t, err)

	// Sometimes, this is the error returned by AWS.
	if !strings.Contains(stderr, "Error: Failed to get existing workspaces: operation error S3: ListObjectsV2, https response error StatusCode: 301") {
		assert.Contains(t, stderr, `"msg":"Initializing the backend..."`)
	}

	// check if output can be extracted in json
	jsonStrings := strings.Split(stderr, "\n")
	for _, jsonString := range jsonStrings {
		if len(jsonString) == 0 {
			continue
		}
		var output map[string]interface{}
		err = json.Unmarshal([]byte(jsonString), &output)
		require.NoErrorf(t, err, "Failed to parse json %s", jsonString)
		assert.NotNil(t, output["level"])
		assert.NotNil(t, output["time"])
	}
}

func TestTerragruntOutputFromDependencyLogsJson(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		arg string
	}{
		{"--terragrunt-json-log"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json"},
		{"--terragrunt-forward-tf-stdout"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-forward-tf-stdout"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run("terragrunt output with "+testCase.arg, func(t *testing.T) {
			t.Parallel()
			tmpEnvPath := copyEnvironment(t, testFixtureDependencyOutput)
			rootTerragruntPath := util.JoinPath(tmpEnvPath, testFixtureDependencyOutput)
			// apply dependency first
			dependencyTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "dependency")
			_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s ", dependencyTerragruntConfigPath))
			require.NoError(t, err)
			appTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "app")
			stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s %s", appTerragruntConfigPath, testCase.arg))
			require.NoError(t, err)
			output := fmt.Sprintf("%s %s", stderr, stdout)
			assert.NotContains(t, output, "invalid character")
		})

	}
}

func TestTerragruntJsonPlanJsonOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		arg string
	}{
		{"--terragrunt-json-log"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json"},
		{"--terragrunt-forward-tf-stdout"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-forward-tf-stdout"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run("terragrunt with "+testCase.arg, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			_, _, _, err := testRunAllPlan(t, fmt.Sprintf("--terragrunt-json-out-dir %s %s", tmpDir, testCase.arg))
			require.NoError(t, err)
			list, err := findFilesWithExtension(tmpDir, ".json")
			require.NoError(t, err)
			assert.Len(t, list, 2)
			for _, file := range list {
				assert.Equal(t, "tfplan.json", filepath.Base(file))
				// verify that file is not empty
				content, err := os.ReadFile(file)
				require.NoError(t, err)
				assert.NotEmpty(t, content)
				// check that produced json is valid and can be unmarshalled
				var plan map[string]interface{}
				err = json.Unmarshal(content, &plan)
				require.NoError(t, err)
				// check that plan is not empty
				assert.NotEmpty(t, plan)
			}
		})

	}
}

func TestErrorMessageIncludeInOutput(t *testing.T) {
	t.Parallel()

	tmpEnvPath := copyEnvironment(t, testFixtureErrorPrint)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureErrorPrint)

	_, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply  --terragrunt-non-interactive --terragrunt-working-dir "+testPath+" --terragrunt-tfpath "+testPath+"/custom-tf-script.sh --terragrunt-log-level debug")
	require.Error(t, err)

	assert.Contains(t, err.Error(), "Custom error from script")
}
