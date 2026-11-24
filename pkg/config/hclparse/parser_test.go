package hclparse_test

import (
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRejectsOutOfRangeNumbers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
	}{
		{name: "attribute", content: `x = 1e110210270`},
		{name: "negative exponent", content: `x = 1e-110210270`},
		{name: "template", content: `x = "${1e110210270}"`},
		{name: "function argument", content: `x = toset(["web", 1e110210270, "api"])`},
		{name: "nested block", content: "locals {\n  x = [1, 1e110210270]\n}"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := hclparse.NewParser().ParseFromString(tc.content, "terragrunt.hcl")
			assertNumberOutOfRange(t, err)
		})
	}
}

// pins that a second parse of the same path, which HCL serves from its file
// cache without the original diagnostics, still rejects the number
func TestParseRejectsOutOfRangeNumbersOnReparse(t *testing.T) {
	t.Parallel()

	parser := hclparse.NewParser()

	for range 2 {
		_, err := parser.ParseFromString(`x = 1e110210270`, "terragrunt.hcl")
		assertNumberOutOfRange(t, err)
	}
}

func TestParseAllowsInRangeNumbers(t *testing.T) {
	t.Parallel()

	_, err := hclparse.NewParser().
		ParseFromString("x = 1e4000\ny = -1e-4000\nz = 42", "terragrunt.hcl")
	require.NoError(t, err)
}

func TestParseRecordsTheFunctionsAFileCalls(t *testing.T) {
	t.Parallel()

	const content = `locals {
  name = upper(lower("A"))
  env  = "${get_env("HOME")}"
  dirs = [for d in fileset(".", "*") : basename(d)]
}`

	file, err := hclparse.NewParser().ParseFromString(content, "terragrunt.hcl")
	require.NoError(t, err)

	want := []string{"basename", "fileset", "get_env", "lower", "upper"}

	for _, kept := range []*hclparse.File{file, file.WithoutParser(), file.WithoutParser().Rebind(hclparse.NewParser())} {
		require.True(t, kept.FunctionCalls().Known())
		assert.Equal(t, want, slices.Sorted(kept.FunctionCalls().All))
		assert.True(t, kept.FunctionCalls().Calls("get_env"))
		assert.False(t, kept.FunctionCalls().Calls("get_terragrunt_dir"))
	}
}

func TestParseOfJSONMayCallAnyFunction(t *testing.T) {
	t.Parallel()

	file, err := hclparse.NewParser().
		ParseFromString(`{"locals": {"env": "${get_env(\"HOME\")}"}}`, "terragrunt.hcl.json")
	require.NoError(t, err)

	assert.False(t, file.FunctionCalls().Known())
	assert.True(t, file.FunctionCalls().Calls("get_terragrunt_dir"))
}

func assertNumberOutOfRange(t *testing.T, err error) {
	t.Helper()

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	assert.Equal(t, "Number out of range", diags[0].Summary)
}
