package tflint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInputsToTflintVar(t *testing.T) {
	var inputs = map[string]interface{}{
		"region":         "eu-central-1",
		"instance_count": 3,
	}

	actual, err := inputsToTflintVar(inputs)

	assert.NoError(t, err)

	expected := []string{"--var=region=eu-central-1", "--var=instance_count=3"}

	assert.Equal(t, expected, actual)
}
