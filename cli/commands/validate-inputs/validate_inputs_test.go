package validateinputs

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVarFlagsFromExtraArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		args             []string
		expectedVars     []string
		expectedVarFiles []string
	}{
		{
			"VarsWithQuotes",
			[]string{`-var='hello=world'`, `-var="foo=bar"`, `-var="'"enabled"'"=false`},
			[]string{"'enabled'", "foo", "hello"},
			[]string{},
		},
		{
			"VarFilesWithQuotes",
			[]string{`-var-file='terraform.tfvars'`, `-var-file="other_vars.tfvars"`},
			[]string{},
			[]string{"other_vars.tfvars", "terraform.tfvars"},
		},
		{
			"MixedWithOtherIrrelevantArgs",
			[]string{"-lock=true", "-var=enabled=true", "-refresh=false"},
			[]string{"enabled"},
			[]string{},
		},
		{
			"None",
			[]string{"-lock=true", "-refresh=false"},
			[]string{},
			[]string{},
		},
		{
			"SpaceInVarFileName",
			[]string{"-var-file='this is a test.tfvars'"},
			[]string{},
			[]string{"this is a test.tfvars"},
		},
	}

	for _, testCase := range testCases {
		// Capture range variable so that it is brought into the scope within the for loop, so that it is stable even
		// when subtests are run in parallel.
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			vars, varFiles, err := getVarFlagsFromArgList(testCase.args)
			require.NoError(t, err)
			sort.Strings(vars)
			sort.Strings(varFiles)
			assert.Equal(t, testCase.expectedVars, vars)
			assert.Equal(t, testCase.expectedVarFiles, varFiles)
		})
	}

}
