package config_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestUpdateUnknownCtyValValues(t *testing.T) {
	t.Parallel()

	tc := []struct {
		value         cty.Value
		expectedValue cty.Value
	}{
		{
			value: cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("foo"),
					"lastname":  cty.UnknownVal(cty.String),
				})}),
			})}),
			expectedValue: cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("foo"),
					"lastname":  cty.StringVal(""),
				})}),
			})}),
		},
		{
			value:         cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
			expectedValue: cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
		},
		{
			value:         cty.ObjectVal(map[string]cty.Value{}),
			expectedValue: cty.ObjectVal(map[string]cty.Value{}),
		},
		{
			value:         cty.ObjectVal(map[string]cty.Value{"key": cty.UnknownVal(cty.String)}),
			expectedValue: cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("")}),
		},
	}

	for i, tt := range tc {
		testCase := tt

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualValue, err := config.UpdateUnknownCtyValValues(testCase.value)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedValue, actualValue)
		})
	}
}
