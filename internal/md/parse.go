package md

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Kind names a sort of block. A block this package has no name for names
// itself, so a caller that asserts on a kind can still say what it found.
type Kind string

// KindParagraph is prose: what a value written as text becomes.
const KindParagraph Kind = "Paragraph"

// Doc is a parsed Markdown document. It keeps the source it was parsed from,
// because a parse records where each piece of the document is rather than
// copying the text out of it.
type Doc struct {
	root   ast.Node
	source []byte
}

// Parse reads source as Markdown.
func Parse(source string) *Doc {
	src := []byte(source)

	return &Doc{root: newParser().Parse(text.NewReader(src)), source: src}
}

// Headings returns the text of every heading in the document, in the order it
// writes them, so a caller can ask what a document is made of rather than
// which lines it happens to contain.
func (d *Doc) Headings() []string {
	var titles []string

	walk(d.root, func(n ast.Node) bool {
		heading, ok := n.(*ast.Heading)
		if !ok {
			return false
		}

		titles = append(titles, d.headingText(heading))

		return true
	})

	return titles
}

// CodeSpans returns the content of every code span in the document, so a
// caller can ask what a value became rather than which characters it was
// written as.
func (d *Doc) CodeSpans() []string {
	var spans []string

	walk(d.root, func(n ast.Node) bool {
		span, ok := n.(*ast.CodeSpan)
		if !ok {
			return false
		}

		spans = append(spans, d.spanText(span))

		return true
	})

	return spans
}

// Autolinks returns the target of every autolink in the document, so a caller
// can ask whether a value became a link rather than text that merely looks
// like one.
func (d *Doc) Autolinks() []string {
	var targets []string

	walk(d.root, func(n ast.Node) bool {
		link, ok := n.(*ast.AutoLink)
		if !ok {
			return false
		}

		targets = append(targets, string(link.URL(d.source)))

		return true
	})

	return targets
}

// TableRowCells returns the number of cells in each body row of the document's
// tables, so a caller can ask whether a value holding a pipe stayed inside the
// cell it was written to.
func (d *Doc) TableRowCells() []int {
	var rows []int

	walk(d.root, func(n ast.Node) bool {
		row, ok := n.(*extast.TableRow)
		if !ok {
			return false
		}

		rows = append(rows, row.ChildCount())

		return true
	})

	return rows
}

// BlockAfter returns the block that follows the heading titled title. It
// reports false when the document holds no such heading, and when the heading
// closes the document.
func (d *Doc) BlockAfter(title string) (Block, bool) {
	for n := d.root.FirstChild(); n != nil; n = n.NextSibling() {
		heading, ok := n.(*ast.Heading)
		if !ok || d.headingText(heading) != title {
			continue
		}

		next := n.NextSibling()
		if next == nil {
			return Block{}, false
		}

		return Block{doc: d, node: next}, true
	}

	return Block{}, false
}

// Block is one of the blocks a document is built from: a paragraph, a fenced
// block, a table, or any other construct that stands on its own.
type Block struct {
	doc  *Doc
	node ast.Node
}

// Kind reports what sort of block b is.
func (b Block) Kind() Kind {
	return Kind(b.node.Kind().String())
}

// Text returns b's text as a reader sees it.
func (b Block) Text() string {
	return b.doc.text(b.node)
}

// newParser returns the parser a document is read with. Tables are the one
// extension enabled; linkification in particular stays off, so a caller asking
// whether a value became a link is answered by the document rather than by the
// parser. A parser per document keeps concurrent reads off a shared one.
func newParser() parser.Parser {
	return goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()
}

// headingText returns a heading's own lines, which is the source text between
// the marker that opens the heading and the end of it.
func (d *Doc) headingText(heading *ast.Heading) string {
	return string(heading.Lines().Value(d.source))
}

// text returns the text under n as a reader sees it: an escape resolves to the
// character it was holding off, and a character reference to the character it
// names. Markup that carries no text of its own, such as an image or raw HTML,
// contributes nothing.
func (d *Doc) text(n ast.Node) string {
	var content strings.Builder

	d.writeText(&content, n)

	return content.String()
}

func (d *Doc) writeText(content *strings.Builder, n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch node := c.(type) {
		case *ast.Text:
			content.Write(resolveEscapes(node.Value(d.source)))

			if node.SoftLineBreak() || node.HardLineBreak() {
				content.WriteString("\n")
			}
		case *ast.CodeSpan:
			content.WriteString(d.spanText(node))
		case *ast.String:
			content.Write(node.Value)
		case *ast.AutoLink:
			content.Write(node.URL(d.source))
		default:
			d.writeText(content, c)
		}
	}
}

// spanText returns a code span's content, which the document holds as it was
// written: an escape inside one is text like any other.
func (d *Doc) spanText(span *ast.CodeSpan) string {
	var content strings.Builder

	for c := span.FirstChild(); c != nil; c = c.NextSibling() {
		node, ok := c.(*ast.Text)
		if !ok {
			continue
		}

		content.Write(node.Value(d.source))
	}

	return content.String()
}

// resolveEscapes returns text as it reads rather than as it was written. The
// parse leaves the source untouched, so an escape and a character reference
// both survive it as the characters that spell them.
func resolveEscapes(value []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(value)))
}

// walk visits every node under n, outermost first. visit reports whether the
// node it was given holds nothing else the walk needs to see.
func walk(n ast.Node, visit func(n ast.Node) bool) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if visit(c) {
			continue
		}

		walk(c, visit)
	}
}
