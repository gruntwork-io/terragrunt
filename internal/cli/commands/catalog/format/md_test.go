package format_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

func TestMarkdownRendererEntry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		entry *tui.ComponentEntry
		name  string
		want  string
	}{
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
			want: backticks(`## VPC

Creates a VPC.

| Field | Value |
| --- | --- |
| Kind | ~module~ |
| Source | ~github.com/acme/repo~ |
| Directory | ~modules/vpc~ |
| Version | ~v1.2.3~ |
| Component source | ~github.com/acme/repo//modules/vpc~ |

| Tag |
| --- |
| ~networking~ |
| ~aws~ |

~~~markdown
Body text.
~~~

`),
		},
		{
			name: "unit without a readme has no description and no tags",
			entry: tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindUnit,
					"github.com/acme/repo",
					"units/app",
					"",
				),
			).WithSource("github.com/acme/repo"),
			want: backticks(`## app

| Field | Value |
| --- | --- |
| Kind | ~unit~ |
| Source | ~github.com/acme/repo~ |
| Directory | ~units/app~ |
| Component source | ~github.com/acme/repo//units/app~ |

`),
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
			want: backticks(`## Repo Root

The repository itself is the component.

| Field | Value |
| --- | --- |
| Kind | ~module~ |
| Source | ~github.com/acme/repo~ |
| Component source | ~github.com/acme/repo~ |

~~~markdown
Root body.
~~~

`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.NoError(t, format.NewMarkdownRenderer().Entry(&buf, tc.entry))
			assert.Equal(t, tc.want, buf.String())
		})
	}
}

// TestMarkdownRendererEntryTables reads a section back with a table parser:
// the fields parse as a two-column table, and the tags as a table of their
// own, one column wide.
func TestMarkdownRendererEntryTables(t *testing.T) {
	t.Parallel()

	entry := tui.NewComponentEntry(
		tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/acme/repo",
			"modules/vpc",
			vpcReadme,
		),
	).WithSource("github.com/acme/repo").WithVersion("v1.2.3")

	var buf bytes.Buffer

	require.NoError(t, format.NewMarkdownRenderer().Entry(&buf, entry))

	assert.Equal(
		t,
		[]int{2, 2, 2, 2, 2, 1, 1},
		tableRows(t, buf.String()),
		"five field rows of two cells, then a tag each in a table one cell wide",
	)
}

// TestMarkdownRendererDocument renders every kind of component the catalog
// discovers and reads the result back as Markdown: the sections of the
// document are the components, and nothing else.
func TestMarkdownRendererDocument(t *testing.T) {
	t.Parallel()

	cases := entryCases()

	var buf bytes.Buffer

	renderer := format.NewMarkdownRenderer()

	require.NoError(t, renderer.Open(&buf))

	for _, tc := range cases {
		require.NoError(t, renderer.Entry(&buf, tc.entry))
	}

	require.NoError(t, renderer.Close(&buf, format.Summary{Entries: len(cases), Sources: 2}))

	assert.Equal(
		t,
		[]string{"Terragrunt Catalog", "VPC", "service", "app", "prod", "Repo Root"},
		headings(t, buf.String()),
	)
	assert.True(t, strings.HasSuffix(buf.String(), "Discovered 5 components from 2 sources.\n"))
}

// TestMarkdownRendererKeepsReadmeStructureOutOfTheDocument covers the reason
// READMEs are fenced: a README carrying its own headings must not add
// sections to the document that contains it.
func TestMarkdownRendererKeepsReadmeStructureOutOfTheDocument(t *testing.T) {
	t.Parallel()

	const readme = `---
name: VPC
---

# Not a component

## Neither is this

---
`

	entry := tui.NewComponentEntry(
		tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/acme/repo",
			"modules/vpc",
			readme,
		),
	).WithSource("github.com/acme/repo")

	var buf bytes.Buffer

	renderer := format.NewMarkdownRenderer()

	require.NoError(t, renderer.Open(&buf))
	require.NoError(t, renderer.Entry(&buf, entry))
	require.NoError(t, renderer.Close(&buf, format.Summary{Entries: 1, Sources: 1}))

	assert.Equal(t, []string{"Terragrunt Catalog", "VPC"}, headings(t, buf.String()))
}

