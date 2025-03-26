package util_test

import (
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsTerraformEnvVarJsonValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    any
		expected string
	}{
		{"aws_region", "aws_region"},
		{[]string{"10.0.0.0/16", "10.0.0.10/16"}, "[\"10.0.0.0/16\",\"10.0.0.10/16\"]"},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.AsTerraformEnvVarJSONValue(tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
