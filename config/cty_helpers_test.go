package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestUpdateUnknownCtyValValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value         cty.Value
		expectedValue cty.Value
	}{
		{
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("joo"),
					"lastname":  cty.StringVal("bob"),
				})}),
			})}),
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("joo"),
					"lastname":  cty.StringVal("bob"),
				})}),
			})}),
		},
		{
			cty.ListVal([]cty.Value{cty.ObjectVal(make(map[string]cty.Value))}),
			cty.ListVal([]cty.Value{cty.ObjectVal(make(map[string]cty.Value))}),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualValue, err := updateUnknownCtyValValues(testCase.value)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedValue, actualValue)
		})
	}
}
