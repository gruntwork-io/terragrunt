package scaffold_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	boilerplateoptions "github.com/gruntwork-io/boilerplate/options"
	"github.com/gruntwork-io/boilerplate/templates"
	"github.com/gruntwork-io/boilerplate/variables"
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBoilerplateOptions creates a BoilerplateOptions for testing
func newTestBoilerplateOptions(templateFolder, outputFolder string, vars map[string]any, noShell, noHooks bool) *boilerplateoptions.BoilerplateOptions {
	return &boilerplateoptions.BoilerplateOptions{
		TemplateFolder:          templateFolder,
		OutputFolder:            outputFolder,
		OnMissingKey:            boilerplateoptions.DefaultMissingKeyAction,
		OnMissingConfig:         boilerplateoptions.DefaultMissingConfigAction,
		Vars:                    vars,
		ShellCommandAnswers:     map[string]bool{},
		NoShell:                 noShell,
		NoHooks:                 noHooks,
		NonInteractive:          true,
		DisableDependencyPrompt: false,
	}
}

func TestDefaultTemplateVariables(t *testing.T) {
	t.Parallel()

	// set pre-defined variables
	vars := map[string]any{}

	var requiredVariables, optionalVariables []*config.ParsedVariable

	requiredVariables = append(requiredVariables, &config.ParsedVariable{
		Name:                    "required_var_1",
		Description:             "required_var_1 description",
		Type:                    "string",
		DefaultValuePlaceholder: "\"\"",
	})

	optionalVariables = append(optionalVariables, &config.ParsedVariable{
		Name:         "optional_var_2",
		Description:  "optional_ver_2 description",
		Type:         "number",
		DefaultValue: "42",
	})

	vars["requiredVariables"] = requiredVariables
	vars["optionalVariables"] = optionalVariables

	vars["sourceUrl"] = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.53.8"

	vars["EnableRootInclude"] = false
	vars["RootFileName"] = "root.hcl"

	workDir := t.TempDir()
	templateDir := util.JoinPath(workDir, "template")
	err := os.Mkdir(templateDir, 0755)
	require.NoError(t, err)

	outputDir := util.JoinPath(workDir, "output")
	err = os.Mkdir(outputDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(util.JoinPath(templateDir, "terragrunt.hcl"), []byte(scaffold.DefaultTerragruntTemplate), 0644)
	require.NoError(t, err)

	err = os.WriteFile(util.JoinPath(templateDir, "boilerplate.yml"), []byte(scaffold.DefaultBoilerplateConfig), 0644)
	require.NoError(t, err)

	boilerplateOpts := newTestBoilerplateOptions(templateDir, outputDir, vars, true, true)

	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	require.NoError(t, err)

	content, err := util.ReadFileAsString(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)
	require.Contains(t, content, "required_var_1")
	require.Contains(t, content, "optional_var_2")

	// read generated HCL file and check if it is parsed correctly
	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	l := logger.CreateLogger()

	cfg, err := config.ReadTerragruntConfig(t.Context(), l, opts, config.DefaultParserOptions(l, opts))
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["required_var_1"]
	require.True(t, found)
	require.Equal(t, "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.53.8", *cfg.Terraform.Source)
}

func TestCatalogConfigApplication(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cliNoShell       *bool
		cliNoHooks       *bool
		name             string
		terragruntConfig string
		description      string
		expectedNoShell  bool
		expectedNoHooks  bool
	}{
		{
			name: "config_both_flags_true",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = true
  no_hooks = true
}`,
			expectedNoShell: true,
			expectedNoHooks: true,
			description:     "Catalog config sets both flags to true",
		},
		{
			name: "config_both_flags_false",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = false
  no_hooks = false
}`,
			description: "Catalog config sets both flags to false",
		},
		{
			name: "config_shell_true_hooks_false",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = true
  no_hooks = false
}`,
			expectedNoShell: true,
			description:     "Catalog config sets no_shell=true, no_hooks=false",
		},
		{
			name: "config_shell_false_hooks_true",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = false
  no_hooks = true
}`,
			expectedNoHooks: true,
			description:     "Catalog config sets no_shell=false, no_hooks=true",
		},
		// Test CLI flags overriding catalog config
		{
			name: "cli_override_config_true_with_false",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = true
  no_hooks = true
}`,
			cliNoShell:  boolPtr(false),
			cliNoHooks:  boolPtr(false),
			description: "CLI flags override catalog config (CLI false > config true)",
		},
		{
			name: "cli_override_config_false_with_true",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = false
  no_hooks = false
}`,
			cliNoShell:      boolPtr(true),
			cliNoHooks:      boolPtr(true),
			expectedNoShell: true,
			expectedNoHooks: true,
			description:     "CLI flags override catalog config (CLI true > config false)",
		},
		{
			name: "cli_partial_override_shell_only",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = false
  no_hooks = true
}`,
			cliNoShell:      boolPtr(true),
			expectedNoShell: true,
			expectedNoHooks: true,
			description:     "CLI --no-shell overrides config, no_hooks from config",
		},
		{
			name: "cli_partial_override_hooks_only",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = true
  no_hooks = false
}`,
			cliNoHooks:      boolPtr(true),
			expectedNoShell: true,
			expectedNoHooks: true,
			description:     "CLI --no-hooks overrides config, no_shell from config",
		},
		// Test behavior when attributes are omitted from config
		{
			name: "config_omitted_attributes_no_cli",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
}`,
			description: "Config omits no_shell/no_hooks, no CLI flags - should default to false",
		},
		{
			name: "config_omitted_attributes_cli_true",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
}`,
			cliNoShell:      boolPtr(true),
			cliNoHooks:      boolPtr(true),
			expectedNoShell: true,
			expectedNoHooks: true,
			description:     "Config omits no_shell/no_hooks, CLI sets both true - CLI should take effect",
		},
		{
			name: "config_omitted_attributes_cli_false",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
}`,
			cliNoShell:  boolPtr(false),
			cliNoHooks:  boolPtr(false),
			description: "Config omits no_shell/no_hooks, CLI sets both false - should remain false",
		},
		{
			name: "config_omitted_attributes_cli_partial",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
}`,
			cliNoShell:      boolPtr(true),
			expectedNoShell: true,
			description:     "Config omits attributes, only CLI --no-shell set - only no_shell should be true",
		},
		// Test mixed scenarios with some attributes omitted
		{
			name: "config_partial_shell_only_no_cli",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = true
}`,
			expectedNoShell: true,
			description:     "Config sets only no_shell=true, no_hooks omitted - should be true/false",
		},
		{
			name: "config_partial_hooks_only_no_cli",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_hooks = true
}`,
			expectedNoHooks: true,
			description:     "Config sets only no_hooks=true, no_shell omitted - should be false/true",
		},
		{
			name: "config_partial_shell_only_cli_override_hooks",
			terragruntConfig: `
catalog {
  urls = ["test-url"]
  no_shell = false
}`,
			cliNoHooks:      boolPtr(true),
			expectedNoHooks: true,
			description:     "Config sets no_shell=false, no_hooks omitted, CLI --no-hooks - should be false/true",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			workDir := t.TempDir()
			configDir := util.JoinPath(workDir, "config")

			err := os.MkdirAll(configDir, 0755)
			require.NoError(t, err)

			terragruntConfigPath := util.JoinPath(configDir, "terragrunt.hcl")
			err = os.WriteFile(terragruntConfigPath, []byte(tc.terragruntConfig), 0644)
			require.NoError(t, err)

			opts := options.NewTerragruntOptions()
			// Set CLI flags if specified in test case
			if tc.cliNoShell != nil {
				opts.NoShell = *tc.cliNoShell
			} else {
				opts.NoShell = false
			}

			if tc.cliNoHooks != nil {
				opts.NoHooks = *tc.cliNoHooks
			} else {
				opts.NoHooks = false
			}

			opts.TerragruntConfigPath = terragruntConfigPath
			opts.WorkingDir = configDir
			opts.ScaffoldRootFileName = "terragrunt.hcl"

			l := logger.CreateLogger()

			// First, verify catalog config parsing
			catalogCfg, err := config.ReadCatalogConfig(context.Background(), l, opts)
			require.NoError(t, err)
			require.NotNil(t, catalogCfg, tc.description)

			// Verify config parsing based on whether attributes are present in the config
			if strings.Contains(tc.terragruntConfig, "no_shell") {
				assert.NotNil(t, catalogCfg.NoShell, "NoShell should not be nil when specified in config: %s", tc.description)
			} else {
				assert.Nil(t, catalogCfg.NoShell, "NoShell should be nil when omitted from config: %s", tc.description)
			}

			if strings.Contains(tc.terragruntConfig, "no_hooks") {
				assert.NotNil(t, catalogCfg.NoHooks, "NoHooks should not be nil when specified in config: %s", tc.description)
			} else {
				assert.Nil(t, catalogCfg.NoHooks, "NoHooks should be nil when omitted from config: %s", tc.description)
			}

			// Apply catalog config settings to options (simulating scaffold.Run behavior)
			// Only apply config values if CLI flags weren't explicitly set
			if tc.cliNoShell == nil && catalogCfg.NoShell != nil && *catalogCfg.NoShell {
				opts.NoShell = true
			}

			if tc.cliNoHooks == nil && catalogCfg.NoHooks != nil && *catalogCfg.NoHooks {
				opts.NoHooks = true
			}

			// Verify final option values match expected (after config application + CLI override)
			assert.Equal(t, tc.expectedNoShell, opts.NoShell, "Final NoShell value should match expected: %s", tc.description)
			assert.Equal(t, tc.expectedNoHooks, opts.NoHooks, "Final NoHooks value should match expected: %s", tc.description)
		})
	}
}

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}

