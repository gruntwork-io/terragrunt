// Package hclparse_test exercises the rebinding contract that pkg/config relies on
// when reusing a cached File across parsing contexts with different ParserOptions.
package hclparse_test

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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

// jsonWithInvalidIdentifier parses cleanly: JSON keys are arbitrary strings, so a
// name that HCL's own syntax would reject only surfaces when the attributes are read.
const jsonWithInvalidIdentifier = `{"foo bar": "baz"}`

// TestJustAttributesRejectsInvalidIdentifier pins that a JSON attribute name that
// isn't a valid HCL identifier fails the read rather than being read as valid.
func TestJustAttributesRejectsInvalidIdentifier(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	parser := hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &buf, true))

	file, err := parser.ParseFromString(jsonWithInvalidIdentifier, "/virtual/test.hcl.json")
	require.NoError(t, err)

	attrs, err := file.JustAttributes()

	require.Error(t, err, "an attribute name that is not a valid identifier must fail the read")
	assert.Nil(t, attrs)
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

	original := hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &originalBuf, true))

	file, err := original.ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	rebound := file.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &reboundBuf, true)))

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

	original := hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &originalBuf, true))

	file, err := original.ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	rebound := file.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &reboundBuf, true)))
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

	original := hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), io.Discard, true))

	file, err := original.ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	rebound := file.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &reboundBuf, true)))

	var out fooOnly

	decodeErr := rebound.Decode(&out, evalContextMissingDependency())
	require.Error(t, decodeErr)

	rendered := reboundBuf.String()

	assert.Contains(t, rendered, fixturePath, "diagnostic must reference the file path")
	assert.Contains(
		t,
		rendered,
		"dependency.bar.outputs.baz",
		"diagnostic must render the source snippet, proving the AST was registered with the new parser",
	)
}

// TestRebindWithRacing exercises concurrent Rebind+Decode on a shared cached
// File. The CI "Race" job runs tests matching .*WithRacing with -race. This
// mirrors the production flow where the parse cache is read by many goroutines
// (e.g. during `run --all`), each binding the file to a fresh parser before
// decoding.
func TestRebindWithRacing(t *testing.T) {
	t.Parallel()

	const goroutines = 32

	cached, err := hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), io.Discard, true)).
		ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	var (
		start sync.WaitGroup
		done  sync.WaitGroup
	)

	start.Add(1)

	for range goroutines {
		done.Go(func() {
			start.Wait()

			var buf bytes.Buffer

			rebound := cached.Rebind(hclparse.NewParser(hclparse.WithDiagnosticsWriter(venvtest.New(), &buf, true)))

			var out fooOnly

			decodeErr := rebound.Decode(&out, evalContextMissingDependency())
			//nolint:testifylint // require's FailNow must run on the test goroutine, not this spawned one
			assert.Error(t, decodeErr)
			assert.Contains(t, buf.String(), `no variable named "dependency"`)
		})
	}

	start.Done()
	done.Wait()
}

// TestRebindPanicsOnNilParser pins the documented contract that Rebind must be
// called with a non-nil parser. Production callers obtain the parser from
// hclparse.NewParser, which never returns nil; the panic exists so a future
// misuse fails loudly at the call site instead of deep inside AddFile.
func TestRebindPanicsOnNilParser(t *testing.T) {
	t.Parallel()

	file, err := hclparse.NewParser().ParseFromString(hclWithUndefinedVar, fixturePath)
	require.NoError(t, err)

	assert.PanicsWithValue(t, "hclparse: Rebind called with nil parser", func() {
		file.Rebind(nil)
	})
}

// hclWithBareInclude carries the unlabelled include block that WithFileUpdate rewrites, so a
// parse that ran the update handler would hold different content from one that did not.
const hclWithBareInclude = `
include {
  path = "../root.hcl"
}

foo = "bar"
`

// includeAndFoo decodes the whole of hclWithBareInclude.
type includeAndFoo struct {
	Foo      string `hcl:"foo"`
	Includes []struct {
		Path string `hcl:"path"`
	} `hcl:"include,block"`
}

// TestParserOptionsDoNotChangeASuccessfulParse pins what lets pkg/config key one cached AST on
// the file alone: whatever options a caller parses under, a parse that raises no diagnostics
// produces the same AST. The options in use decide only what becomes of the diagnostics.
func TestParserOptionsDoNotChangeASuccessfulParse(t *testing.T) {
	t.Parallel()

	optionSets := map[string][]hclparse.Option{
		"no options": nil,
		"logger":     {hclparse.WithLogger(logger.CreateLogger())},
		"diagnostics writer": {
			hclparse.WithDiagnosticsWriter(venvtest.New(), io.Discard, true),
		},
		"file update": {
			hclparse.WithFileUpdate(func(*hclparse.File) error { return nil }),
		},
		"diagnostics handler": {
			hclparse.WithDiagnosticsHandler(
				func(_ *hcl.File, _ hcl.Diagnostics) (hcl.Diagnostics, error) { return nil, nil },
			),
		},
		"halt on error only for blocks": {
			hclparse.WithHaltOnErrorOnlyForBlocks([]string{"catalog"}),
		},
	}

	for name, options := range optionSets {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, err := hclparse.NewParser(options...).ParseFromString(hclWithBareInclude, fixturePath)
			require.NoError(t, err)

			assert.Equal(t, hclWithBareInclude, file.Content())
			assert.False(t, file.HasDiagnostics())

			blocks, err := file.Blocks("include", true)
			require.NoError(t, err, "the include block must still be the unlabelled one that was parsed")
			require.Len(t, blocks, 1)
			assert.Empty(t, blocks[0].Labels)
		})
	}
}

// TestFileUpdateRunsAtDecodeRatherThanParse pins the half of the option contract that would
// otherwise change an AST: the handler that rewrites a bare `include {}` into a labelled one
// leaves the parse alone and runs when the file is decoded.
func TestFileUpdateRunsAtDecodeRatherThanParse(t *testing.T) {
	t.Parallel()

	updates := 0

	parser := hclparse.NewParser(hclparse.WithFileUpdate(func(*hclparse.File) error {
		updates++

		return nil
	}))

	file, err := parser.ParseFromString(hclWithBareInclude, fixturePath)
	require.NoError(t, err)
	assert.Zero(t, updates, "parsing must not run the file update handler")

	var out includeAndFoo

	require.NoError(t, file.Decode(&out, nil))
	assert.Equal(t, 1, updates, "decoding must run the file update handler")
	assert.Equal(t, "bar", out.Foo)
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
