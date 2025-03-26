package ctyhelper_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
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
					"firstname": cty.StringVal("foo"),
					"lastname":  cty.UnknownVal(cty.String),
				})}),
			})}),
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
				"items": cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{
					"firstname": cty.StringVal("foo"),
					"lastname":  cty.StringVal(""),
				})}),
			})}),
		},
		{
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
			cty.ListVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{})}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{}),
			cty.ObjectVal(map[string]cty.Value{}),
		},
		{
			cty.ObjectVal(map[string]cty.Value{"key": cty.UnknownVal(cty.String)}),
			cty.ObjectVal(map[string]cty.Value{"key": cty.StringVal("")}),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualValue, err := ctyhelper.UpdateUnknownCtyValValues(tc.value)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedValue, actualValue)
		})
	}
}
