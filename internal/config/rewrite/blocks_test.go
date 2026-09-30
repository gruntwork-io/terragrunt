package config_test

import (
	"cmp"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// maxSchemaDepth bounds the block nesting schemaConfig writes.
const maxSchemaDepth = 4

// TestSchemaCoverage pins parity for a config that declares every attribute and block, nested ones included, of
// [pkgconfig.TerragruntConfigFile].
func TestSchemaCoverage(t *testing.T) {
	t.Parallel()

	var body strings.Builder

	writeSchemaBody(t, &body, reflect.TypeFor[pkgconfig.TerragruntConfigFile](), 0)

	root := venvtest.Root("/fixture")

	assertParity(t, parityCase{
		newFS:     memFS(root, map[string]string{pkgconfig.DefaultTerragruntConfigPath: body.String()}),
		cfgPath:   filepath.Join(root, pkgconfig.DefaultTerragruntConfigPath),
		configure: func(*pkgconfig.ParsingContext) {},
	})
}

// TestSchemaMatchesPkgConfig pins that each header and body pair declares exactly the attributes, block types,
// labels, and field types of the matching pkg/config struct, with no name in both.
func TestSchemaMatchesPkgConfig(t *testing.T) {
	t.Parallel()

	rows := []schemaRow{
		{
			block:  "",
			v1:     reflect.TypeFor[pkgconfig.TerragruntConfigFile](),
			header: reflect.TypeFor[config.UnitConfig](),
			body:   reflect.TypeFor[config.UnitBody](),
		},
		{
			block:  pkgconfig.MetadataTerraform,
			v1:     reflect.TypeFor[pkgconfig.TerraformConfig](),
			header: reflect.TypeFor[config.Terraform](),
		},
		{
			block:  "before_hook",
			v1:     reflect.TypeFor[pkgconfig.Hook](),
			header: reflect.TypeFor[config.Hook](),
			body:   reflect.TypeFor[config.HookBody](),
		},
		{
			block:  "after_hook",
			v1:     reflect.TypeFor[pkgconfig.Hook](),
			header: reflect.TypeFor[config.Hook](),
			body:   reflect.TypeFor[config.HookBody](),
		},
		{
			block:  "error_hook",
			v1:     reflect.TypeFor[pkgconfig.ErrorHook](),
			header: reflect.TypeFor[config.ErrorHook](),
			body:   reflect.TypeFor[config.ErrorHookBody](),
		},
		{
			block:  "extra_arguments",
			v1:     reflect.TypeFor[pkgconfig.TerraformExtraArguments](),
			header: reflect.TypeFor[config.ExtraArguments](),
			body:   reflect.TypeFor[config.ExtraArgumentsBody](),
		},
		{
			block:  pkgconfig.MetadataRemoteState,
			v1:     reflect.TypeFor[remotestate.ConfigFile](),
			header: reflect.TypeFor[config.RemoteState](),
			body:   reflect.TypeFor[config.RemoteStateBody](),
		},
		{
			block:  pkgconfig.MetadataEngine,
			v1:     reflect.TypeFor[pkgconfig.EngineConfig](),
			header: reflect.TypeFor[config.Engine](),
			body:   reflect.TypeFor[config.EngineBody](),
		},
		{
			block:  pkgconfig.MetadataErrors,
			v1:     reflect.TypeFor[pkgconfig.ErrorsConfig](),
			header: reflect.TypeFor[config.Errors](),
		},
		{
			block:  pkgconfig.MetadataRetry,
			v1:     reflect.TypeFor[pkgconfig.RetryBlock](),
			header: reflect.TypeFor[config.Retry](),
			body:   reflect.TypeFor[config.RetryBody](),
		},
		{
			block:  pkgconfig.MetadataIgnore,
			v1:     reflect.TypeFor[pkgconfig.IgnoreBlock](),
			header: reflect.TypeFor[config.Ignore](),
			body:   reflect.TypeFor[config.IgnoreBody](),
		},
		{
			block:  pkgconfig.MetadataGenerateConfigs,
			v1:     reflect.TypeFor[pkgconfig.TerragruntGenerateBlock](),
			header: reflect.TypeFor[config.Generate](),
		},
		{
			block:  pkgconfig.MetadataCatalog,
			v1:     reflect.TypeFor[pkgconfig.CatalogConfig](),
			header: reflect.TypeFor[config.Catalog](),
		},
		{
			block:  pkgconfig.MetadataExclude,
			v1:     reflect.TypeFor[pkgconfig.ExcludeConfig](),
			header: reflect.TypeFor[config.Exclude](),
		},
		{
			block:  pkgconfig.MetadataFeatureFlag,
			v1:     reflect.TypeFor[pkgconfig.FeatureFlag](),
			header: reflect.TypeFor[config.FeatureFlag](),
		},
		{
			block:  pkgconfig.MetadataDependencies,
			v1:     reflect.TypeFor[pkgconfig.ModuleDependencies](),
			header: reflect.TypeFor[config.ModuleDependencies](),
		},
		{
			block:      pkgconfig.MetadataDependency,
			header:     reflect.TypeFor[config.Dependency](),
			labelsOnly: true,
		},
		{
			block:      pkgconfig.MetadataInclude,
			header:     reflect.TypeFor[config.Include](),
			labelsOnly: true,
		},
		{
			block:      pkgconfig.MetadataLocals,
			header:     reflect.TypeFor[config.Locals](),
			labelsOnly: true,
		},
	}

	rowBlocks := map[string]struct{}{}
	for _, row := range rows {
		rowBlocks[row.block] = struct{}{}
	}

	topLevel := impliedSchema(reflect.TypeFor[pkgconfig.TerragruntConfigFile]())

	for _, row := range rows {
		t.Run(cmp.Or(row.block, "top level"), func(t *testing.T) {
			t.Parallel()

			if row.labelsOnly {
				idx := slices.IndexFunc(
					topLevel.Blocks,
					func(b hcl.BlockHeaderSchema) bool { return b.Type == row.block },
				)
				require.GreaterOrEqual(t, idx, 0, "pkg/config has no %s block", row.block)
				assert.Equal(t, topLevel.Blocks[idx].LabelNames, labelNames(row.header), "labels of %s", row.block)

				return
			}

			header := impliedSchema(row.header)
			body := &hcl.BodySchema{}

			if row.body != nil {
				body = impliedSchema(row.body)
			}

			assert.Empty(t, overlap(header, body), "names in both the header and the body of %s", row.block)

			v1 := impliedSchema(row.v1)
			assert.Equal(
				t,
				sortedAttributes(v1.Attributes),
				sortedAttributes(slices.Concat(header.Attributes, body.Attributes)),
				"attributes of %s",
				row.block,
			)
			assert.Equal(t, sortedBlocks(v1.Blocks), sortedBlocks(slices.Concat(header.Blocks, body.Blocks)),
				"block types of %s", row.block)
			assert.Equal(t, labelNames(row.v1), labelNames(row.header), "labels of %s", row.block)
			assert.Equal(
				t,
				attributeTypes(row.v1, nil),
				attributeTypes(row.header, row.body),
				"attribute types of %s",
				row.block,
			)

			for _, block := range v1.Blocks {
				assert.Contains(t, rowBlocks, block.Type, "no row compares the %s block of %s", block.Type, row.block)
			}
		})
	}
}

// TestToV1LeavesDecodedBlocksUnchanged pins that repeated ToV1 calls on one parse return equal configs, although
// include merges mutate what they merge.
func TestToV1LeavesDecodedBlocksUnchanged(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	files := map[string]string{
		"root-dep/terragrunt.hcl": ``,
		"mid-dep/terragrunt.hcl":  ``,
		"unit-dep/terragrunt.hcl": ``,
		"auto-dep/terragrunt.hcl": ``,
		"root.hcl": `
terraform {
  extra_arguments "common" {
    commands  = ["plan"]
    arguments = ["-lock=false"]
  }

  before_hook "root" {
    commands = ["plan"]
    execute  = ["echo", "root"]
  }

  include_in_copy = ["root.txt"]
}

dependencies {
  paths = ["../root-dep"]
}

errors {
  retry "default" {
    retryable_errors   = ["root"]
    max_attempts       = 2
    sleep_interval_sec = 1
  }

  ignore "unit" {
    ignorable_errors = ["root"]
    signals = {
      root = true
    }
  }
}

catalog {
  urls = ["root-catalog"]
}

feature "root_flag" {
  default = false
}

exclude {
  if      = false
  actions = ["plan"]
}

engine {
  source = "root-engine"
  meta = {
    root = "yes"
  }
}

generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents  = "root"
}

inputs = {
  tags = { root = "yes" }
}
`,
		"mid.hcl": `
terraform {
  before_hook "root" {
    commands = ["apply"]
    execute  = ["echo", "mid"]
  }

  after_hook "mid" {
    commands = ["plan"]
    execute  = ["echo", "mid"]
  }

  include_in_copy = ["mid.txt"]
}

dependencies {
  paths = ["../mid-dep"]
}

`,
		"unit/terragrunt.hcl": `
include "root" {
  path           = "{{root}}/root.hcl"
  merge_strategy = "deep"
}

include "mid" {
  path           = "{{root}}/mid.hcl"
  merge_strategy = "shallow"
}

terraform {
  source = "."

  extra_arguments "common" {
    commands  = ["apply"]
    arguments = ["-input=false"]
  }

  error_hook "unit" {
    commands  = ["apply"]
    execute   = ["echo", "unit"]
    on_errors = [".*"]
  }
}

dependencies {
  paths = ["../unit-dep"]
}

errors {
  retry "default" {
    retryable_errors   = ["unit"]
    max_attempts       = 3
    sleep_interval_sec = 1
  }

  ignore "unit" {
    ignorable_errors = ["unit"]
    signals = {
      unit = true
    }
  }
}

catalog {
  urls = ["unit-catalog"]
}

feature "unit_flag" {
  default = true
}

engine {
  source  = "unit-engine"
  version = "v1.0.0"
  meta = {
    unit = "yes"
  }
}

inputs = {
  tags = { unit = "yes" }
}
`,
		"unit/terragrunt.autoinclude.hcl": `
terraform {
  after_hook "mid" {
    commands = ["apply"]
    execute  = ["echo", "autoinclude"]
  }
}

dependencies {
  paths = ["../auto-dep"]
}

inputs = {
  tags = { auto = "yes" }
}
`,
	}

	cfgPath := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)

	assertParity(t, parityCase{
		newFS:     memFS(root, files),
		cfgPath:   cfgPath,
		configure: func(*pkgconfig.ParsingContext) {},
	})

	v := venvtest.New().WithFS(memFS(root, files)(t))
	ctx, pctx := newTestParsingContext(t, cfgPath)

	l := logger.CreateLogger()

	parsed, err := config.ParseConfigFile(ctx, l, v, &hclparse.Store{}, config.NewParseContext(pctx), cfgPath)
	require.NoError(t, err)

	first, err := parsed.ToV1(ctx, l, v)
	require.NoError(t, err)

	second, err := parsed.ToV1(ctx, l, v)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Len(t, first.Dependencies.Paths, 4)
}

