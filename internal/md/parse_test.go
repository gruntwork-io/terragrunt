package md_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/md"
)

func TestDocHeadings(t *testing.T) {
	t.Parallel()

	doc := md.Parse(backticks(`# Catalog

## VPC

Creates a VPC.

~~~markdown
## Not a heading of this document
~~~

## app
`))

	assert.Equal(t, []string{"Catalog", "VPC", "app"}, doc.Headings())
}

func TestDocCodeSpans(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		doc  string
		want []string
	}{
		{
			name: "spans of their own",
			doc:  backticks("A ~module~ in ~modules/vpc~."),
			want: []string{"module", "modules/vpc"},
		},
		{
			name: "a delimiter longer than the run it holds",
			doc:  backticks("~~net~work~~"),
			want: []string{"net`work"},
		},
		{
			name: "a padded span gives its padding back",
			doc:  backticks("~~ ~network~ ~~"),
			want: []string{"`network`"},
		},
		{
			name: "an escape inside a span is text of its own",
			doc:  backticks(`~a \| b~`),
			want: []string{`a \| b`},
		},
		{
			name: "a fenced block is not a span",
			doc:  backticks("~~~\ninputs = {}\n~~~\n"),
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, md.Parse(tc.doc).CodeSpans())
		})
	}
}

func TestDocAutolinks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		doc  string
		want []string
	}{
		{
			name: "angle brackets make a link",
			doc:  "<https://github.com/acme/repo>",
			want: []string{"https://github.com/acme/repo"},
		},
		{
			name: "a bare URL is text",
			doc:  "https://github.com/acme/repo",
			want: nil,
		},
		{
			name: "a path is text",
			doc:  backticks("~/Users/jane/modules/vpc~"),
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, md.Parse(tc.doc).Autolinks())
		})
	}
}

func TestDocTableRowCells(t *testing.T) {
	t.Parallel()

	doc := md.Parse(`| Field | Value |
| --- | --- |
| Kind | module |
| Source | A \| B |

| Tag |
| --- |
| networking |
`)

	assert.Equal(t, []int{2, 2, 1}, doc.TableRowCells())
}

func TestDocBlockAfter(t *testing.T) {
	t.Parallel()

	const doc = `# Catalog

## VPC

Creates a VPC.

## app
`

	testCases := []struct {
		name    string
		heading string
		want    md.Kind
		found   bool
	}{
		{name: "prose", heading: "VPC", want: md.KindParagraph, found: true},
		{name: "another section", heading: "Catalog", want: "Heading", found: true},
		{name: "the heading closes the document", heading: "app", found: false},
		{name: "no such heading", heading: "missing", found: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			block, ok := md.Parse(doc).BlockAfter(tc.heading)
			require.Equal(t, tc.found, ok)

			if !tc.found {
				return
			}

			assert.Equal(t, tc.want, block.Kind())
		})
	}
}

// TestBlockTextReadsAsWritten covers the escapes a document carries to keep a
// value out of its structure: they hold the value together on the page and
// must not reach the reader.
func TestBlockTextReadsAsWritten(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		body string
		want string
	}{
		{name: "plain prose", body: "Creates a VPC.", want: "Creates a VPC."},
		{name: "escaped list marker", body: `1\. Creates a VPC.`, want: "1. Creates a VPC."},
		{name: "escaped heading", body: `\# Creates a VPC.`, want: "# Creates a VPC."},
		{name: "escaped pipe", body: `A \| B`, want: "A | B"},
		{name: "character reference", body: "A &amp; B", want: "A & B"},
		{name: "emphasis", body: "Creates a *VPC*.", want: "Creates a VPC."},
		{
			name: "a code span holds its content as written",
			body: backticks(`Set ~a \| b~.`),
			want: `Set a \| b.`,
		},
		{
			name: "a soft line break is a line of its own",
			body: "Creates a VPC.\nAnd a subnet.",
			want: "Creates a VPC.\nAnd a subnet.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			block, ok := md.Parse("## VPC\n\n" + tc.body + "\n").BlockAfter("VPC")
			require.True(t, ok)
			require.Equal(t, md.KindParagraph, block.Kind())

			assert.Equal(t, tc.want, block.Text())
		})
	}
}

// backticks turns the tildes of a raw string literal into the backticks a
// Markdown document is written with, which a raw string literal cannot hold.
func backticks(s string) string {
	return strings.ReplaceAll(s, "~", "`")
}
