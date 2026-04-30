// This file validates that the pkg/config package is usable by external consumers
// as a public API. All tests here use only the external (black-box) package name
// `config_test` and import only public packages — no `internal/` imports are allowed.

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func createExternalLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func TestExternalConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "terragrunt.hcl", config.DefaultTerragruntConfigPath)
	assert.Equal(t, "terragrunt.stack.hcl", config.DefaultStackFile)
	assert.Equal(t, ".terragrunt-stack", config.StackDir)
	assert.Equal(t, "terragrunt.hcl.json", config.DefaultTerragruntJSONConfigPath)
	assert.Equal(t, "root.hcl", config.RecommendedParentConfigName)
	assert.Equal(t, "found_in_file", config.FoundInFile)
	assert.NotEmpty(t, config.DefaultTerragruntConfigPaths)
}

func TestExternalTerragruntConfigStruct(t *testing.T) {
	t.Parallel()

	cfg := &config.TerragruntConfig{
		TerraformBinary:             "tofu",
		TerragruntVersionConstraint: ">= 0.50.0",
		TerraformVersionConstraint:  ">= 1.5.0",
		DownloadDir:                 "/tmp/download",
		IamRole:                     "arn:aws:iam::123456789012:role/test",
		IamAssumeRoleSessionName:    "test-session",
		IamWebIdentityToken:         "token-value",
		Inputs:                      map[string]any{"key": "value"},
		Locals:                      map[string]any{"local_key": "local_value"},
		IsPartial:                   false,
	}

	assert.Equal(t, "tofu", cfg.TerraformBinary)
	assert.Equal(t, ">= 0.50.0", cfg.TerragruntVersionConstraint)
	assert.Equal(t, ">= 1.5.0", cfg.TerraformVersionConstraint)
	assert.Equal(t, "/tmp/download", cfg.DownloadDir)
	assert.Equal(t, "arn:aws:iam::123456789012:role/test", cfg.IamRole)
	assert.Equal(t, "test-session", cfg.IamAssumeRoleSessionName)
	assert.Equal(t, "token-value", cfg.IamWebIdentityToken)
	assert.Equal(t, map[string]any{"key": "value"}, cfg.Inputs)
	assert.Equal(t, map[string]any{"local_key": "local_value"}, cfg.Locals)
	assert.False(t, cfg.IsPartial)
	assert.Nil(t, cfg.Terraform)
	assert.Nil(t, cfg.RemoteState)
	assert.Nil(t, cfg.Dependencies)
	assert.Nil(t, cfg.Engine)
	assert.Nil(t, cfg.PreventDestroy)
}

func TestExternalTerragruntConfigAsCty(t *testing.T) {
	t.Parallel()

	cfg := &config.TerragruntConfig{
		TerraformBinary: "terraform",
		Inputs:          map[string]any{"env": "dev"},
	}

	ctyVal, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)
	assert.True(t, ctyVal.IsKnown())
	assert.True(t, ctyVal.Type().IsObjectType())
}

func TestExternalGetTerraformSourceURL(t *testing.T) {
	t.Parallel()

	t.Run("explicit source overrides config", func(t *testing.T) {
		t.Parallel()

		result, err := config.GetTerraformSourceURL("explicit-source", nil, "config.hcl", &config.TerragruntConfig{})
		require.NoError(t, err)
		assert.Equal(t, "explicit-source", result)
	})

	t.Run("no source returns dot", func(t *testing.T) {
		t.Parallel()

		result, err := config.GetTerraformSourceURL("", nil, "config.hcl", &config.TerragruntConfig{})
		require.NoError(t, err)
		assert.Equal(t, ".", result)
	})
}

func TestExternalEngineConfig(t *testing.T) {
	t.Parallel()

	version := "1.0.0"
	engineType := "rpc"

	engine := &config.EngineConfig{
		Source:  "github.com/example/engine",
		Version: &version,
		Type:    &engineType,
	}

	t.Run("clone", func(t *testing.T) {
		t.Parallel()

		cloned := engine.Clone()
		assert.Equal(t, engine.Source, cloned.Source)
		assert.Equal(t, *engine.Version, *cloned.Version)
		assert.Equal(t, *engine.Type, *cloned.Type)
	})

	t.Run("merge", func(t *testing.T) {
		t.Parallel()

		base := &config.EngineConfig{
			Source: "original-source",
		}
		newVersion := "2.0.0"
		override := &config.EngineConfig{
			Source:  "new-source",
			Version: &newVersion,
		}
		base.Merge(override)
		assert.Equal(t, "new-source", base.Source)
		assert.Equal(t, "2.0.0", *base.Version)
	})
}

