package config

import (
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGraphCreation(t *testing.T) {
	config := `
locals {
	full-name = "${local.name}-${local.region}"
	name = "test"
	region = "us-east-1"
}
globals {
	region = local.region
}
include {
	path = "../${global.region}/terragrunt.hcl"
}
input = {
	region = global.region
}
`

	file, _ := parseHcl(hclparse.NewParser(), config, "terragrunt.hcl")
	values := getValuesFromHclFile(file)
	assert.Equal(t,"test-us-east-1", values["local"]["full-name"].AsString())
}
