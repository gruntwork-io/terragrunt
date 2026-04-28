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
		// Locals evaluation seeds (cycles, multiple locals, types).
		`locals { a = "1"; b = "2"; c = "3" }; unit "x" { source = "."; path = "x" }`,
		`locals { a = local.b; b = local.a }; unit "x" { source = "."; path = "x" }`,
		`locals { x = 42 }; unit "x" { source = "."; path = "x" }`,
		`locals { flag = true }; unit "x" { source = "."; path = "x" }`,
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

// FuzzGenerateAutoIncludeFile tests the full generate pipeline with arbitrary stack HCL.
// Parses a stack file, resolves autoinclude, generates the output file.
// Verifies that the pipeline never panics regardless of input.
func FuzzGenerateAutoIncludeFile(f *testing.F) {
	seeds := []string{
		`unit "vpc" { source = "."; path = "vpc" }
unit "app" { source = "."; path = "app"
  autoinclude { dependency "vpc" { config_path = unit.vpc.path } inputs = { val = dependency.vpc.outputs.val } }
}`,
		`unit "a" { source = "."; path = "a" }
unit "b" { source = "."; path = "b"
  autoinclude { dependency "a" { config_path = unit.a.path; mock_outputs = { x = "y" } } }
}`,
		`locals { env = "prod" }
unit "vpc" { source = "."; path = "vpc" }
unit "app" { source = "."; path = "app"
  autoinclude { dependency "vpc" { config_path = unit.vpc.path } env_name = local.env }
}`,
		`unit "x" { source = "."; path = "x"; autoinclude {} }`,
		`unit "x" { source = "."; path = "x" }`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		srcBytes := []byte(input)
		fs := vfs.NewMemMapFS()

		result, err := hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
			Src:      srcBytes,
			Filename: "fuzz.hcl",
			StackDir: "/fuzz",
		})
		if err != nil || result == nil {
			return
		}

		for _, resolved := range result.AutoIncludes {
			_ = hclparse.GenerateAutoIncludeFile(fs, resolved, "/fuzz/.terragrunt-stack/out", srcBytes, resolved.EvalCtx)
		}
	})
}

// FuzzAutoIncludeDependencyPaths tests dependency path extraction from arbitrary autoinclude files.
// Verifies that AutoIncludeDependencyPaths never panics regardless of file content.
func FuzzAutoIncludeDependencyPaths(f *testing.F) {
	seeds := []string{
		`dependency "vpc" { config_path = "../vpc" }`,
		`dependency "db" { config_path = "/abs/path/db" }`,
		`dependency "a" { config_path = "../a" }
dependency "b" { config_path = "../b" }`,
		`inputs = { val = dependency.vpc.outputs.val }`,
		``,
		`dependency "x" {}`,
		`dependency "x" { config_path = 42 }`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		fs := vfs.NewMemMapFS()

		dir := "/fuzz/unit"
		_ = fs.MkdirAll(dir, 0755)
		_ = vfs.WriteFile(fs, dir+"/terragrunt.autoinclude.hcl", []byte(input), 0644)

		_, _ = hclparse.AutoIncludeDependencyPaths(fs, dir)
	})
}

// FuzzBuildComponentRefMap tests component ref map building with arbitrary names
// and paths at multiple nesting levels (2 deep).
// Verifies that BuildComponentRefMap never panics regardless of input.
func FuzzBuildComponentRefMap(f *testing.F) {
	f.Add("vpc", "/path/vpc", "", "", "", "")
	f.Add("infra", "/path/infra", "deep", "/path/deep", "db", "/path/db")
	f.Add("", "", "", "", "", "")
	f.Add("path", "/reserved", "name", "/also-reserved", "path", "/double-reserved")

	f.Fuzz(func(t *testing.T, name, path, childName, childPath, grandchildName, grandchildPath string) {
		ref := hclparse.ComponentRef{
			Name: name,
			Path: path,
		}

		if childName != "" {
			child := hclparse.ComponentRef{Name: childName, Path: childPath}

			if grandchildName != "" {
				child.ChildRefs = []hclparse.ComponentRef{
					{Name: grandchildName, Path: grandchildPath},
				}
			}

			ref.ChildRefs = []hclparse.ComponentRef{child}
		}

		_ = hclparse.BuildComponentRefMap([]hclparse.ComponentRef{ref})
	})
}

// FuzzNestedStackPath tests stack.<name>.<nested_stack>.path resolution with
// arbitrary stack and unit names. Creates a parent stack referencing a child
// stack that contains a nested stack, and verifies the pipeline never panics.
// Fuzz inputs are injected directly into HCL templates without escaping:
// this is intentional to test that invalid HCL is handled gracefully.
func FuzzNestedStackPath(f *testing.F) {
	f.Add("infra", "deep", "vpc", "db")
	f.Add("network", "storage", "subnet", "bucket")
	f.Add("a", "b", "c", "d")
	f.Add("", "", "", "")
	f.Add("path", "name", "source", "autoinclude")

	f.Fuzz(func(t *testing.T, stackName, nestedStackName, unitName, nestedUnitName string) {
		// Skip empty names: HCL labels can't be empty
		if stackName == "" || nestedStackName == "" || unitName == "" || nestedUnitName == "" {
			return
		}

		fs := vfs.NewMemMapFS()

		// Create the nested (deepest) stack file
		nestedStackDir := "/fuzz/stacks/nested"
		_ = fs.MkdirAll(nestedStackDir, 0755)

		nestedContent := `unit "` + nestedUnitName + `" { source = "."; path = "` + nestedUnitName + `" }`
		_ = vfs.WriteFile(fs, nestedStackDir+"/terragrunt.stack.hcl", []byte(nestedContent), 0644)

		// Create the middle stack that contains a unit + nested stack
		midStackDir := "/fuzz/stacks/mid"
		_ = fs.MkdirAll(midStackDir, 0755)

		midContent := `unit "` + unitName + `" { source = "."; path = "` + unitName + `" }
stack "` + nestedStackName + `" { source = "../nested"; path = "` + nestedStackName + `" }`
		_ = vfs.WriteFile(fs, midStackDir+"/terragrunt.stack.hcl", []byte(midContent), 0644)

		// Create parent stack referencing the middle stack + an app unit
		parentSrc := `stack "` + stackName + `" { source = "../stacks/mid"; path = "` + stackName + `" }
unit "app" { source = "."; path = "app"
  autoinclude {
    dependency "dep" { config_path = stack.` + stackName + `.` + nestedStackName + `.path }
  }
}`
		_, _ = hclparse.ParseStackFile(fs, &hclparse.ParseStackFileInput{
			Src:      []byte(parentSrc),
			Filename: "fuzz.hcl",
			StackDir: "/fuzz/live",
		})

		// Also exercise DiscoverStackChildUnits directly on the middle stack.
		_ = hclparse.DiscoverStackChildUnits(fs, "/fuzz/stacks/mid", "/fuzz/gen")
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
		_, _ = autoInclude.Resolve(evalCtx, nil)
	})
}
