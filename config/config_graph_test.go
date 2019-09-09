package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGraphCreation(t *testing.T) {
	filename := "../test/fixture-config-graph/one/two/three/" + DefaultTerragruntConfigPath

	child, parent, err := ParseConfigVariables(filename, terragruntOptionsForTest(t, filename))
	if err != nil {
		t.Error(err)
		return
	}

	assert.Equal(t,"test-us-east-1", child.Variables["local"].AsValueMap()["full-name"].AsString())
	assert.Equal(t,"../../../terragrunt.hcl-one/two/three", child.Variables["global"].AsValueMap()["source-postfix"].AsString())
	assert.Equal(t,"../../../terragrunt.hcl-one/two/three", parent.Variables["global"].AsValueMap()["source-postfix"].AsString())
}