// writeSchemaBody writes a null attribute for every attribute of ty and a labeled block for every block type,
// recursing into each block's type.
func writeSchemaBody(t *testing.T, w *strings.Builder, ty reflect.Type, depth int) {
	t.Helper()

	require.Less(t, depth, maxSchemaDepth, "schema of %s nests too deep", ty)

	schema, _ := gohcl.ImpliedBodySchema(reflect.New(ty).Interface())
	blockTypes := map[string]struct{}{}

	for _, block := range schema.Blocks {
		blockTypes[block.Type] = struct{}{}
	}

	for _, attr := range schema.Attributes {
		if _, isBlock := blockTypes[attr.Name]; isBlock {
			continue
		}

		fmt.Fprintf(w, "%s = null\n", attr.Name)
	}

	for _, block := range schema.Blocks {
		w.WriteString(block.Type)

		for _, label := range block.LabelNames {
			fmt.Fprintf(w, " %q", "label-"+label)
		}

		w.WriteString(" {\n")
		writeSchemaBody(t, w, blockStructType(t, ty, block.Type), depth+1)
		w.WriteString("}\n")
	}
}

// blockStructType returns the struct type of the field of ty that holds blocks of typeName.
func blockStructType(t *testing.T, ty reflect.Type, typeName string) reflect.Type {
	t.Helper()

	for field := range ty.Fields() {
		if field.Tag.Get("hcl") != typeName+",block" {
			continue
		}

		fieldType := field.Type
		for fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		return fieldType
	}

	require.Failf(t, "no block field", "%s has no %s block field", ty, typeName)

	return nil
}

