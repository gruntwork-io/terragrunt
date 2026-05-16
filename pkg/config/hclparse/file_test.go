// Package hclparse_test exercises the rebinding contract that pkg/config relies on
// when reusing a cached File across parsing contexts with different ParserOptions.
package hclparse_test

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// hclWithUndefinedVar parses cleanly but evaluates with an "Unknown variable"
// diagnostic when the EvalContext has no `dependency` variable.
const hclWithUndefinedVar = `
foo = dependency.bar.outputs.baz
`

const fixturePath = "/virtual/test.hcl"

type fooOnly struct {
	Foo string `hcl:"foo"`
}

// TestRebindRoutesDiagnosticsThroughNewWriter checks that Decode-time diagnostics
// flow through the rebound parser's writer rather than the parser the file was
// originally parsed with.
func TestRebindRoutesDiagnosticsThroughNewWriter(t *testing.T) {
	t.Parallel()

	var (
		originalBuf bytes.Buffer
		reboundBuf  bytes.Buffer
	)

	original := hclparse.NewParser(hclparse.WithDiagnosticsWriter(&originalBuf, true))

	file, err := original.ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	rebound := file.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(&reboundBuf, true)))

	var out fooOnly

	decodeErr := rebound.Decode(&out, evalContextMissingDependency())
	require.Error(t, decodeErr, "decode must surface the undefined-variable diagnostic as an error")

	assert.Empty(t, originalBuf.String(),
		"original parser's writer must not receive diagnostics from the rebound file")
	assert.Contains(t, reboundBuf.String(), `no variable named "dependency"`,
		"rebound parser's writer must receive the diagnostic")
}

// TestRebindLeavesOriginalFileUnaffected checks that decoding the original file
// after a Rebind still routes diagnostics through the original parser's writer.
func TestRebindLeavesOriginalFileUnaffected(t *testing.T) {
	t.Parallel()

	var (
		originalBuf bytes.Buffer
		reboundBuf  bytes.Buffer
	)

	original := hclparse.NewParser(hclparse.WithDiagnosticsWriter(&originalBuf, true))

	file, err := original.ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	rebound := file.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(&reboundBuf, true)))
	require.NotNil(t, rebound, "Rebind must return a usable file wrapper")

	var out fooOnly

	decodeErr := file.Decode(&out, evalContextMissingDependency())
	require.Error(t, decodeErr)

	assert.Contains(t, originalBuf.String(), `no variable named "dependency"`,
		"original parser's writer must still receive diagnostics from the original file")
	assert.Empty(t, reboundBuf.String(),
		"rebound parser must not receive diagnostics from the original file wrapper")
}

// TestRebindRendersSourceSnippet checks that the rebound parser's diagnostic
// includes the source snippet. This pins the AddFile side effect: without it,
// the diagnostic writer's file map (captured by reference at construction time)
// has no AST for the snippet renderer.
func TestRebindRendersSourceSnippet(t *testing.T) {
	t.Parallel()

	var reboundBuf bytes.Buffer

	original := hclparse.NewParser(hclparse.WithDiagnosticsWriter(io.Discard, true))

	file, err := original.ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	rebound := file.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(&reboundBuf, true)))

	var out fooOnly

	decodeErr := rebound.Decode(&out, evalContextMissingDependency())
	require.Error(t, decodeErr)

	rendered := reboundBuf.String()

	assert.Contains(t, rendered, fixturePath, "diagnostic must reference the file path")
	assert.Contains(t, rendered, "dependency.bar.outputs.baz",
		"diagnostic must render the source snippet, proving the AST was registered with the new parser")
}

// TestRebindWithRacing exercises concurrent Rebind+Decode on a shared cached
// File. The CI "Race" job runs tests matching .*WithRacing with -race. This
// mirrors the production flow where the parse cache is read by many goroutines
// (e.g. during `run --all`), each binding the file to a fresh parser before
// decoding.
func TestRebindWithRacing(t *testing.T) {
	t.Parallel()

	const goroutines = 32

	cached, err := hclparse.NewParser(hclparse.WithDiagnosticsWriter(io.Discard, true)).
		ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
	)

	start.Add(1)

	for range goroutines {
		done.Add(1)

		go func() {
			defer done.Done()

			start.Wait()

			var buf bytes.Buffer

			rebound := cached.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(&buf, true)))

			var out fooOnly

			decodeErr := rebound.Decode(&out, evalContextMissingDependency())
			assert.Error(t, decodeErr)
			assert.Contains(t, buf.String(), `no variable named "dependency"`)
		}()
	}

	start.Done()
	done.Wait()
}

// evalContextMissingDependency returns an EvalContext with a non-empty Variables
// map that omits `dependency`. A non-empty Variables map is required to produce
// the "no variable named X" wording; an empty context yields "Variables not
// allowed" instead.
func evalContextMissingDependency() *hcl.EvalContext {
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"sentinel": cty.StringVal("present"),
		},
	}
}