func TestExternalCtyHelpers(t *testing.T) {
	t.Parallel()

	t.Run("GetValueString with string", func(t *testing.T) {
		t.Parallel()

		result, err := config.GetValueString(cty.StringVal("hello"))
		require.NoError(t, err)
		assert.Equal(t, "hello", result)
	})

	t.Run("GetValueString with number", func(t *testing.T) {
		t.Parallel()

		result, err := config.GetValueString(cty.NumberIntVal(42))
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})

	t.Run("GetFirstKey", func(t *testing.T) {
		t.Parallel()

		m := map[string]cty.Value{"only_key": cty.StringVal("val")}
		assert.Equal(t, "only_key", config.GetFirstKey(m))
	})

	t.Run("GetFirstKey empty map", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, config.GetFirstKey(map[string]cty.Value{}))
	})

	t.Run("IsComplexType", func(t *testing.T) {
		t.Parallel()

		assert.False(t, config.IsComplexType(cty.StringVal("simple")))
		assert.False(t, config.IsComplexType(cty.NumberIntVal(1)))
		assert.True(t, config.IsComplexType(cty.ObjectVal(map[string]cty.Value{"k": cty.StringVal("v")})))
		assert.True(t, config.IsComplexType(cty.ListVal([]cty.Value{cty.StringVal("a")})))
	})

	t.Run("ConvertValuesMapToCtyVal", func(t *testing.T) {
		t.Parallel()

		valMap := map[string]cty.Value{
			"str": cty.StringVal("value"),
			"num": cty.NumberIntVal(10),
		}
		result, err := config.ConvertValuesMapToCtyVal(valMap)
		require.NoError(t, err)
		assert.True(t, result.Type().IsObjectType())
	})

	t.Run("ConvertValuesMapToCtyVal empty", func(t *testing.T) {
		t.Parallel()

		result, err := config.ConvertValuesMapToCtyVal(map[string]cty.Value{})
		require.NoError(t, err)
		assert.Equal(t, cty.EmptyObjectVal, result)
	})
}

func TestExternalTerraformOutputJSONToCtyValueMap(t *testing.T) {
	t.Parallel()

	jsonOutput := []byte(`{
		"vpc_id": {
			"sensitive": false,
			"type": "string",
			"value": "vpc-abc123"
		},
		"instance_count": {
			"sensitive": false,
			"type": "number",
			"value": 3
		}
	}`)

	result, err := config.TerraformOutputJSONToCtyValueMap("test-config", jsonOutput)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	vpcID := result["vpc_id"]
	assert.Equal(t, cty.String, vpcID.Type())
	assert.Equal(t, "vpc-abc123", vpcID.AsString())
}

func TestExternalGetUnitDir(t *testing.T) {
	t.Parallel()

	t.Run("with stack dir", func(t *testing.T) {
		t.Parallel()

		unit := &config.Unit{
			Name:   "app",
			Source: "./modules/app",
			Path:   "app",
		}
		dir := config.GetUnitDir("/project", unit)
		assert.Equal(t, filepath.Join("/project", config.StackDir, "app"), dir)
	})

	t.Run("no stack dir", func(t *testing.T) {
		t.Parallel()

		noStack := true
		unit := &config.Unit{
			Name:    "app",
			Source:  "./modules/app",
			Path:    "app",
			NoStack: &noStack,
		}
		dir := config.GetUnitDir("/project", unit)
		assert.Equal(t, filepath.Join("/project", "app"), dir)
	})
}

func TestExternalStackTypes(t *testing.T) {
	t.Parallel()

	t.Run("StackConfig", func(t *testing.T) {
		t.Parallel()

		sc := &config.StackConfig{
			Units: []*config.Unit{
				{Name: "web", Source: "./web", Path: "web"},
			},
			Stacks: []*config.Stack{
				{Name: "infra", Source: "./infra", Path: "infra"},
			},
		}
		assert.Len(t, sc.Units, 1)
		assert.Len(t, sc.Stacks, 1)
		assert.Equal(t, "web", sc.Units[0].Name)
		assert.Equal(t, "infra", sc.Stacks[0].Name)
	})

	t.Run("Unit fields", func(t *testing.T) {
		t.Parallel()

		noValidation := true
		unit := &config.Unit{
			Name:         "db",
			Path:         "database",
			NoValidation: &noValidation,
		}
		assert.Equal(t, "db", unit.Name)
		assert.Equal(t, "database", unit.Path)
		assert.True(t, *unit.NoValidation)
	})

	t.Run("Stack fields", func(t *testing.T) {
		t.Parallel()

		noStack := false
		stack := &config.Stack{
			Name:    "networking",
			Path:    "net",
			NoStack: &noStack,
		}
		assert.Equal(t, "networking", stack.Name)
		assert.Equal(t, "net", stack.Path)
		assert.False(t, *stack.NoStack)
	})
}

