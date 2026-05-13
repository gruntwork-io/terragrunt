package hclparse_test

import (
	"errors"
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
	f.Add("path", "name", "source", "autoinclude")

	f.Fuzz(func(t *testing.T, stackName, nestedStackName, unitName, nestedUnitName string) {
		if stackName == "" || nestedStackName == "" || unitName == "" || nestedUnitName == "" {
			t.Skip("HCL labels cannot be empty")
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
		_, _ = autoInclude.Resolve(evalCtx)
	})
}

// FuzzParseStackFileFromPath_ArgPanics fuzzes stackDir to probe panic paths
// and inner behavior. Empty strings must panic; any non-empty value must not
// panic (it may return an error — that's fine).
func FuzzParseStackFileFromPath_ArgPanics(f *testing.F) {
	f.Add("/project")
	f.Add("")
	f.Add("relative/path")
	f.Add("/")
	f.Add(".")
	f.Add("\x00")
	f.Add("\n")
	f.Add("../..")
	f.Add("/very/deep/nested/path/that/does/not/exist")

	f.Fuzz(func(t *testing.T, stackDir string) {
		fs := vfs.NewMemMapFS()

		defer func() {
			r := recover()

			switch {
			case stackDir == "" && r == nil:
				t.Errorf("expected panic for empty stackDir, got none")
			case stackDir != "" && r != nil:
				t.Errorf("unexpected panic for stackDir=%q: %v", stackDir, r)
			}
		}()

		_, _ = hclparse.ParseStackFileFromPath(fs, stackDir)
	})
}

// FuzzUnitPathsFromStackDir_ArgPanics mirrors the above for UnitPathsFromStackDir.
func FuzzUnitPathsFromStackDir_ArgPanics(f *testing.F) {
	f.Add("/project")
	f.Add("")
	f.Add("relative/path")
	f.Add("/")
	f.Add("\x00")
	f.Add("unicode/café")

	f.Fuzz(func(t *testing.T, stackDir string) {
		fs := vfs.NewMemMapFS()

		defer func() {
			r := recover()

			switch {
			case stackDir == "" && r == nil:
				t.Errorf("expected panic for empty stackDir, got none")
			case stackDir != "" && r != nil:
				t.Errorf("unexpected panic for stackDir=%q: %v", stackDir, r)
			}
		}()

		_, _ = hclparse.UnitPathsFromStackDir(fs, stackDir)
	})
}

// FuzzAutoIncludeDependencyPaths_ArgErrors fuzzes unitDir plus arbitrary file
// contents written to the in-memory FS, exercising both the argument-validation
// error and the HCL parsing path. Empty unitDir must return EmptyArgError;
// non-empty unitDir must not panic regardless of file content.
func FuzzAutoIncludeDependencyPaths_ArgErrors(f *testing.F) {
	f.Add("/unit", `dependency "vpc" { config_path = "../vpc" }`)
	f.Add("", `dependency "x" {}`)
	f.Add("/unit", ``)
	f.Add("/unit", `dependency "x" { config_path = 42 }`)
	f.Add("relative", `{}`)
	f.Add("/unit", `not even hcl!@#$`)

	f.Fuzz(func(t *testing.T, unitDir, content string) {
		fs := vfs.NewMemMapFS()

		if unitDir != "" {
			_ = fs.MkdirAll(unitDir, 0755)
			_ = vfs.WriteFile(fs, unitDir+"/"+hclparse.AutoIncludeFile, []byte(content), 0644)
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("unexpected panic for unitDir=%q: %v", unitDir, r)
			}
		}()

		_, err := hclparse.AutoIncludeDependencyPaths(fs, unitDir)

		if unitDir == "" {
			var emptyErr hclparse.EmptyArgError
			if !errors.As(err, &emptyErr) {
				t.Errorf("expected EmptyArgError for empty unitDir, got %v", err)
			}
		}
	})
}

