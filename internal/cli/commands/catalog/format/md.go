package format

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

// minFenceLength is the shortest code fence CommonMark recognizes.
const minFenceLength = 3

const (
	// fieldTableHeader opens a component's table of metadata, one field per row.
	fieldTableHeader = "| Field | Value |\n| --- | --- |\n"

	// tagTableHeader opens a component's table of tags. Tags are a flat list
	// today, so the table has the one column; a value column joins it the day
	// they carry one.
	tagTableHeader = "| Tag |\n| --- |\n"

	// indexTableHeader opens the table of components the document closes with.
	indexTableHeader = "| Component | Kind | Component source |\n| --- | --- | --- |\n"
)

// markdownHeader opens the document. It states what the reader is holding,
// because the document is as likely to be piped into a tool or handed to an
// agent as it is to be read by the person who ran the command.
const markdownHeader = `# Terragrunt Catalog

Every component that ` + "`terragrunt catalog`" + ` discovers has a section below, holding the metadata the catalog user interface shows for it, followed by the component's README.

Each section records a component source, which is where the component is scaffolded from. ` + "`terragrunt scaffold`" + ` takes one for a module or a template; the catalog user interface scaffolds a unit or a stack by copying.

Sections are written as components are discovered, so they interleave the repositories being loaded, and their order changes between runs. READMEs are reproduced inside fenced blocks, so a heading within one is not part of this document's structure.

The document ends with an index of the components it holds and a count of what was discovered, which is how a reader tells a complete document from one that was cut short.

`

// MarkdownRenderer writes the catalog as a Markdown document: a header, a
// section per component, and a closing index of everything it wrote.
type MarkdownRenderer struct {
	index []indexRow
}

// indexRow is what a component contributes to the closing index. Three fields
// per component is what a table at the end of a streamed document costs; the
// READMEs, which are the bulk of the output, are written and forgotten.
type indexRow struct {
	title  string
	kind   string
	source string
}

// NewMarkdownRenderer returns a renderer that emits Markdown.
func NewMarkdownRenderer() *MarkdownRenderer {
	return &MarkdownRenderer{}
}

// Open implements [Renderer], writing the document header.
func (r *MarkdownRenderer) Open(w io.Writer) error {
	if _, err := io.WriteString(w, markdownHeader); err != nil {
		return err
	}

	return flush(w)
}

// Entry implements [Renderer], writing e as one section.
func (r *MarkdownRenderer) Entry(w io.Writer, e *tui.ComponentEntry) error {
	entry := NewEntry(e)
	title := oneLine(entry.Title)

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "## %s\n\n", title)

	// The description is the component's own prose, so it opens the section as
	// prose rather than as a row in the table of facts below it.
	if entry.Description != "" {
		fmt.Fprintf(&buf, "%s\n\n", paragraph(entry.Description))
	}

	buf.WriteString(fieldTableHeader)

	for _, f := range entryFields(entry) {
		// A row is written only when the component has a value for it, so the
		// table carries what it has rather than a column of empty cells.
		if f.value == "" {
			continue
		}

		fmt.Fprintf(&buf, "| %s | %s |\n", f.label, f.value)
	}

	writeTags(&buf, entry.Tags)

	if entry.Doc != "" {
		buf.WriteString("\n")
		writeDoc(&buf, entry.Doc, docLanguageOf(e.Component))
	}

	buf.WriteString("\n")

	r.index = append(r.index, indexRow{
		title:  title,
		kind:   entry.Kind,
		source: entry.ComponentSource,
	})

	// Build the whole section before writing, for the reasons given in
	// [JSONLRenderer.Entry].
	return writeAndFlush(w, &buf)
}

// Close implements [Renderer], writing the index of what the document holds
// and the count that marks it complete.
func (r *MarkdownRenderer) Close(w io.Writer, summary Summary) error {
	var buf bytes.Buffer

	buf.WriteString("---\n\n")

	if summary.Entries == 0 {
		buf.WriteString("No components were discovered.\n")

		return writeAndFlush(w, &buf)
	}

	buf.WriteString(indexTableHeader)

	for _, row := range r.index {
		fmt.Fprintf(&buf, "| %s | %s | %s |\n", cell(row.title), code(row.kind), code(row.source))
	}

	fmt.Fprintf(
		&buf, "\nDiscovered %s from %s.\n",
		plural(summary.Entries, "component"),
		plural(summary.Sources, "source"),
	)

	return writeAndFlush(w, &buf)
}