func TestExternalHclparse(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()
	parser := hclparse.NewParser(hclparse.WithLogger(l))

	hclContent := `
		name = "test"
		count = 42
	`

	file, err := parser.ParseFromString(hclContent, "test.hcl")
	require.NoError(t, err)
	require.NotNil(t, file)

	attrs, err := file.JustAttributes()
	require.NoError(t, err)

	attrNames := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		attrNames = append(attrNames, attr.Name)
	}

	assert.Contains(t, attrNames, "name")
	assert.Contains(t, attrNames, "count")
}

func TestExternalModuleDependencies(t *testing.T) {
	t.Parallel()

	t.Run("create and read", func(t *testing.T) {
		t.Parallel()

		deps := &config.ModuleDependencies{
			Paths: []string{"../vpc", "../rds"},
		}
		assert.Len(t, deps.Paths, 2)
		assert.Equal(t, "../vpc", deps.Paths[0])
	})

	t.Run("merge", func(t *testing.T) {
		t.Parallel()

		deps := &config.ModuleDependencies{
			Paths: []string{"../vpc"},
		}
		other := &config.ModuleDependencies{
			Paths: []string{"../rds", "../vpc"},
		}
		deps.Merge(other)
		assert.Len(t, deps.Paths, 2)
		assert.Contains(t, deps.Paths, "../vpc")
		assert.Contains(t, deps.Paths, "../rds")
	})

	t.Run("merge nil", func(t *testing.T) {
		t.Parallel()

		deps := &config.ModuleDependencies{
			Paths: []string{"../vpc"},
		}
		deps.Merge(nil)
		assert.Len(t, deps.Paths, 1)
	})
}

func TestExternalGetDefaultConfigPath(t *testing.T) {
	t.Parallel()

	// When given a non-existent directory, GetDefaultConfigPath returns a path
	// ending with the default config file name.
	result := config.GetDefaultConfigPath("/some/nonexistent/path")
	assert.Contains(t, result, "terragrunt.hcl")
}

func TestExternalParseAndDecodeVarFile(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	varFileContent := []byte(`
		region = "us-east-1"
		enabled = true
	`)

	var out map[string]any

	err := config.ParseAndDecodeVarFile(l, "test.hcl", varFileContent, &out)
	require.NoError(t, err)

	assert.Contains(t, out, "region")
	assert.Contains(t, out, "enabled")
	assert.Equal(t, "us-east-1", out["region"])
	assert.Equal(t, true, out["enabled"])
}

// TestExternalParseConfigStringNoCommand validates that an external consumer can
// parse a Terragrunt config using NewParsingContext with zero options (no
// internal/ imports required). This previously caused a nil pointer dereference
// when TerraformCliArgs was nil.
func TestExternalParseConfigStringNoCommand(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	hclConfig := `
inputs = {
  env = "dev"
}
`

	ctx := t.Context()
	ctx, pctx := config.NewParsingContext(ctx, l)
	cfg, err := config.ParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, hclConfig, nil)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "dev", cfg.Inputs["env"])
}

// TestExternalParseStackConfigString validates that an external consumer can
// parse a terragrunt.stack.hcl config using NewParsingContext with zero options
// and no internal/ imports.
func TestExternalParseStackConfigString(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	stackHCL := `
unit "app" {
  source = "./modules/app"
  path   = "app"
}

unit "db" {
  source = "./modules/db"
  path   = "database"
}
`

	ctx := t.Context()
	ctx, pctx := config.NewParsingContext(ctx, l)

	v := cty.ObjectVal(map[string]cty.Value{})

	sc, err := config.ReadStackConfigString(
		ctx,
		l,
		pctx,
		config.DefaultStackFile,
		stackHCL,
		&v,
	)
	require.NoError(t, err)
	require.NotNil(t, sc)
	require.Len(t, sc.Units, 2)
	assert.Equal(t, "app", sc.Units[0].Name)
	assert.Equal(t, "./modules/app", sc.Units[0].Source)
	assert.Equal(t, "app", sc.Units[0].Path)
	assert.Equal(t, "db", sc.Units[1].Name)
	assert.Equal(t, "database", sc.Units[1].Path)
}

