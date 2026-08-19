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
			description: "string and untyped variables are read literally",
			files: map[string]string{
				"main.tf": `
variable "str" {
  type = string
}

variable "untyped" {}
`,
			},
			expected: map[string]tf.ModuleVariable{
				"str":     {ParsingMode: tf.VariableParseLiteral},
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

variable "num" {
  type = number
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
				"num":      {ParsingMode: tf.VariableParseHCL},
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
				"required": {},
				"optional": {HasDefault: true},
				"nullable": {HasDefault: true},
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