// schemaRow pairs a pkg/config struct with the rewrite's header and body structs for the same body.
type schemaRow struct {
	// v1 is the pkg/config struct, nil when labelsOnly is set.
	v1 reflect.Type
	// header decodes first.
	header reflect.Type
	// body decodes from header's Remain, or is nil when header decodes the whole body.
	body reflect.Type
	// block is the block type, empty for the top level.
	block string
	// labelsOnly compares only labels, for a block whose body pkg/config decodes elsewhere.
	labelsOnly bool
}

// impliedSchema returns the schema gohcl derives from the struct type ty.
func impliedSchema(ty reflect.Type) *hcl.BodySchema {
	schema, _ := gohcl.ImpliedBodySchema(reflect.New(ty).Interface())

	return schema
}

// labelNames returns the label names of the struct type ty, in label order.
func labelNames(ty reflect.Type) []string {
	var names []string

	for field := range ty.Fields() {
		name, kind, _ := strings.Cut(field.Tag.Get("hcl"), ",")
		if kind == "label" {
			names = append(names, name)
		}
	}

	return names
}

// attributeTypes maps each attribute of header and body to its field type. A header [hcl.Attribute] field takes the
// type of body's field of the same name.
func attributeTypes(header, body reflect.Type) map[string]string {
	types := map[string]string{}

	for _, ty := range []reflect.Type{header, body} {
		if ty == nil {
			continue
		}

		for field := range ty.Fields() {
			tag := field.Tag.Get("hcl")
			if tag == "" {
				continue
			}

			name, kind, _ := strings.Cut(tag, ",")
			if kind != "" && kind != "attr" && kind != "optional" {
				continue
			}

			types[name] = field.Type.String()

			if body == nil || field.Type != reflect.TypeFor[*hcl.Attribute]() {
				continue
			}

			if bodyField, ok := body.FieldByName(field.Name); ok {
				types[name] = bodyField.Type.String()
			}
		}
	}

	return types
}

