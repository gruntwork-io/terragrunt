package tflint_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/tflint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputsToTflintVar(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		inputs   map[string]any
		expected []string
	}{
		{
			"strings",
			map[string]any{"region": "eu-central-1", "instance_count": 3},
			[]string{"--var=region=eu-central-1", "--var=instance_count=3"},
		},
		{
			"strings and arrays",
			map[string]any{"cidr_blocks": []string{"10.0.0.0/16"}},
			[]string{"--var=cidr_blocks=[\"10.0.0.0/16\"]"},
		},
		{
			"boolean",
			map[string]any{"create_resource": true},
			[]string{"--var=create_resource=true"},
		},
		{
			"with white spaces",
			// With white spaces, the string is still validated by tflint.
			map[string]any{"region": " eu-central-1 "},
			[]string{"--var=region= eu-central-1 "},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual, err := tflint.InputsToTflintVar(tc.inputs)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.expected, actual)
		})
	}
}