// TestCatalogConfigParsing tests that catalog config is properly parsed with new attributes
func TestCatalogConfigParsing(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	// Test with no_shell and no_hooks attributes
	terragruntConfig := `
catalog {
  default_template = "test-template"
  urls = ["url1", "url2"]
  no_shell = true
  no_hooks = false
}
`
	terragruntConfigPath := util.JoinPath(workDir, "terragrunt.hcl")
	err := os.WriteFile(terragruntConfigPath, []byte(terragruntConfig), 0644)
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.TerragruntConfigPath = terragruntConfigPath
	opts.WorkingDir = workDir
	opts.ScaffoldRootFileName = "terragrunt.hcl"

	l := logger.CreateLogger()

	// Parse the configuration
	catalogCfg, err := config.ReadCatalogConfig(context.Background(), l, opts)
	require.NoError(t, err)
	require.NotNil(t, catalogCfg)

	// Verify all fields are correctly parsed
	assert.Equal(t, "test-template", catalogCfg.DefaultTemplate)
	assert.Equal(t, []string{"url1", "url2"}, catalogCfg.URLs)
	assert.NotNil(t, catalogCfg.NoShell)
	assert.True(t, *catalogCfg.NoShell)
	assert.NotNil(t, catalogCfg.NoHooks)
	assert.False(t, *catalogCfg.NoHooks)
}