// writeAndFlush pushes a built-up chunk of the document to the consumer.
func writeAndFlush(w io.Writer, buf *bytes.Buffer) error {
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}

	return flush(w)
}

// field is one row of a component's metadata table.
type field struct {
	label string
	value string
}

// entryFields returns a component's metadata rows in the order they are
// written.
func entryFields(e *Entry) []field {
	return []field{
		{label: "Kind", value: code(e.Kind)},
		{label: "Source", value: code(e.Source)},
		{label: "Directory", value: code(e.Dir)},
		{label: "Version", value: code(e.Version)},
		{label: "URL", value: autolink(e.URL)},
		{label: "Component source", value: code(e.ComponentSource)},
	}
}

// writeTags writes a component's tags as a table of their own. A component
// can carry any number of them, and a row each stays readable where a single
// cell holding the whole list would not.
func writeTags(buf *bytes.Buffer, tags []string) {
	if len(tags) == 0 {
		return
	}

	buf.WriteString("\n" + tagTableHeader)

	for _, tag := range tags {
		fmt.Fprintf(buf, "| %s |\n", code(tag))
	}
}

// docLanguage is the info string a README's fenced block carries.
type docLanguage string

const (
	// docPlain leaves a README that isn't Markdown (AsciiDoc, today) with a
	// bare fence, rather than an info string claiming a language it isn't
	// written in.
	docPlain docLanguage = ""

	docMarkdown docLanguage = "markdown"
)

// docLanguageOf returns the info string to fence c's README with.
func docLanguageOf(c *tui.Component) docLanguage {
	if c.IsMarkDown() {
		return docMarkdown
	}

	return docPlain
}

// writeDoc writes a component's README inside a fenced block, so that the
// headings and thematic breaks a README carries cannot be read as part of the
// surrounding document.
func writeDoc(buf *bytes.Buffer, doc string, lang docLanguage) {
	fence := docFence(doc)

	buf.WriteString(fence)
	buf.WriteString(string(lang))
	buf.WriteString("\n")
	buf.WriteString(doc)

	if !strings.HasSuffix(doc, "\n") {
		buf.WriteString("\n")
	}

	buf.WriteString(fence)
	buf.WriteString("\n")
}

// docFence returns a fence long enough to hold doc. A fenced block ends at
// the first fence at least as long as the one that opened it, so a README
// containing fences of its own needs a longer one around it.
func docFence(doc string) string {
	longest, run := 0, 0

	for _, r := range doc {
		if r != '`' {
			run = 0

			continue
		}

		run++
		longest = max(longest, run)
	}

	return strings.Repeat("`", max(minFenceLength, longest+1))
}

// oneLine collapses whitespace so that a value taken from README front matter
// stays on the line the document put it on.
func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// blockStarters open a block construct when they open a line: a heading, a
// fence, a thematic break, a list, a quote, or raw HTML.
const blockStarters = "#>`~<-_*+"

// paragraph renders a value as prose of its own, escaping a leading character
// that would otherwise open a block. A value the component wrote must not add
// a section to the document, or a fence that swallows the rest of it.
func paragraph(value string) string {
	line := oneLine(value)
	if line == "" {
		return ""
	}

	if strings.ContainsAny(line[:1], blockStarters) {
		return `\` + line
	}

	return line
}

// cell renders a value as the content of a table cell. A pipe would end the
// cell, and a line break would end the row, so neither survives as itself.
// Every value this file writes lands in a cell, so the wrappers below escape
// as they wrap and nothing escapes twice. The escape holds inside a code span
// too, which is where most of these values end up.
func cell(value string) string {
	return strings.ReplaceAll(oneLine(value), "|", `\|`)
}

// code wraps value in a code span, leaving an empty value empty so the caller
// can drop its row.
func code(value string) string {
	if value == "" {
		return ""
	}

	return "`" + cell(value) + "`"
}

// autolink wraps a URL in the angle brackets that make it a link without a
// separate label.
func autolink(url string) string {
	if url == "" {
		return ""
	}

	return "<" + cell(url) + ">"
}

// plural renders a count with the singular or plural form of unit.
func plural(count int, unit string) string {
	if count == 1 {
		return "1 " + unit
	}

	return strconv.Itoa(count) + " " + unit + "s"
}