// FuzzDiscoverStackChildUnits_ArgPanics fuzzes both string args.
func FuzzDiscoverStackChildUnits_ArgPanics(f *testing.F) {
	f.Add("/src", "/gen")
	f.Add("", "/gen")
	f.Add("/src", "")
	f.Add("", "")
	f.Add("\x00", "\x00")

	f.Fuzz(func(t *testing.T, stackSourceDir, stackGenDir string) {
		fs := vfs.NewMemMapFS()

		defer func() {
			r := recover()

			shouldPanic := stackSourceDir == "" || stackGenDir == ""
			switch {
			case shouldPanic && r == nil:
				t.Errorf("expected panic for empty args (src=%q gen=%q), got none", stackSourceDir, stackGenDir)
			case !shouldPanic && r != nil:
				t.Errorf("unexpected panic for src=%q gen=%q: %v", stackSourceDir, stackGenDir, r)
			}
		}()

		_ = hclparse.DiscoverStackChildUnits(fs, stackSourceDir, stackGenDir)
	})
}

// FuzzGenerateAutoIncludeFile_ArgPanics fuzzes targetDir.
// Nil resolved is legitimate — function returns nil without panicking.
func FuzzGenerateAutoIncludeFile_ArgPanics(f *testing.F) {
	f.Add("/target")
	f.Add("")
	f.Add("/")
	f.Add("\x00")
	f.Add("very/long/nested/path/with/many/segments")

	f.Fuzz(func(t *testing.T, targetDir string) {
		fs := vfs.NewMemMapFS()

		defer func() {
			r := recover()

			switch {
			case targetDir == "" && r == nil:
				t.Errorf("expected panic for empty targetDir, got none")
			case targetDir != "" && r != nil:
				t.Errorf("unexpected panic for targetDir=%q: %v", targetDir, r)
			}
		}()

		// Nil resolved is a legitimate no-op; this exercises the argument-panic
		// paths without needing to build a full AutoIncludeResolved.
		_ = hclparse.GenerateAutoIncludeFile(fs, nil, targetDir, nil, nil)
	})
}

// FuzzEvalString verifies EvalString never panics across arbitrary HCL expressions, with and without an eval context. Exercises the literal, nil-ctx, null, unknown, and function-call paths.
func FuzzEvalString(f *testing.F) {
	seeds := []string{
		`"literal"`,
		`null`,
		`42`,
		`true`,
		`upper("vpc")`,
		`local.x`,
		`format("%s-%s", "a", "b")`,
		`"${unit.vpc.path}"`,
		``,
		`{`,
		`[1, 2, 3]`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	ctxWithVars := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"vpc": cty.ObjectVal(map[string]cty.Value{"path": cty.StringVal("/abs/vpc")}),
			}),
			"local": cty.ObjectVal(map[string]cty.Value{"x": cty.StringVal("local-x")}),
		},
	}

	f.Fuzz(func(t *testing.T, input string) {
		parsed, diags := hclsyntax.ParseExpression([]byte(input), "fuzz.hcl", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return
		}

		_, _ = hclparse.EvalString(parsed, nil, "fuzz")
		_, _ = hclparse.EvalString(parsed, ctxWithVars, "fuzz")
		_, _ = hclparse.EvalString(nil, ctxWithVars, "fuzz")
	})
}

// FuzzAutoIncludeResolveWithValues exercises the new `values = {...}` attribute path on AutoIncludeHCL.Resolve, including the dependency.<name>.outputs mock_outputs binding. Verifies no panic on any body content.
func FuzzAutoIncludeResolveWithValues(f *testing.F) {
	seeds := []string{
		`values = { v = "literal" }`,
		`dependency "u" { config_path = "../u"; mock_outputs = { val = "mock-v" } }; values = { v = dependency.u.outputs.val }`,
		`dependency "u" { config_path = "../u" }; values = { v = dependency.u.outputs.val }`,
		`values = {}`,
		`values = null`,
		`values = "not-an-object"`,
		`dependency "u" { config_path = "../u"; mock_outputs = "bad" }; values = { v = dependency.u.outputs.val }`,
		``,
		`{`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit":  cty.EmptyObjectVal,
			"stack": cty.EmptyObjectVal,
		},
	}

	f.Fuzz(func(t *testing.T, input string) {
		file, diags := hclsyntax.ParseConfig([]byte(input), "fuzz.hcl", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return
		}

		autoInclude := &hclparse.AutoIncludeHCL{Remain: file.Body}
		_, _ = autoInclude.Resolve(evalCtx)
	})
}