// TestMarkdownRendererKeepsDescriptionOutOfTheDocumentStructure covers the
// other half of a README the component wrote: a description is prose in the
// document rather than fenced, so one that opens with Markdown structure must
// not add a section or open a fence of its own.
func TestMarkdownRendererKeepsDescriptionOutOfTheDocumentStructure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		description string
	}{
		{name: "heading", description: "# Not a component"},
		{name: "fence", description: backticks("~~~")},
		{name: "thematic break", description: "---"},
		{name: "quote", description: "> Creates a VPC."},
		{name: "list", description: "- Creates a VPC."},
		{name: "html", description: "<p>Creates a VPC.</p>"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entry := tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindModule,
					"github.com/acme/repo",
					"modules/vpc",
					"---\nname: VPC\ndescription: \""+tc.description+"\"\n---\n\nBody.\n",
				),
			).WithSource("github.com/acme/repo")

			var buf bytes.Buffer

			renderer := format.NewMarkdownRenderer()

			require.NoError(t, renderer.Open(&buf))
			require.NoError(t, renderer.Entry(&buf, entry))
			require.NoError(t, renderer.Close(&buf, format.Summary{Entries: 1, Sources: 1}))

			assert.Equal(t, []string{"Terragrunt Catalog", "VPC"}, headings(t, buf.String()))
			assert.Equal(
				t,
				ast.KindParagraph,
				blockAfter(t, buf.String(), "VPC"),
				"the description opens the section as prose",
			)
		})
	}
}

func TestMarkdownRendererFencesReadmeContainingFences(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		body  string
		fence string
	}{
		{
			name: "no backticks",
			body: `Body text.
`,
			fence: "```",
		},
		{
			name: "inline code span",
			body: backticks(`Set ~vpc_name~.
`),
			fence: "```",
		},
		{
			name: "fenced block",
			body: backticks(`~~~hcl
inputs = {}
~~~
`),
			fence: "````",
		},
		{
			name: "fenced block holding a fence",
			body: backticks(`~~~~md
~~~hcl
~~~
~~~~
`),
			fence: "`````",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entry := tui.NewComponentEntry(
				tui.NewComponentForTest(
					tui.ComponentKindModule,
					"github.com/acme/repo",
					"modules/vpc",
					"---\nname: VPC\n---\n\n"+tc.body,
				),
			).WithSource("github.com/acme/repo")

			var buf bytes.Buffer

			require.NoError(t, format.NewMarkdownRenderer().Entry(&buf, entry))

			assert.Contains(t, buf.String(), "\n"+tc.fence+"markdown\n"+tc.body+tc.fence+"\n")
		})
	}
}

// TestMarkdownRendererFencesNonMarkdownReadme pins the info string to what the
// README is actually written in: an AsciiDoc README is fenced without one
// rather than labelled as Markdown.
func TestMarkdownRendererFencesNonMarkdownReadme(t *testing.T) {
	t.Parallel()

	component := tui.NewComponentForTest(
		tui.ComponentKindModule,
		"github.com/acme/repo",
		"modules/vpc",
		"",
	)
	component.Doc = tui.NewComponentDoc(`= VPC

Body text.
`, ".adoc")

	entry := tui.NewComponentEntry(component).WithSource("github.com/acme/repo")

	var buf bytes.Buffer

	require.NoError(t, format.NewMarkdownRenderer().Entry(&buf, entry))
	assert.Contains(t, buf.String(), backticks(`
~~~
= VPC

Body text.
~~~
`))
}

// TestMarkdownRendererKeepsReadmeBodyVerbatim pins the body against the TUI's
// plain-text stripping, which deletes fenced code blocks and reads the
// underscores in vpc_name as emphasis markers.
func TestMarkdownRendererKeepsReadmeBodyVerbatim(t *testing.T) {
	t.Parallel()

	const frontmatter = `---
name: VPC
---

`

	body := backticks(`# VPC

Set ~vpc_name~ and ~cidr_block~.

~~~hcl
inputs = {
  vpc_name = "prod"
}
~~~
`)

	entry := tui.NewComponentEntry(
		tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/acme/repo",
			"modules/vpc",
			frontmatter+body,
		),
	).WithSource("github.com/acme/repo")

	var buf bytes.Buffer

	require.NoError(t, format.NewMarkdownRenderer().Entry(&buf, entry))
	assert.Contains(t, buf.String(), body)
}

// TestMarkdownRendererIndex covers the document footer: a table naming every
// component the document holds, and the count that marks it complete.
func TestMarkdownRendererIndex(t *testing.T) {
	t.Parallel()

	cases := entryCases()

	var buf bytes.Buffer

	renderer := format.NewMarkdownRenderer()

	for _, tc := range cases {
		require.NoError(t, renderer.Entry(io.Discard, tc.entry))
	}

	require.NoError(t, renderer.Close(&buf, format.Summary{Entries: len(cases), Sources: 2}))

	assert.Equal(t, backticks(`---

| Component | Kind | Component source |
| --- | --- | --- |
| VPC | ~module~ | ~github.com/acme/repo//modules/vpc~ |
| service | ~template~ | ~github.com/acme/repo//templates/service?ref=v2~ |
| app | ~unit~ | ~github.com/acme/repo//units/app~ |
| prod | ~stack~ | ~github.com/acme/repo//stacks/prod~ |
| Repo Root | ~module~ | ~github.com/acme/repo~ |

Discovered 5 components from 2 sources.
`), buf.String())
}