// TestCatalogConfigOptional tests that no_shell and no_hooks are optional attributes
func TestCatalogConfigOptional(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	// Test without no_shell and no_hooks attributes
	terragruntConfig := `
catalog {
  default_template = "test-template"
  urls = ["url1"]
}
`
	terragruntConfigPath := util.JoinPath(workDir, "terragrunt.hcl")
	err := os.WriteFile(terragruntConfigPath, []byte(terragruntConfig), 0644)
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.TerragruntConfigPath = terragruntConfigPath
	opts.WorkingDir = workDir
	opts.ScaffoldRootFileName = "terragrunt.hcl"

	l := logger.CreateLogger()

	// Parse the configuration
	catalogCfg, err := config.ReadCatalogConfig(context.Background(), l, opts)
	require.NoError(t, err)
	require.NotNil(t, catalogCfg)

	// Verify optional fields are nil when not specified
	assert.Equal(t, "test-template", catalogCfg.DefaultTemplate)
	assert.Equal(t, []string{"url1"}, catalogCfg.URLs)
	assert.Nil(t, catalogCfg.NoShell, "NoShell should be nil when not specified")
	assert.Nil(t, catalogCfg.NoHooks, "NoHooks should be nil when not specified")
}

// TestBoilerplateShellTemplateFunctionDisabled tests that NoShell=true disables shell template functions
func TestBoilerplateShellTemplateFunctionDisabled(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	templateDir := util.JoinPath(workDir, "template")
	outputDir := util.JoinPath(workDir, "output")

	// Create template and output directories
	err := os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(outputDir, 0755)
	require.NoError(t, err)

	// Create boilerplate.yml
	boilerplateConfig := `
variables:
  - name: TestVar
    description: A test variable
    type: string
    default: "test-value"
`
	err = os.WriteFile(util.JoinPath(templateDir, "boilerplate.yml"), []byte(boilerplateConfig), 0644)
	require.NoError(t, err)

	// Create template file with shell template function
	templateContent := `# Test template with shell function
test_var = "{{ .TestVar }}"
# This shell function should NOT execute when NoShell=true
shell_output = "{{ shell "echo SHELL_EXECUTED" }}"
`
	err = os.WriteFile(util.JoinPath(templateDir, "test.txt"), []byte(templateContent), 0644)
	require.NoError(t, err)

	// Create BoilerplateOptions with NoShell=true
	boilerplateOpts := newTestBoilerplateOptions(templateDir, outputDir, map[string]any{}, true, false)

	// Process the template
	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	require.NoError(t, err)

	// Verify the file was generated
	generatedFile := util.JoinPath(outputDir, "test.txt")
	require.FileExists(t, generatedFile)

	content, err := util.ReadFileAsString(generatedFile)
	require.NoError(t, err)

	// Verify that template variables were processed
	assert.Contains(t, content, "test-value", "Template variable should be processed")

	// When shell is disabled, the shell function should remain unprocessed
	// Note: The exact behavior depends on how boilerplate handles disabled shell functions
	// It might either leave the template as-is or throw an error
	assert.NotContains(t, content, "SHELL_EXECUTED", "Shell function should not execute when NoShell=true")
}

