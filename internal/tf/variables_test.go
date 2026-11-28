package tf_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleVariables(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files       map[string]string
		expected    map[string]tf.ModuleVariable
		description string
	}{
		{
			description: "no files",
			files:       map[string]string{},
			expected:    map[string]tf.ModuleVariable{},
		},
		{
			description: "primitive and untyped variables are read literally",
			files: map[string]string{
				"main.tf": `
variable "str" {
  type = string
}

variable "num" {
  type = number
}

variable "flag" {
  type = bool
}

variable "untyped" {}
`,
			},
			expected: map[string]tf.ModuleVariable{
				"str":     {ParsingMode: tf.VariableParseLiteral},
				"num":     {ParsingMode: tf.VariableParseLiteral},
				"flag":    {ParsingMode: tf.VariableParseLiteral},
				"untyped": {ParsingMode: tf.VariableParseLiteral},
			},
		},
		{
			description: "every other type constraint is parsed as HCL",
			files: map[string]string{
				"main.tf": `
variable "anything" {
  type = any
}

variable "strs" {
  type = list(string)
}

variable "obj" {
  type = object({
    name    = string
    enabled = optional(bool, false)
  })
}
`,
			},
			expected: map[string]tf.ModuleVariable{
				"anything": {ParsingMode: tf.VariableParseHCL},
				"strs":     {ParsingMode: tf.VariableParseHCL},
				"obj":      {ParsingMode: tf.VariableParseHCL},
			},
		},
		{
			description: "variables with a default are optional",
			files: map[string]string{
				"main.tf": `
variable "required" {
  type = string
}

variable "optional" {
  type    = string
  default = "value"
}

variable "nullable" {
  default = null
}
`,
			},
			expected: map[string]tf.ModuleVariable{
				"required": {ParsingMode: tf.VariableParseLiteral},
				"optional": {ParsingMode: tf.VariableParseLiteral, HasDefault: true},
				"nullable": {ParsingMode: tf.VariableParseLiteral, HasDefault: true},
			},
		},
		{
			description: "an override that sets only a default keeps the type constraint",
			files: map[string]string{
				"main.tf":     `variable "rules" { type = map(string) }`,
				"override.tf": `variable "rules" { default = {} }`,
			},
			expected: map[string]tf.ModuleVariable{
				"rules": {ParsingMode: tf.VariableParseHCL, HasDefault: true},
			},
		},
		{
			description: "an override that sets only a type constraint keeps the default",
			files: map[string]string{
				"main.tf": `
variable "to_hcl" {
  type    = string
  default = "value"
}

variable "to_literal" {
  type    = map(string)
  default = {}
}
`,
				"override.tf": `
variable "to_hcl" {
  type = any
}

variable "to_literal" {
  type = string
}
`,
			},
			expected: map[string]tf.ModuleVariable{
				"to_hcl":     {ParsingMode: tf.VariableParseHCL, HasDefault: true},
				"to_literal": {ParsingMode: tf.VariableParseLiteral, HasDefault: true},
			},
		},
		{
			description: "an override file applies after files that sort later",
			files: map[string]string{
				"a_override.tf": `variable "rules" { type = string }`,
				"main.tf":       `variable "rules" { type = map(string) }`,
			},
			expected: map[string]tf.ModuleVariable{
				"rules": {ParsingMode: tf.VariableParseLiteral},
			},
		},
		{
			description: "override files apply in name order",
			files: map[string]string{
				"main.tf":       `variable "rules" { type = string }`,
				"a_override.tf": `variable "rules" { type = map(string) }`,
				"b_override.tf": `variable "rules" { type = bool }`,
			},
			expected: map[string]tf.ModuleVariable{
				"rules": {ParsingMode: tf.VariableParseLiteral},
			},
		},
		{
			description: "override files are recognized in every configuration file extension",
			files: map[string]string{
				"main.tf": `
variable "tf" { type = map(string) }
variable "tofu" { type = map(string) }
variable "tf_json" { type = map(string) }
variable "tofu_json" { type = map(string) }
variable "named" { type = map(string) }
`,
				"override.tf":             `variable "tf" { default = {} }`,
				"override.tofu":           `variable "tofu" { default = {} }`,
				"override.tf.json":        `{"variable": {"tf_json": {"default": {}}}}`,
				"override.tofu.json":      `{"variable": {"tofu_json": {"default": {}}}}`,
				"defaults_override.tf":    `variable "named" { default = {} }`,
				"unrelated_override.tofu": `variable "named" { default = {} }`,
			},
			expected: map[string]tf.ModuleVariable{
				"tf":        {ParsingMode: tf.VariableParseHCL, HasDefault: true},
				"tofu":      {ParsingMode: tf.VariableParseHCL, HasDefault: true},
				"tf_json":   {ParsingMode: tf.VariableParseHCL, HasDefault: true},
				"tofu_json": {ParsingMode: tf.VariableParseHCL, HasDefault: true},
				"named":     {ParsingMode: tf.VariableParseHCL, HasDefault: true},
			},
		},
		{
			description: "a file name that only contains override is not an override file",
			files: map[string]string{
				"overrides.tf":     `variable "plural" { default = {} }`,
				"override_vars.tf": `variable "prefixed" { default = {} }`,
			},
			expected: map[string]tf.ModuleVariable{
				"plural":   {ParsingMode: tf.VariableParseLiteral, HasDefault: true},
				"prefixed": {ParsingMode: tf.VariableParseLiteral, HasDefault: true},
			},
		},
		{
			description: "declarations are collected across files, including JSON syntax",
			files: map[string]string{
				"vars.tofu":    `variable "from_tofu" { type = map(string) }`,
				"vars.tf.json": `{"variable": {"from_json": {"type": "string"}, "json_list": {"type": "list(string)"}}}`,
			},
			expected: map[string]tf.ModuleVariable{
				"from_tofu": {ParsingMode: tf.VariableParseHCL},
				"from_json": {ParsingMode: tf.VariableParseLiteral},
				"json_list": {ParsingMode: tf.VariableParseHCL},
			},
		},
		{
			description: "files that are not module sources are ignored",
			files: map[string]string{
				"main.tf":                      `variable "str" { type = string }`,
				"terragrunt-debug.tfvars.json": `{"str": "value"}`,
			},
			expected: map[string]tf.ModuleVariable{
				"str": {ParsingMode: tf.VariableParseLiteral},
			},
		},
		{
			// Regression test for https://github.com/gruntwork-io/terragrunt/issues/3425
			description: "provider function syntax is parsed",
			files: map[string]string{
				"main.tf": `
data "azurerm_subnet" "main" {
  name = var.subnet.name

  lifecycle {
    postcondition {
      condition     = anytrue([for prefix in self.address_prefixes : provider::assert::cidrv6(prefix)])
      error_message = "Subnet does not contain a valid IPv6 CIDR."
    }
  }
}

variable "subnet" {
  type = object({
    name        = string
    enable_ipv6 = optional(bool, false)
  })
}
`,
			},
			expected: map[string]tf.ModuleVariable{
				"subnet": {ParsingMode: tf.VariableParseHCL},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			modulePath := "/module"

			require.NoError(t, fsys.MkdirAll(modulePath, 0755))

			for filename, content := range tc.files {
				path := filepath.Join(modulePath, filename)
				require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0644))
			}

			declared, err := tf.ModuleVariables(fsys, modulePath)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, declared)
		})
	}
}

func TestModuleVariablesUnparseableModule(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	modulePath := "/module"

	require.NoError(t, fsys.MkdirAll(modulePath, 0755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(modulePath, "main.tf"), []byte(`variable "str" {`), 0644),
	)

	_, err := tf.ModuleVariables(fsys, modulePath)
	require.Error(t, err)
}
