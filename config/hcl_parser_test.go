package config

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
)

func TestDecodeHclEmptyInEmptyOut(t *testing.T) {
	t.Parallel()

	testFile := hcl.File{
		Body: MockHclBody{},
	}
	testFilename := "my-test.hcl"
	testOut := make(map[string]string)
	testTerragruntOptions := options.TerragruntOptions{}
	testExtensions := EvalContextExtensions{}

	actualResult := decodeHcl(&testFile, testFilename, &testOut, &testTerragruntOptions, testExtensions)

	assert.Nil(t, actualResult)
	assert.NoError(t, actualResult)
}
