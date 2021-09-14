package config

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/gruntwork-io/terragrunt/errors"
)

func TestEvaluateLocalsBlock(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	parser := hclparse.NewParser()
	file, err := parseHcl(parser, LocalsTestConfig, mockFilename)
	require.NoError(t, err)

	evaluatedLocals, err := evaluateLocalsBlock(terragruntOptions, parser, file, mockFilename, nil, nil)
	require.NoError(t, err)

	var actualRegion string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["region"], &actualRegion))
	assert.Equal(t, actualRegion, "us-east-1")

	var actualS3Url string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["s3_url"], &actualS3Url))
	assert.Equal(t, actualS3Url, "com.amazonaws.us-east-1.s3")

	var actualX float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["x"], &actualX))
	assert.Equal(t, actualX, float64(1))

	var actualY float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["y"], &actualY))
	assert.Equal(t, actualY, float64(2))

	var actualZ float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["z"], &actualZ))
	assert.Equal(t, actualZ, float64(3))

	var actualFoo struct{ First Foo }
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["foo"], &actualFoo))
	assert.Equal(t, actualFoo.First, Foo{
		Region: "us-east-1",
		Foo:    "bar",
	})

	var actualBar string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["bar"], &actualBar))
	assert.Equal(t, actualBar, "us-east-1")
}

func TestEvaluateLocalsBlockMultiDeepReference(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	parser := hclparse.NewParser()
	file, err := parseHcl(parser, LocalsTestMultiDeepReferenceConfig, mockFilename)
	require.NoError(t, err)

	evaluatedLocals, err := evaluateLocalsBlock(terragruntOptions, parser, file, mockFilename, nil, nil)
	require.NoError(t, err)

	expected := "a"

	var actualA string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["a"], &actualA))
	assert.Equal(t, actualA, expected)

	testCases := []string{
		"b",
		"c",
		"d",
		"e",
		"f",
		"g",
		"h",
		"i",
		"j",
	}
	for _, testCase := range testCases {
		expected = fmt.Sprintf("%s/%s", expected, testCase)

		var actual string
		require.NoError(t, gocty.FromCtyValue(evaluatedLocals[testCase], &actual))
		assert.Equal(t, actual, expected)
	}
}

func TestEvaluateLocalsBlockImpossibleWillFail(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	parser := hclparse.NewParser()
	file, err := parseHcl(parser, LocalsTestImpossibleConfig, mockFilename)
	require.NoError(t, err)

	_, err = evaluateLocalsBlock(terragruntOptions, parser, file, mockFilename, nil, nil)
	require.Error(t, err)

	switch errors.Unwrap(err).(type) {
	case CouldNotEvaluateAllLocalsError:
	default:
		t.Fatalf("Did not get expected error: %s", err)
	}
}

func TestEvaluateLocalsBlockMultipleLocalsBlocksWillFail(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	parser := hclparse.NewParser()
	file, err := parseHcl(parser, MultipleLocalsBlockConfig, mockFilename)
	require.NoError(t, err)

	_, err = evaluateLocalsBlock(terragruntOptions, parser, file, mockFilename, nil, nil)
	require.Error(t, err)
}

type Foo struct {
	Region string `cty:"region"`
	Foo    string `cty:"foo"`
}

const LocalsTestConfig = `
locals {
  region = "us-east-1"

  // Simple reference
  s3_url = "com.amazonaws.${local.region}.s3"

  // Nested reference
  foo = [
    merge(
      {region = local.region},
	  {foo = "bar"},
	)
  ]
  bar = local.foo[0]["region"]

  // Multiple references
  x = 1
  y = 2
  z = local.x + local.y
}
`

const LocalsTestMultiDeepReferenceConfig = `
# 10 chains deep
locals {
  a = "a"
  b = "${local.a}/b"
  c = "${local.b}/c"
  d = "${local.c}/d"
  e = "${local.d}/e"
  f = "${local.e}/f"
  g = "${local.f}/g"
  h = "${local.g}/h"
  i = "${local.h}/i"
  j = "${local.i}/j"
}
`

const LocalsTestImpossibleConfig = `
locals {
  a = local.b
  b = local.a
}
`

const MultipleLocalsBlockConfig = `
locals {
  a = "a"
}

locals {
  b = "b"
}
`
