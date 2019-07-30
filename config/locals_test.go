package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/gocty"
)

func TestEvaluateLocalsBlock(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)
	mockFilename := "terragrunt.hcl"

	file, err := parseHcl(LocalsTestConfig, mockFilename)
	require.NoError(t, err)

	evaluatedLocals, err := evaluateLocalsBlock(terragruntOptions, file, mockFilename)
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
