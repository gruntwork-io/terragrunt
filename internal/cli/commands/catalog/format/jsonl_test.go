package format_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

// vpcReadme carries every piece of metadata an entry can pick up from a
// README: a title, a description, tags, and a body.
const vpcReadme = `---
name: VPC
description: Creates a VPC.
tags:
  - networking
  - aws
---

Body text.
`

const rootReadme = `---
name: Repo Root
description: The repository itself is the component.
---

Root body.
`

// schemaPath locates the published schema. Tests read the file the docs site
// serves so the renderer and the contract consumers validate against cannot
// drift apart.
var schemaPath = filepath.Join(
	"..", "..", "..", "..", "..",
	"docs", "public", "schemas", "catalog", "v1", "schema.json",
)

type entryCase struct {
	entry *tui.ComponentEntry
	name  string
	want  format.Entry
}

func entryCases() []entryCase {
	return []entryCase{
		{
			name: "module with full readme metadata",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindModule,
					"github.com/acme/repo",
					"modules/vpc",
					vpcReadme,
				),
			).WithSource("github.com/acme/repo").WithVersion("v1.2.3"),
			want: format.Entry{
				Kind:            "module",
				Title:           "VPC",
				Description:     "Creates a VPC.",
				Tags:            []string{"networking", "aws"},
				Source:          "github.com/acme/repo",
				Dir:             "modules/vpc",
				Version:         "v1.2.3",
				ComponentSource: "github.com/acme/repo//modules/vpc",
				Doc:             "Body text.\n",
			},
		},
		{
			name: "template without a readme",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindTemplate,
					"github.com/acme/repo?ref=v2",
					"templates/service",
					"",
				),
			).WithSource("github.com/acme/repo"),
			want: format.Entry{
				Kind:            "template",
				Title:           "service",
				Description:     "(no description found)",
				Source:          "github.com/acme/repo",
				Dir:             "templates/service",
				ComponentSource: "github.com/acme/repo//templates/service?ref=v2",
			},
		},
		{
			name: "unit is copyable",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindUnit,
					"github.com/acme/repo",
					"units/app",
					"",
				),
			).WithSource("github.com/acme/repo"),
			want: format.Entry{
				Kind:            "unit",
				Title:           "app",
				Description:     "(no description found)",
				Source:          "github.com/acme/repo",
				Dir:             "units/app",
				ComponentSource: "github.com/acme/repo//units/app",
				Copyable:        true,
			},
		},
		{
			name: "stack is copyable",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindStack,
					"github.com/acme/repo",
					"stacks/prod",
					"",
				),
			).WithSource("github.com/acme/repo"),
			want: format.Entry{
				Kind:            "stack",
				Title:           "prod",
				Description:     "(no description found)",
				Source:          "github.com/acme/repo",
				Dir:             "stacks/prod",
				ComponentSource: "github.com/acme/repo//stacks/prod",
				Copyable:        true,
			},
		},
		{
			name: "repository root component",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindModule,
					"github.com/acme/repo",
					"",
					rootReadme,
				),
			).WithSource("github.com/acme/repo"),
			want: format.Entry{
				Kind:            "module",
				Title:           "Repo Root",
				Description:     "The repository itself is the component.",
				Source:          "github.com/acme/repo",
				ComponentSource: "github.com/acme/repo",
				Doc:             "Root body.\n",
			},
		},
	}
}

func TestJSONLRendererEntry(t *testing.T) {
	t.Parallel()

	for _, tc := range entryCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.NoError(t, format.NewJSONLRenderer().Entry(&buf, tc.entry))

			line, ok := strings.CutSuffix(buf.String(), "\n")
			require.True(t, ok, "a record must end with a newline")
			require.NotContains(t, line, "\n", "a record must occupy a single line")

			var got format.Entry

			require.NoError(t, json.Unmarshal([]byte(line), &got))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestJSONLRendererOmitsUnsetFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		entry *tui.ComponentEntry
		name  string
		want  []string
	}{
		{
			name: "entry without optional metadata",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindModule,
					"github.com/acme/repo",
					"modules/vpc",
					"",
				),
			).WithSource("github.com/acme/repo"),
			want: []string{"component_source", "description", "dir", "kind", "source", "title"},
		},
		{
			name: "entry with every field populated",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindUnit,
					"github.com/acme/repo",
					"units/app",
					vpcReadme,
				),
			).WithSource("github.com/acme/repo").WithVersion("v1.2.3"),
			want: []string{
				"component_source", "copyable", "description", "dir", "doc",
				"kind", "source", "tags", "title", "version",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.NoError(t, format.NewJSONLRenderer().Entry(&buf, tc.entry))

			var got map[string]any

			require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
			assert.Equal(t, tc.want, slices.Sorted(maps.Keys(got)))
		})
	}
}

