package validate_test

import (
	"sort"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/validate"
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vars, varFiles, err := validate.GetVarFlagsFromArgList(tc.args)
			require.NoError(t, err)
			sort.Strings(vars)
			sort.Strings(varFiles)
			assert.Equal(t, tc.expectedVars, vars)
			assert.Equal(t, tc.expectedVarFiles, varFiles)
		})
	}
}
