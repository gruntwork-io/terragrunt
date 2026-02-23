package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestEvaluateLocalsBlock(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)

	file, err := hclparse.NewParser().ParseFromString(LocalsTestConfig, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	ctx, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	evaluatedLocals, err := config.EvaluateLocalsBlock(ctx, pctx, logger.CreateLogger(), file)
	require.NoError(t, err)

	var actualRegion string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["region"], &actualRegion))
	assert.Equal(t, "us-east-1", actualRegion)

	var actualS3Url string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["s3_url"], &actualS3Url))
	assert.Equal(t, "com.amazonaws.us-east-1.s3", actualS3Url)

	var actualX float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["x"], &actualX))
	assert.InEpsilon(t, float64(1), actualX, 0.0000001)

	var actualY float64                                                    //codespell:ignore
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["y"], &actualY)) //codespell:ignore
	assert.InEpsilon(t, float64(2), actualY, 0.0000001)                    //codespell:ignore

	var actualZ float64
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["z"], &actualZ))
	assert.InEpsilon(t, float64(3), actualZ, 0.0000001)

	var actualFoo struct{ First Foo }
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["foo"], &actualFoo))
	assert.Equal(t, Foo{
		Region: "us-east-1",
		Foo:    "bar",
	}, actualFoo.First)

	var actualBar string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["bar"], &actualBar))
	assert.Equal(t, "us-east-1", actualBar)
}

func TestEvaluateLocalsBlockMultiDeepReference(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)

	file, err := hclparse.NewParser().ParseFromString(LocalsTestMultiDeepReferenceConfig, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	ctx, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	evaluatedLocals, err := config.EvaluateLocalsBlock(ctx, pctx, logger.CreateLogger(), file)
	require.NoError(t, err)

	expected := "a"

	var actualA string
	require.NoError(t, gocty.FromCtyValue(evaluatedLocals["a"], &actualA))
	assert.Equal(t, expected, actualA)

	testCases := []string{
		"b",
		"c",
		"d",
		"e",
		"f",
		"g",
		"h",
		"i",
		"j",
	}
	for _, tc := range testCases {
		expected = fmt.Sprintf("%s/%s", expected, tc)

		var actual string
		require.NoError(t, gocty.FromCtyValue(evaluatedLocals[tc], &actual))
		assert.Equal(t, expected, actual)
	}
}

func TestEvaluateLocalsBlockImpossibleWillFail(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)

	file, err := hclparse.NewParser().ParseFromString(LocalsTestImpossibleConfig, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	ctx, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	_, err = config.EvaluateLocalsBlock(ctx, pctx, logger.CreateLogger(), file)
	require.Error(t, err)

	switch errors.Unwrap(err).(type) { //nolint:errorlint
	case config.CouldNotEvaluateAllLocalsError:
	default:
		t.Fatalf("Did not get expected error: %s", err)
	}
}

func TestEvaluateLocalsBlockMultipleLocalsBlocksWillFail(t *testing.T) {
	t.Parallel()

	terragruntOptions := mockOptionsForTest(t)

	file, err := hclparse.NewParser().ParseFromString(MultipleLocalsBlockConfig, config.DefaultTerragruntConfigPath)
	require.NoError(t, err)

	ctx, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
	_, err = config.EvaluateLocalsBlock(ctx, pctx, logger.CreateLogger(), file)
	require.Error(t, err)
}

type Foo struct {
	Region string `cty:"region"`
	Foo    string `cty:"foo"`
}

const LocalsTestConfig = `
locals {
  region = "us-east-1"

  // Simple reference
  s3_url = "com.amazonaws.${local.region}.s3"

  // Nested reference
  foo = [
    merge(
      {region = local.region},
	  {foo = "bar"},
	)
  ]
  bar = local.foo[0]["region"]

  // Multiple references
  x = 1
  y = 2
  z = local.x + local.y
}
`

