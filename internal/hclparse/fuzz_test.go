package hclparse_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// FuzzParseStackFile tests ParseStackFile with arbitrary HCL input.
// Verifies that the two-pass parser never panics regardless of input.
func FuzzParseStackFile(f *testing.F) {
	seeds := []string{
		`unit "vpc" { source = "../units/vpc"; path = "vpc" }`,
		`unit "app" { source = "../units/app"; path = "app"; autoinclude { dependency "vpc" { config_path = unit.vpc.path } } }`,
		`stack "infra" { source = "../stacks/infra"; path = "infra" }`,
		`locals { env = "prod" }; unit "app" { source = "."; path = "app" }`,
		`unit "x" { source = "."; path = "x"; no_dot_terragrunt_stack = true }`,
		`include "extra" { path = "./extra.hcl" }`,
		``,
		`{}`,
		`unit`,
		`unit "x" {}`,
		`unit "x" { source = "." }`,
		`unit "x" { path = "x" }`,
		`unit "x" { source = "."; path = "x"; autoinclude {} }`,
		`unit "x" { source = "."; path = "x"; autoinclude { inputs = { a = 1 } } }`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		fs := vfs.NewMemMapFS()

		_, _ = hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
			Src:      []byte(input),
			Filename: "fuzz.hcl",
			StackDir: "/fuzz",
		})
	})
}

// FuzzPartialEval tests the partial evaluator with arbitrary HCL expressions.
// Verifies that PartialEval never panics regardless of expression input.
func FuzzPartialEval(f *testing.F) {
	seeds := []string{
		`val = "hello"`,
		`val = 42`,
		`val = true`,
		`val = local.env`,
		`val = dependency.vpc.outputs.id`,
		`val = "${local.env}-${dependency.vpc.outputs.id}"`,
		`val = { a = local.env, b = dependency.vpc.outputs.id }`,
		`val = [local.env, dependency.vpc.outputs.id]`,
		`val = local.flag ? local.env : "default"`,
		`val = (local.env)`,
		`val = try(dependency.vpc.outputs.id, "fallback")`,
		`val = merge(local.tags, { id = dependency.vpc.outputs.id })`,
		`val = local.count + dependency.vpc.outputs.extra`,
		`val = [for k, v in dependency.vpc.outputs.tags : v]`,
		``,
		`val = null`,
		`val = {}`,
		`val = []`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"local": cty.ObjectVal(map[string]cty.Value{
				"env":   cty.StringVal("production"),
				"flag":  cty.True,
				"count": cty.NumberIntVal(3),
				"tags":  cty.EmptyObjectVal,
			}),
		},
	}

	deferred := map[string]bool{"dependency": true}

	f.Fuzz(func(t *testing.T, input string) {
		srcBytes := []byte(input)

		file, diags := hclsyntax.ParseConfig(srcBytes, "fuzz.hcl", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return
		}

		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return
		}

		for _, attr := range body.Attributes {
			_ = hclparse.PartialEval(attr.Expr, &hclparse.EvalArgs{
				EvalCtx:  evalCtx,
				Deferred: deferred,
				SrcBytes: srcBytes,
			})
		}
	})
}

// FuzzAutoIncludeResolve tests AutoIncludeHCL.Resolve with arbitrary HCL input.
// Verifies that resolution never panics regardless of autoinclude body content.
func FuzzAutoIncludeResolve(f *testing.F) {
	seeds := []string{
		`dependency "vpc" { config_path = "../vpc" }`,
		`dependency "db" { config_path = "../db"; mock_outputs = { id = "mock" } }`,
		`inputs = { val = dependency.vpc.outputs.val }`,
		`dependency "vpc" { config_path = "../vpc" }; inputs = { val = dependency.vpc.outputs.val }`,
		``,
		`{}`,
		`dependency "x" {}`,
		`retry "err" { retryable_errors = [".*"]; max_attempts = 3 }`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{
					"path": cty.StringVal("../vpc"),
					"name": cty.StringVal("vpc"),
				}),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	f.Fuzz(func(t *testing.T, input string) {
		srcBytes := []byte(input)

		file, diags := hclsyntax.ParseConfig(srcBytes, "fuzz.hcl", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return
		}

		autoInclude := &hclparse.AutoIncludeHCL{Remain: file.Body}
		_, _ = autoInclude.Resolve(evalCtx)
	})
}
