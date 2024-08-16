package scaffold_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	boilerplateoptions "github.com/gruntwork-io/boilerplate/options"
	"github.com/gruntwork-io/boilerplate/templates"
	"github.com/gruntwork-io/boilerplate/variables"
	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTemplateVariables(t *testing.T) {
	t.Parallel()

	// set pre-defined variables
	vars := map[string]interface{}{}
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

	vars["sourceUrl"] = "git::https://github.com/gruntwork-io/terragrunt.git//test/fixture-inputs?ref=v0.53.8"

	vars["EnableRootInclude"] = false

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

	boilerplateOpts := &boilerplateoptions.BoilerplateOptions{
		OutputFolder:    outputDir,
		OnMissingKey:    boilerplateoptions.DefaultMissingKeyAction,
		OnMissingConfig: boilerplateoptions.DefaultMissingConfigAction,
		Vars:            vars,
		DisableShell:    true,
		DisableHooks:    true,
		NonInteractive:  true,
		TemplateFolder:  templateDir,
	}

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

	cfg, err := config.ReadTerragruntConfig(context.Background(), opts, config.DefaultParserOptions(opts))
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Inputs)
	assert.Len(t, cfg.Inputs, 1)
	_, found := cfg.Inputs["required_var_1"]
	require.True(t, found)
	require.Equal(t, "git::https://github.com/gruntwork-io/terragrunt.git//test/fixture-inputs?ref=v0.53.8", *cfg.Terraform.Source)

}
