package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAsTerraformEnvVarJsonValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    interface{}
		expected string
	}{
		{"aws_region", "aws_region"},
		{[]string{"10.0.0.0/16", "10.0.0.10/16"}, "[\"10.0.0.0/16\",\"10.0.0.10/16\"]"},
	}

	for _, testCase := range testCases {
		actual, err := AsTerraformEnvVarJsonValue(testCase.value)
		assert.NoError(t, err)
		assert.Equal(t, testCase.expected, actual)
	}
}