// TestBoilerplateShellTemplateFunctionEnabled tests that NoShell=false allows shell template functions
func TestBoilerplateShellTemplateFunctionEnabled(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	templateDir := util.JoinPath(workDir, "template")
	outputDir := util.JoinPath(workDir, "output")

	// Create template and output directories
	err := os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(outputDir, 0755)
	require.NoError(t, err)

	// Create boilerplate.yml
	boilerplateConfig := `
variables:
  - name: TestVar
    description: A test variable
    type: string
    default: "test-value"
`
	err = os.WriteFile(util.JoinPath(templateDir, "boilerplate.yml"), []byte(boilerplateConfig), 0644)
	require.NoError(t, err)

	// Create template file with shell template function
	templateContent := `# Test template with shell function
test_var = "{{ .TestVar }}"
# This shell function SHOULD execute when NoShell=false
shell_output = "{{ shell "echo" "SHELL_EXECUTED" }}"
`
	err = os.WriteFile(util.JoinPath(templateDir, "test.txt"), []byte(templateContent), 0644)
	require.NoError(t, err)

	// Create BoilerplateOptions with NoShell=false
	boilerplateOpts := newTestBoilerplateOptions(templateDir, outputDir, map[string]any{}, false, false)

	// Process the template
	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	require.NoError(t, err)

	// Verify the file was generated
	generatedFile := util.JoinPath(outputDir, "test.txt")
	require.FileExists(t, generatedFile)

	content, err := util.ReadFileAsString(generatedFile)
	require.NoError(t, err)

	// Verify that template variables were processed
	assert.Contains(t, content, "test-value", "Template variable should be processed")

	// When shell is enabled, the shell function should execute and output should be present
	assert.Contains(t, content, "SHELL_EXECUTED", "Shell function should execute when NoShell=false")
}

// TestBoilerplateHooksDisabled tests that NoHooks=true disables hooks
func TestBoilerplateHooksDisabled(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	templateDir := util.JoinPath(workDir, "template")
	outputDir := util.JoinPath(workDir, "output")

	// Create template and output directories
	err := os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(outputDir, 0755)
	require.NoError(t, err)

	// Create boilerplate.yml with hooks
	boilerplateConfig := `
variables:
  - name: TestVar
    description: A test variable
    type: string
    default: "test-value"

hooks:
  before:
    - command: touch
      args:
        - ` + outputDir + `/before_hook_not_executed.txt
      description: "Test hook that should NOT execute"
  after:
    - command: touch
      args:
        - ` + outputDir + `/after_hook_not_executed.txt
      description: "Test hook that should NOT execute"
`
	err = os.WriteFile(util.JoinPath(templateDir, "boilerplate.yml"), []byte(boilerplateConfig), 0644)
	require.NoError(t, err)

	// Create simple template file
	templateContent := `# Test template
test_var = "{{ .TestVar }}"
`
	err = os.WriteFile(util.JoinPath(templateDir, "test.txt"), []byte(templateContent), 0644)
	require.NoError(t, err)

	// Create BoilerplateOptions with NoHooks=true
	boilerplateOpts := newTestBoilerplateOptions(templateDir, outputDir, map[string]any{}, false, true)

	// Process the template
	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	require.NoError(t, err)

	// Verify the template file was generated
	generatedFile := util.JoinPath(outputDir, "test.txt")
	require.FileExists(t, generatedFile)

	content, err := util.ReadFileAsString(generatedFile)
	require.NoError(t, err)
	assert.Contains(t, content, "test-value", "Template variable should be processed")

	// Verify that hooks did NOT execute (hook files should not exist)
	beforeHookFile := util.JoinPath(outputDir, "before_hook_not_executed.txt")
	afterHookFile := util.JoinPath(outputDir, "after_hook_not_executed.txt")

	assert.NoFileExists(t, beforeHookFile, "Before hook file should not exist when NoHooks=true")
	assert.NoFileExists(t, afterHookFile, "After hook file should not exist when NoHooks=true")
}

