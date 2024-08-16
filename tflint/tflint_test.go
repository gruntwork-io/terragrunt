package tflint_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/tflint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputsToTflintVar(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		inputs   map[string]interface{}
		expected []string
	}{
		{
			"strings",
			map[string]interface{}{"region": "eu-central-1", "instance_count": 3},
			[]string{"--var=region=eu-central-1", "--var=instance_count=3"},
		},
		{
			"strings and arrays",
			map[string]interface{}{"cidr_blocks": []string{"10.0.0.0/16"}},
			[]string{"--var=cidr_blocks=[\"10.0.0.0/16\"]"},
		},
		{
			"boolean",
			map[string]interface{}{"create_resource": true},
			[]string{"--var=create_resource=true"},
		},
		{
			"with white spaces",
			// With white spaces, the string is still validated by tflint.
			map[string]interface{}{"region": " eu-central-1 "},
			[]string{"--var=region= eu-central-1 "},
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			actual, err := tflint.InputsToTflintVar(tt.inputs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, actual)
		})
	}
}