// overlap returns the attribute names and block types that both header and body declare.
func overlap(header, body *hcl.BodySchema) []string {
	names := map[string]struct{}{}

	for _, attr := range header.Attributes {
		names[attr.Name] = struct{}{}
	}

	for _, block := range header.Blocks {
		names[block.Type] = struct{}{}
	}

	var both []string

	for _, attr := range body.Attributes {
		if _, ok := names[attr.Name]; ok {
			both = append(both, attr.Name)
		}
	}

	for _, block := range body.Blocks {
		if _, ok := names[block.Type]; ok {
			both = append(both, block.Type)
		}
	}

	return both
}

// sortedAttributes returns a copy of attrs sorted by name.
func sortedAttributes(attrs []hcl.AttributeSchema) []hcl.AttributeSchema {
	return slices.SortedFunc(slices.Values(attrs), func(a, b hcl.AttributeSchema) int {
		return strings.Compare(a.Name, b.Name)
	})
}

// sortedBlocks returns a copy of blocks sorted by type.
func sortedBlocks(blocks []hcl.BlockHeaderSchema) []hcl.BlockHeaderSchema {
	return slices.SortedFunc(slices.Values(blocks), func(a, b hcl.BlockHeaderSchema) int {
		return strings.Compare(a.Type, b.Type)
	})
}
