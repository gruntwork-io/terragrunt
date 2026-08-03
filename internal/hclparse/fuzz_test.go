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
			_, _ = hclparse.PartialEval(attr.Expr, &hclparse.EvalArgs{
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
			_ = hclparse.GenerateAutoIncludeFile(
				fs,
				resolved,
				"/fuzz/.terragrunt-stack/out",
				srcBytes,
				resolved.EvalCtx,
			)
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

// FuzzBuildComponentRefMap tests component ref map building with arbitrary
// names and paths. Verifies that BuildComponentRefMap never panics regardless
// of input.
func FuzzBuildComponentRefMap(f *testing.F) {
	f.Add("vpc", "/path/vpc")
	f.Add("", "")
	f.Add("path", "/reserved")
	f.Add("name", "/also-reserved")

	f.Fuzz(func(t *testing.T, name, path string) {
		hclparse.BuildComponentRefMap([]hclparse.ComponentRef{{Name: name, Path: path}})
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
// panic (it may return an error - that's fine).
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

		_, _ = hclparse.UnitPathsFromStackDir(fs, stackDir, noFuncs)
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

// FuzzGenerateAutoIncludeFile_ArgPanics fuzzes targetDir.
// Nil resolved is legitimate - function returns nil without panicking.
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

// FuzzAutoIncludeResolveForKindStack tests AutoIncludeHCL.ResolveForKind for the stack kind with
// arbitrary autoinclude bodies, exercising the dependency-values validator that rejects an injected
// unit/stack whose values reference dependency outputs (dependency.foo, dependency["foo"], and the
// dynamic dependency[values.x] forms). Verifies stack-kind resolution never panics regardless of input.
func FuzzAutoIncludeResolveForKindStack(f *testing.F) {
	seeds := []string{
		`unit "extra" { source = "."; path = "extra"; values = { v = unit.producer.path } }`,
		`unit "extra" { source = "."; path = "extra"; values = { v = dependency.foo.outputs.bar } }`,
		`unit "extra" { source = "."; path = "extra"; values = { v = dependency["foo"].outputs.bar } }`,
		`unit "extra" { source = "."; path = "extra"; values = { v = dependency[values.x].outputs.bar } }`,
		`stack "child" { source = "."; path = "child"; values = { v = dependency.foo.outputs.bar } }`,
		`dependency "foo" { config_path = unit.producer.path }
unit "extra" { source = "."; path = "extra"; values = { v = dependency.foo.outputs.bar } }`,
		`dependency "foo" { config_path = "../foo" }`,
		``,
		`{}`,
		`unit "x" {}`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"unit": cty.ObjectVal(map[string]cty.Value{
				"producer": cty.ObjectVal(
					map[string]cty.Value{"path": cty.StringVal("../producer")},
				),
			}),
			"stack": cty.EmptyObjectVal,
		},
	}

	f.Fuzz(func(t *testing.T, input string) {
		file, diags := hclsyntax.ParseConfig([]byte(input), "fuzz.hcl", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return
		}

		autoInclude := &hclparse.AutoIncludeHCL{Remain: file.Body}
		_, _ = autoInclude.ResolveForKind(evalCtx, hclparse.KindStack, "fuzz")
	})
}

// FuzzUnitPathsFromStackDir_AutoIncludeContent fuzzes the content of a sibling
// terragrunt.autoinclude.stack.hcl while running discovery expansion, exercising
// mergeDiscoveryStackAutoInclude end to end: the hclsyntax parse, the dependency-values backstop, and
// the strict unit/stack-only decode that rejects locals, includes, and other stray top-level content.
// Verifies discovery never panics regardless of autoinclude content.
func FuzzUnitPathsFromStackDir_AutoIncludeContent(f *testing.F) {
	seeds := []string{
		`unit "extra" { source = "."; path = "extra" }`,
		`stack "child" { source = "."; path = "child" }`,
		`unit "extra" { source = "."; path = "extra"; values = { v = dependency.foo.outputs.bar } }`,
		`unit "base" { source = "."; path = "base2" }`,
		`locals { x = 1 }`,
		`include "root" { path = "../root.hcl" }`,
		`generate "x" { path = "x.tf"; if_exists = "overwrite"; contents = "" }`,
		`inputs = { a = 1 }`,
		``,
		`{}`,
		`not hcl @#$`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, autoIncludeContent string) {
		fs := vfs.NewMemMapFS()
		_ = fs.MkdirAll("/fuzz", 0755)
		_ = vfs.WriteFile(
			fs,
			"/fuzz/terragrunt.stack.hcl",
			[]byte(`unit "base" { source = "."; path = "base" }`),
			0644,
		)
		_ = vfs.WriteFile(
			fs,
			"/fuzz/"+hclparse.AutoIncludeStackFile,
			[]byte(autoIncludeContent),
			0644,
		)

		_, _ = hclparse.UnitPathsFromStackDir(fs, "/fuzz", noFuncs)
	})
}