const LocalsTestMultiDeepReferenceConfig = `
# 10 chains deep
locals {
  a = "a"
  b = "${local.a}/b"
  c = "${local.b}/c"
  d = "${local.c}/d"
  e = "${local.d}/e"
  f = "${local.e}/f"
  g = "${local.f}/g"
  h = "${local.g}/h"
  i = "${local.h}/i"
  j = "${local.i}/j"
}
`

const LocalsTestImpossibleConfig = `
locals {
  a = local.b
  b = local.a
}
`

// TestEvaluateLocalsBlockTernaryOnlyRunsSelectedBranch verifies that a ternary
// expression in locals only executes the selected branch of run_cmd.
// The unselected branch uses a non-existent command: if the fix is broken and
// the unselected branch executes, run_cmd fails and EvaluateLocalsBlock returns
// an error, which causes the test to fail.
// Regression test for https://github.com/gruntwork-io/terragrunt/issues/1448
func TestEvaluateLocalsBlockTernaryOnlyRunsSelectedBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        string
		targetKey  string
		wantVal    string
		assertMode string // "string" (default), "tuple", "object"
		wantErr    bool
	}{
		{
			name: "direct ternary true condition runs only true branch",
			cfg: `
locals {
  condition = true
  result    = local.condition ? run_cmd("echo", "branch_true") : run_cmd("__nonexistent_terragrunt_test_command__")
}
`,
			targetKey: "result",
			wantVal:   "branch_true",
		},
		{
			name: "direct ternary false condition runs only false branch",
			cfg: `
locals {
  condition = false
  result    = local.condition ? run_cmd("__nonexistent_terragrunt_test_command__") : run_cmd("echo", "branch_false")
}
`,
			targetKey: "result",
			wantVal:   "branch_false",
		},
		{
			name: "ternary nested inside tuple runs only selected branch",
			cfg: `
locals {
  condition = true
  results   = [local.condition ? run_cmd("echo", "branch_true") : run_cmd("__nonexistent_terragrunt_test_command__")]
}
`,
			targetKey:  "results",
			wantVal:    "branch_true",
			assertMode: "tuple",
		},
		{
			name: "ternary nested inside object value runs only selected branch",
			cfg: `
locals {
  condition = true
  result    = {key = local.condition ? run_cmd("echo", "branch_true") : run_cmd("__nonexistent_terragrunt_test_command__")}
}
`,
			targetKey:  "result",
			wantVal:    "branch_true",
			assertMode: "object",
		},
		{
			name: "ternary nested inside function arg runs only selected branch",
			cfg: `
locals {
  condition = true
  result    = tostring(local.condition ? run_cmd("echo", "branch_true") : run_cmd("__nonexistent_terragrunt_test_command__"))
}
`,
			targetKey: "result",
			wantVal:   "branch_true",
		},
		{
			name: "ternary inside string interpolation runs only selected branch",
			cfg: `
locals {
  condition = true
  result    = "prefix-${local.condition ? run_cmd("echo", "val") : run_cmd("__nonexistent_terragrunt_test_command__")}-suffix"
}
`,
			targetKey: "result",
			wantVal:   "prefix-val-suffix",
		},
		{
			name: "nested ternary runs only selected inner branch",
			cfg: `
locals {
  cond_outer = true
  cond_inner = false
  result     = local.cond_outer ? (local.cond_inner ? run_cmd("__nonexistent_terragrunt_test_command__") : run_cmd("echo", "inner_false")) : run_cmd("__nonexistent_terragrunt_test_command__")
}
`,
			targetKey: "result",
			wantVal:   "inner_false",
		},
		{
			// Critical 2 regression: LiteralValueExpr substitution must not break try/can.
			// try() needs the original expression AST to catch errors internally.
			// If evalFunctionCallLazily substitutes args, try() errors instead of falling back.
			name: "try with failing run_cmd returns fallback not error",
			cfg: `
locals {
  result = try(run_cmd("__nonexistent_terragrunt_test_command__"), "fallback")
}
`,
			targetKey: "result",
			wantVal:   "fallback",
		},
		{
			// Critical 1 regression: args must not be evaluated before function existence check.
			// Before fix, run_cmd executes before the unknown-function error is raised.
			// After fix, HCL returns function-not-found without running run_cmd.
			name:    "unknown function does not execute run_cmd arg",
			wantErr: true,
			cfg: `
locals {
  result = __nonexistent_terragrunt_test_function__(run_cmd("__nonexistent_terragrunt_test_command__"))
}
`,
		},
		{
			// TemplateWrapExpr: bare "${...}" generates TemplateWrapExpr (not TemplateExpr).
			// Verifies the TemplateWrapExpr → evalExpressionLazily(e.Wrapped) delegation.
			name: "template wrap expr runs only selected branch",
			cfg: `
locals {
  condition = true
  result    = "${local.condition ? run_cmd("echo", "wrapped") : run_cmd("__nonexistent_terragrunt_test_command__")}"
}
`,
			targetKey: "result",
			wantVal:   "wrapped",
		},
		{
			// Multi-element tuple: verifies evalTupleConsLazily processes all elements
			// and nested ternaries in any position benefit from lazy eval.
			name: "multi-element tuple with ternary runs only selected branch",
			cfg: `
locals {
  condition = true
  results   = [run_cmd("echo", "first"), local.condition ? run_cmd("echo", "second") : run_cmd("__nonexistent_terragrunt_test_command__")]
}
`,
			targetKey:  "results",
			wantVal:    "second",
			assertMode: "multi_tuple",
		},
		{
			// can() uses the same custom expression decoder as try().
			// Verifies functionHasCustomArgDecoder correctly bypasses lazy eval for can().
			name: "can with failing run_cmd returns false not error",
			cfg: `
locals {
  result = can(run_cmd("__nonexistent_terragrunt_test_command__")) ? "reachable" : "unreachable"
}
`,
			targetKey: "result",
			wantVal:   "unreachable",
		},
		{
			// Condition is itself a ternary: verifies recursive lazy eval on the condition.
			name: "ternary condition is itself a ternary",
			cfg: `
locals {
  outer = true
  inner = true
  result = (local.outer ? local.inner : false) ? run_cmd("echo", "both_true") : run_cmd("__nonexistent_terragrunt_test_command__")
}
`,
			targetKey: "result",
			wantVal:   "both_true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			terragruntOptions := mockOptionsForTest(t)

			file, err := hclparse.NewParser().ParseFromString(tt.cfg, config.DefaultTerragruntConfigPath)
			require.NoError(t, err)

			ctx, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), terragruntOptions)
			evaluatedLocals, err := config.EvaluateLocalsBlock(ctx, pctx, logger.CreateLogger(), file)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "Call to unknown function",
					"expected function-not-found error, not command execution error")

				return
			}

			require.NoError(t, err)

			switch tt.assertMode {
			case "tuple":
				tupleVal := evaluatedLocals[tt.targetKey]
				elems := tupleVal.AsValueSlice()
				require.Len(t, elems, 1)

				var elem string
				require.NoError(t, gocty.FromCtyValue(elems[0], &elem))
				assert.Equal(t, tt.wantVal, elem)
			case "multi_tuple":
				tupleVal := evaluatedLocals[tt.targetKey]
				elems := tupleVal.AsValueSlice()
				require.GreaterOrEqual(t, len(elems), 2)

				var lastElem string
				require.NoError(t, gocty.FromCtyValue(elems[len(elems)-1], &lastElem))
				assert.Equal(t, tt.wantVal, lastElem)
			case "object":
				resultObj := evaluatedLocals[tt.targetKey]
				require.True(t, resultObj.Type().IsObjectType())

				var keyStr string
				require.NoError(t, gocty.FromCtyValue(resultObj.GetAttr("key"), &keyStr))
				assert.Equal(t, tt.wantVal, keyStr)
			case "string", "":
				var result string
				require.NoError(t, gocty.FromCtyValue(evaluatedLocals[tt.targetKey], &result))
				assert.Equal(t, tt.wantVal, result)
			default:
				t.Fatalf("unknown assertMode: %q", tt.assertMode)
			}
		})
	}
}

const MultipleLocalsBlockConfig = `
locals {
  a = "a"
}

locals {
  b = "b"
}
`
