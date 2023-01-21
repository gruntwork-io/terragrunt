package tflint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInputsToTflintVar(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		inputs   map[string]interface{}
		expected []string
	}{
		{
			map[string]interface{}{"region": "eu-central-1", "instance_count": 3},
			[]string{"--var=region=eu-central-1", "--var=instance_count=3"},
		},
		{
			map[string]interface{}{"cidr_blocks": []string{"10.0.0.0/16"}},
			[]string{"--var=cidr_blocks=[\"10.0.0.0/16\"]"},
		},
		{
			map[string]interface{}{"create_resource": true},
			[]string{"--var=create_resource=true"},
		},
		{
			// With white spaces, the string is still validated by tflint.
			map[string]interface{}{"region": " eu-central-1 "},
			[]string{"--var=region= eu-central-1 "},
		},
	}

	for _, testCase := range testCases {
		actual, err := inputsToTflintVar(testCase.inputs)
		assert.NoError(t, err)
		assert.ElementsMatch(t, testCase.expected, actual)
	}
}