// TestExternalParseStackConfigStringNilValues validates that an external consumer can
// parse a terragrunt.stack.hcl config using NewParsingContext with nil for values.
func TestExternalParseStackConfigStringNilValues(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	stackHCL := `
unit "app" {
  source = "./modules/app"
  path   = "app"
}

unit "db" {
  source = "./modules/db"
  path   = "database"
}
`

	ctx := t.Context()
	ctx, pctx := config.NewParsingContext(ctx, l)

	sc, err := config.ReadStackConfigString(
		ctx,
		l,
		pctx,
		config.DefaultStackFile,
		stackHCL,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, sc)
	require.Len(t, sc.Units, 2)
	assert.Equal(t, "app", sc.Units[0].Name)
	assert.Equal(t, "./modules/app", sc.Units[0].Source)
	assert.Equal(t, "app", sc.Units[0].Path)
	assert.Equal(t, "db", sc.Units[1].Name)
	assert.Equal(t, "database", sc.Units[1].Path)
}

// TestExternalParseStackConfigValidValues validates that an external consumer can
// parse a terragrunt.stack.hcl config using NewParsingContext with valid values.
func TestExternalParseStackConfigStringValidValues(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	stackHCL := `
unit "app" {
  source = "./modules/app"
  path   = values.app_path
}

unit "db" {
  source = "./modules/db"
  path   = "database"
}
`

	ctx := t.Context()
	ctx, pctx := config.NewParsingContext(ctx, l)

	v := cty.ObjectVal(map[string]cty.Value{
		"app_path": cty.StringVal("foo"),
	})

	sc, err := config.ReadStackConfigString(
		ctx,
		l,
		pctx,
		config.DefaultStackFile,
		stackHCL,
		&v,
	)
	require.NoError(t, err)
	require.NotNil(t, sc)
	require.Len(t, sc.Units, 2)
	assert.Equal(t, "app", sc.Units[0].Name)
	assert.Equal(t, "./modules/app", sc.Units[0].Source)
	assert.Equal(t, "foo", sc.Units[0].Path)
	assert.Equal(t, "db", sc.Units[1].Name)
	assert.Equal(t, "database", sc.Units[1].Path)
}

// TestExternalReadValuesAndParseStackConfig validates that an external consumer
// can read a terragrunt.values.hcl file from disk using ReadValues and feed the
// result into ReadStackConfigString — no internal/ imports required.
func TestExternalReadValuesAndParseStackConfig(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	// Write a terragrunt.values.hcl file to a temp directory.
	dir := t.TempDir()
	valuesContent := []byte(`
app_path = "my-app"
region   = "us-west-2"
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.values.hcl"), valuesContent, 0644))

	ctx := t.Context()
	ctx, pctx := config.NewParsingContext(ctx, l)

	// Read values from the file on disk.
	values, err := config.ReadValues(ctx, pctx, l, dir)
	require.NoError(t, err)
	require.NotNil(t, values)

	// Parse a stack config that references the values.
	stackHCL := `
unit "app" {
  source = "./modules/app"
  path   = values.app_path
}
`
	sc, err := config.ReadStackConfigString(ctx, l, pctx, config.DefaultStackFile, stackHCL, values)
	require.NoError(t, err)
	require.NotNil(t, sc)
	require.Len(t, sc.Units, 1)
	assert.Equal(t, "app", sc.Units[0].Name)
	assert.Equal(t, "my-app", sc.Units[0].Path)
}

// TestExternalReadValuesAndParseConfig validates that an external consumer can
// parse a regular terragrunt.hcl that references values.* when a
// terragrunt.values.hcl file sits next to it — no internal/ imports required.
//
// ParseConfig automatically calls ReadValues from the config file's directory,
// so the configPath argument must point into the directory containing the
// values file.
func TestExternalReadValuesAndParseConfig(t *testing.T) {
	t.Parallel()

	l := createExternalLogger()

	// Write a terragrunt.values.hcl file to a temp directory.
	dir := t.TempDir()
	valuesContent := []byte(`
env    = "staging"
region = "eu-west-1"
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.values.hcl"), valuesContent, 0644))

	ctx := t.Context()
	ctx, pctx := config.NewParsingContext(ctx, l)

	// Use a configPath inside the temp dir so ParseConfig discovers the
	// adjacent terragrunt.values.hcl automatically.
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)

	hclConfig := `
inputs = {
  env    = values.env
  region = values.region
}
`
	cfg, err := config.ParseConfigString(ctx, pctx, l, configPath, hclConfig, nil)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "staging", cfg.Inputs["env"])
	assert.Equal(t, "eu-west-1", cfg.Inputs["region"])
}