func TestJSONLRendererKeepsReadmeMarkupUnescaped(t *testing.T) {
	t.Parallel()

	entry := tui.NewComponentEntry(
		tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/acme/repo",
			"modules/vpc",
			"---\nname: VPC\n---\n\nUse a & b, and keep 1 < 2.\n",
		),
	).WithSource("github.com/acme/repo")

	var buf bytes.Buffer

	require.NoError(t, format.NewJSONLRenderer().Entry(&buf, entry))
	assert.Contains(t, buf.String(), "Use a & b, and keep 1 < 2.")
}

// TestJSONLRendererKeepsReadmeBodyVerbatim pins the body against the TUI's
// plain-text stripping, which deletes fenced code blocks and reads the
// underscores in vpc_name as emphasis markers.
func TestJSONLRendererKeepsReadmeBodyVerbatim(t *testing.T) {
	t.Parallel()

	const (
		frontmatter = "---\nname: VPC\n---\n\n"
		body        = "# VPC\n\nSet `vpc_name` and `cidr_block`.\n\n```hcl\ninputs = {\n  vpc_name = \"prod\"\n}\n```\n"
	)

	entry := tui.NewComponentEntry(
		tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/acme/repo",
			"modules/vpc",
			frontmatter+body,
		),
	).WithSource("github.com/acme/repo")

	var buf bytes.Buffer

	require.NoError(t, format.NewJSONLRenderer().Entry(&buf, entry))

	var got format.Entry

	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, body, got.Doc)
}

func TestJSONLRendererFlushesEveryEntry(t *testing.T) {
	t.Parallel()

	w := &flushCountingWriter{}
	renderer := format.NewJSONLRenderer()

	require.NoError(t, renderer.Open(w))
	assert.Equal(t, 0, w.flushes)

	for i, tc := range entryCases() {
		require.NoError(t, renderer.Entry(w, tc.entry))
		assert.Equal(t, i+1, w.flushes, "every entry must be flushed as it is written")
	}
}

func TestJSONLRendererSchemaConformance(t *testing.T) {
	t.Parallel()

	schemaBytes, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)

	cases := entryCases()

	var buf bytes.Buffer

	renderer := format.NewJSONLRenderer()

	require.NoError(t, renderer.Open(&buf))

	for _, tc := range cases {
		require.NoError(t, renderer.Entry(&buf, tc.entry))
	}

	require.NoError(t, renderer.Close(&buf, format.Summary{Entries: len(cases), Sources: 1}))

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, len(cases))

	for i, line := range lines {
		result, err := gojsonschema.Validate(schemaLoader, gojsonschema.NewStringLoader(line))
		require.NoError(t, err)
		assert.True(t, result.Valid(), "%s: %v", cases[i].name, result.Errors())
	}
}

func TestNewRenderer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{name: "jsonl", format: "jsonl"},
		{name: "markdown is not implemented yet", format: "md", wantErr: true},
		{name: "the tui is not a renderer", format: "tui", wantErr: true},
		{name: "unknown", format: "yaml", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			renderer, err := format.NewRenderer(tc.format)

			if tc.wantErr {
				require.ErrorIs(t, err, format.ErrUnsupportedFormat)
				assert.Nil(t, renderer)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, renderer)
		})
	}
}

// flushCountingWriter records how many times a renderer asked for its output
// to be pushed through to the consumer.
type flushCountingWriter struct {
	bytes.Buffer

	flushes int
}

func (w *flushCountingWriter) Flush() error {
	w.flushes++

	return nil
}