// TestBoilerplateHooksEnabled tests that NoHooks=false allows hooks to execute
func TestBoilerplateHooksEnabled(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	templateDir := util.JoinPath(workDir, "template")
	outputDir := util.JoinPath(workDir, "output")

	// Create template and output directories
	err := os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(outputDir, 0755)
	require.NoError(t, err)

	// Create boilerplate.yml with hooks
	boilerplateConfig := `
variables:
  - name: TestVar
    description: A test variable
    type: string
    default: "test-value"

hooks:
  before:
    - command: touch
      args:
        - ` + outputDir + `/before_hook_executed.txt
      description: "Test hook that SHOULD execute"
  after:
    - command: touch
      args:
        - ` + outputDir + `/after_hook_executed.txt
      description: "Test hook that SHOULD execute"
`
	err = os.WriteFile(util.JoinPath(templateDir, "boilerplate.yml"), []byte(boilerplateConfig), 0644)
	require.NoError(t, err)

	// Create simple template file
	templateContent := `# Test template
test_var = "{{ .TestVar }}"
`
	err = os.WriteFile(util.JoinPath(templateDir, "test.txt"), []byte(templateContent), 0644)
	require.NoError(t, err)

	// Create BoilerplateOptions with NoHooks=false
	boilerplateOpts := newTestBoilerplateOptions(templateDir, outputDir, map[string]any{}, false, false)

	// Process the template
	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	require.NoError(t, err)

	// Verify the template file was generated
	generatedFile := util.JoinPath(outputDir, "test.txt")
	require.FileExists(t, generatedFile)

	content, err := util.ReadFileAsString(generatedFile)
	require.NoError(t, err)
	assert.Contains(t, content, "test-value", "Template variable should be processed")

	// Verify that hooks DID execute (before and after hook files should exist)
	beforeHookFile := util.JoinPath(outputDir, "before_hook_executed.txt")
	afterHookFile := util.JoinPath(outputDir, "after_hook_executed.txt")

	require.FileExists(t, beforeHookFile, "Before hook file should exist when NoHooks=false")
	require.FileExists(t, afterHookFile, "After hook file should exist when NoHooks=false")
}

// TestBoilerplateBothFlagsDisabled tests that both NoShell=true and NoHooks=true work together
func TestBoilerplateBothFlagsDisabled(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	templateDir := util.JoinPath(workDir, "template")
	outputDir := util.JoinPath(workDir, "output")

	// Create template and output directories
	err := os.MkdirAll(templateDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(outputDir, 0755)
	require.NoError(t, err)

	// Create boilerplate.yml with both hooks and variables
	boilerplateConfig := `
variables:
  - name: TestVar
    description: A test variable
    type: string
    default: "test-value"

hooks:
  before:
    - command: echo "HOOK_EXECUTED" > ` + outputDir + `/hook_output.txt
      description: "Test hook that should NOT execute"
`
	err = os.WriteFile(util.JoinPath(templateDir, "boilerplate.yml"), []byte(boilerplateConfig), 0644)
	require.NoError(t, err)

	// Create template file with shell template function
	templateContent := `# Test template
test_var = "{{ .TestVar }}"
shell_result = "{{ shell "echo SHELL_EXECUTED" }}"
`
	err = os.WriteFile(util.JoinPath(templateDir, "test.txt"), []byte(templateContent), 0644)
	require.NoError(t, err)

	// Create BoilerplateOptions with both NoShell=true and NoHooks=true
	boilerplateOpts := newTestBoilerplateOptions(templateDir, outputDir, map[string]any{}, true, true)

	// Process the template
	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(boilerplateOpts, boilerplateOpts, emptyDep)
	require.NoError(t, err)

	// Verify the template file was generated
	generatedFile := util.JoinPath(outputDir, "test.txt")
	require.FileExists(t, generatedFile)

	content, err := util.ReadFileAsString(generatedFile)
	require.NoError(t, err)

	// Verify that template variables were processed
	assert.Contains(t, content, "test-value", "Template variable should be processed")

	// Verify that shell function did NOT execute
	assert.NotContains(t, content, "SHELL_EXECUTED", "Shell function should not execute when NoShell=true")

	// Verify that hooks did NOT execute
	hookOutputFile := util.JoinPath(outputDir, "hook_output.txt")
	assert.NoFileExists(t, hookOutputFile, "Hook should not execute when NoHooks=true")
}
