package md_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/md"
)

// fuzzSeeds are documents worth starting from: what Terragrunt writes and
// reads back, and the constructs that decide where one piece of a document
// ends and the next begins.
var fuzzSeeds = []string{
	"",
	"# Terragrunt Catalog\n\n## VPC\n\nCreates a VPC.\n",
	"| Field | Value |\n| --- | --- |\n| Kind | `module` |\n| Source | A \\| B |\n",
	"| Tag |\n| --- |\n| ``net`work`` |\n",
	"<https://github.com/acme/repo> and /Users/jane/My Modules/vpc\n",
	"## VPC\n\n1\\. Creates a VPC.\n",
	"## VPC\n\n\\# Creates a VPC. &amp; more &#65; &notareference;\n",
	"```markdown\n# Fenced, so not a heading\n```\n",
	"````md\n```hcl\n```\n````\n",
	"``` unclosed fence\n# still inside\n",
	"`a\nb`\n",
	"Setext\n===\n\nAnother\n---\n",
	"0\n 0\n-",
	// A fence opening a quoted line with nothing but a tab before it crashed
	// goldmark up to v1.8.4.
	"> \t`",
	"> - # quoted heading in a list\n>   - deeper\n",
	"|a|b|\n|-|-|\n||\n|`|`|x|\n",
	"\r\n# CRLF\r\n\r\ntext\r\n",
	"\x00# NUL\n\n\x00\n",
	"# \ufeff\u200b\n\n\U0001f600\u0301 combining\n",
	strings.Repeat("> ", 64) + "deeply nested\n",
	strings.Repeat("*", 64) + "\n",
}

// FuzzParse drives every question a document can be asked. A document is
// untrusted input (a component's README, whatever a repository holds), so no
// reading of one may panic, and what a reading returns has to hold together
// whatever the document turns out to be.
func FuzzParse(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		doc := md.Parse(source)

		for _, heading := range doc.Headings() {
			block, ok := doc.BlockAfter(heading)
			if !ok {
				continue
			}

			assert.NotEmpty(t, block.Kind(), "every block names its kind")

			block.Text()
		}

		for _, span := range doc.CodeSpans() {
			assert.NotContains(t, span, "\n", "a code span is one line to the reader")
		}

		for _, target := range doc.Autolinks() {
			assert.NotEmpty(t, target, "an autolink has a target")
			assert.NotContains(t, target, " ", "a space ends an autolink")
			assert.NotContains(t, target, "\n", "a line ending ends an autolink")
		}

		for _, cells := range doc.TableRowCells() {
			assert.Positive(t, cells, "a row of a table holds cells")
		}
	})
}

// FuzzParseIsRepeatable pins a document to one reading of it. Each parse
// builds a parser of its own, so a reading that depended on state held over
// from an earlier one would answer differently the second time.
func FuzzParseIsRepeatable(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, source string) {
		first, second := md.Parse(source), md.Parse(source)

		require.Equal(t, first.Headings(), second.Headings())
		require.Equal(t, first.CodeSpans(), second.CodeSpans())
		require.Equal(t, first.Autolinks(), second.Autolinks())
		require.Equal(t, first.TableRowCells(), second.TableRowCells())
	})
}

// FuzzBlockAfter looks up a heading that need not be in the document, which is
// how a caller asks whether a document is built the way it expected.
func FuzzBlockAfter(f *testing.F) {
	f.Add("# Terragrunt Catalog\n\n## VPC\n\nCreates a VPC.\n", "VPC")
	f.Add("## VPC\n", "VPC")
	f.Add("", "VPC")
	f.Add("## VPC\n\nCreates a VPC.\n", "")

	f.Fuzz(func(t *testing.T, source, heading string) {
		block, ok := md.Parse(source).BlockAfter(heading)
		if !ok {
			return
		}

		assert.Contains(t, md.Parse(source).Headings(), heading, "a heading that was found is one of the document's")
		assert.NotEmpty(t, block.Kind(), "every block names its kind")

		block.Text()
	})
}

// FuzzRender renders a document the way a TUI does. The source reaching a
// renderer is a file the user pointed at, so no content may panic the terminal
// out from under them.
func FuzzRender(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed, 80)
	}

	f.Add("# VPC\n", 1)
	f.Add(strings.Repeat("word ", 128), md.NoWrapWidth)

	f.Fuzz(func(t *testing.T, source string, width int) {
		// A width the renderer cannot lay a document out in is the caller's
		// error to make, not the fuzzer's to find; the TUIs pass the pane they
		// measured.
		if width < 1 || width > md.NoWrapWidth {
			t.Skip()
		}

		// Terminal escapes and invalid UTF-8 are stripped before a document
		// reaches a renderer, by the sanitizing every TUI pane runs its content
		// through, so what is fuzzed here is what a renderer actually sees.
		source = strings.ToValidUTF8(strings.ReplaceAll(source, "\x1b", ""), "�")

		renderer, err := md.NewTerminalRenderer(width, md.DarkBackground)
		require.NoError(t, err)

		_, err = renderer.Render(source)
		require.NoError(t, err)
	})
}
