package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestEvaluateLocalsBlock(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	file, err := hclparse.NewParser().ParseFromString(LocalsTestConfig, mockFilename)
	require.NoError(t, err)

	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	evaluatedLocals, err := config.EvaluateLocalsBlock(ctx, logger.CreateLogger(), file)
	require.NoError(t, err)

	var actualRegion string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["region"], &actualRegion))
	assert.Equal(t, "us-east-1", actualRegion)

	var actualS3Url string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["s3_url"], &actualS3Url))
	assert.Equal(t, "com.amazonaws.us-east-1.s3", actualS3Url)

	var actualX float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["x"], &actualX))
	assert.InEpsilon(t, float64(1), actualX, 0.0000001)

	var actualY float64                                                    //codespell:ignore
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["y"], &actualY)) //codespell:ignore
	assert.InEpsilon(t, float64(2), actualY, 0.0000001)                    //codespell:ignore

	var actualZ float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["z"], &actualZ))
	assert.InEpsilon(t, float64(3), actualZ, 0.0000001)

	var actualFoo struct{ First Foo }
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["foo"], &actualFoo))
	assert.Equal(t, Foo{
		Region: "us-east-1",
		Foo:    "bar",
	}, actualFoo.First)

	var actualBar string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["bar"], &actualBar))
	assert.Equal(t, "us-east-1", actualBar)
}

func TestEvaluateLocalsBlockMultiDeepReference(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	file, err := hclparse.NewParser().ParseFromString(LocalsTestMultiDeepReferenceConfig, mockFilename)
	require.NoError(t, err)

	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	evaluatedLocals, err := config.EvaluateLocalsBlock(ctx, logger.CreateLogger(), file)
	require.NoError(t, err)

	expected := "a"

	var actualA string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["a"], &actualA))
	assert.Equal(t, expected, actualA)

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
	for _, tc := range testCases {
		expected = fmt.Sprintf("%s/%s", expected, tc)

		var actual string
		require.NoError(t, gocty.FromCtyValue(evaluatedLocals[tc], &actual))
		assert.Equal(t, expected, actual)
	}
}

func TestEvaluateLocalsBlockImpossibleWillFail(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	file, err := hclparse.NewParser().ParseFromString(LocalsTestImpossibleConfig, mockFilename)
	require.NoError(t, err)

	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	_, err = config.EvaluateLocalsBlock(ctx, logger.CreateLogger(), file)
	require.Error(t, err)

	switch errors.Unwrap(err).(type) { //nolint:errorlint
	case config.CouldNotEvaluateAllLocalsError:
	default:
		t.Fatalf("Did not get expected error: %s", err)
	}
}

func TestEvaluateLocalsBlockMultipleLocalsBlocksWillFail(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	file, err := hclparse.NewParser().ParseFromString(MultipleLocalsBlockConfig, mockFilename)
	require.NoError(t, err)

	ctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	_, err = config.EvaluateLocalsBlock(ctx, logger.CreateLogger(), file)
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