func TestMarkdownRendererIndexEscapesPipes(t *testing.T) {
	t.Parallel()

	entry := tui.NewComponentEntry(
		tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/acme/repo",
			"modules/vpc",
			`---
name: A | B
---
`,
		),
	).WithSource("github.com/acme/repo")

	var buf bytes.Buffer

	renderer := format.NewMarkdownRenderer()

	require.NoError(t, renderer.Entry(io.Discard, entry))
	require.NoError(t, renderer.Close(&buf, format.Summary{Entries: 1, Sources: 1}))

	assert.Contains(t, buf.String(), `| A \| B | `)
	assert.Equal(t, []int{3}, tableRows(t, buf.String()), "one row of three cells")
}

func TestMarkdownRendererSummary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		want    string
		summary format.Summary
	}{
		{
			name:    "nothing discovered",
			summary: format.Summary{},
			want: `---

No components were discovered.
`,
		},
		{
			name:    "one component in one source",
			summary: format.Summary{Entries: 1, Sources: 1},
			want: `Discovered 1 component from 1 source.
`,
		},
		{
			name:    "many components across many sources",
			summary: format.Summary{Entries: 12, Sources: 3},
			want: `Discovered 12 components from 3 sources.
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			require.NoError(t, format.NewMarkdownRenderer().Close(&buf, tc.summary))
			assert.True(t, strings.HasSuffix(buf.String(), tc.want), buf.String())
		})
	}
}

func TestMarkdownRendererFlushesEveryEntry(t *testing.T) {
	t.Parallel()

	w := &flushCountingWriter{}
	renderer := format.NewMarkdownRenderer()

	require.NoError(t, renderer.Open(w))
	assert.Equal(t, 1, w.flushes, "the header must reach the consumer before the first entry")

	cases := entryCases()

	for i, tc := range cases {
		require.NoError(t, renderer.Entry(w, tc.entry))
		assert.Equal(t, i+2, w.flushes, "every entry must be flushed as it is written")
	}

	require.NoError(t, renderer.Close(w, format.Summary{Entries: len(cases), Sources: 1}))
	assert.Equal(t, len(cases)+2, w.flushes)
}

// headings parses doc as Markdown and returns the text of every heading, so a
// test can assert what the document is made of rather than which lines it
// happens to contain.
func headings(t *testing.T, doc string) []string {
	t.Helper()

	source := []byte(doc)

	var titles []string

	err := ast.Walk(
		goldmark.DefaultParser().Parse(text.NewReader(source)),
		func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			heading, ok := n.(*ast.Heading)
			if !ok || !entering {
				return ast.WalkContinue, nil
			}

			titles = append(titles, string(heading.Lines().Value(source)))

			return ast.WalkSkipChildren, nil
		},
	)
	require.NoError(t, err)

	return titles
}

// blockAfter parses doc as Markdown and names the kind of block that follows
// the given heading, so a test can assert what a value became rather than
// which characters it was written as.
func blockAfter(t *testing.T, doc, heading string) ast.NodeKind {
	t.Helper()

	source := []byte(doc)
	document := goldmark.DefaultParser().Parse(text.NewReader(source))

	var found ast.Node

	for n := document.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok || string(h.Lines().Value(source)) != heading {
			continue
		}

		found = n

		break
	}

	require.NotNil(t, found, "no heading %q in %s", heading, doc)

	next := found.NextSibling()
	require.NotNil(t, next, "heading %q closes the document", heading)

	return next.Kind()
}

// tableRows parses doc as GitHub-flavored Markdown and returns the number of
// cells in each body row of its table, so a test can assert that a value
// holding a pipe stayed inside the cell it was written to.
func tableRows(t *testing.T, doc string) []int {
	t.Helper()

	parser := goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()

	var rows []int

	err := ast.Walk(
		parser.Parse(text.NewReader([]byte(doc))),
		func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			row, ok := n.(*extast.TableRow)
			if !ok || !entering {
				return ast.WalkContinue, nil
			}

			rows = append(rows, row.ChildCount())

			return ast.WalkSkipChildren, nil
		},
	)
	require.NoError(t, err)

	return rows
}

// backticks turns the tildes of a raw string literal into the backticks the
// renderer writes, which a raw string literal cannot hold.
func backticks(s string) string {
	return strings.ReplaceAll(s, "~", "`")
}
