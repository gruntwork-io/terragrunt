// +build linux darwin

package shell

import (
	"bufio"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestCommandOutputOrder(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)
	terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "same")
	out, err := RunShellCommandWithOutput(terragruntOptions, "../testdata/test_outputs.sh", "same")

	assert.NotNil(t, out, "Should get output")
	assert.Nil(t, err, "Should have no error")

	scanner := bufio.NewScanner(strings.NewReader(out))
	var outputs = []string{}
	for scanner.Scan() {
		outputs = append(outputs, scanner.Text())
	}

	assert.True(t, len(outputs) == 5, "Should have 5 entries")
	assert.Equal(t, "stdout1", outputs[0], "First one from stdout")
	assert.Equal(t, "stderr1", outputs[1], "First one from stderr")
	assert.Equal(t, "stdout2", outputs[2], "Second one from stdout")
	assert.Equal(t, "stderr2", outputs[3], "Second one from stderr")
	assert.Equal(t, "stderr3", outputs[4], "Third one from stderr")
}
