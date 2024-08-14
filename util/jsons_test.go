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

	tc := []struct {
		value    interface{}
		expected string
	}{
		{"aws_region", "aws_region"},
		{[]string{"10.0.0.0/16", "10.0.0.10/16"}, "[\"10.0.0.0/16\",\"10.0.0.10/16\"]"},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.AsTerraformEnvVarJsonValue(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
