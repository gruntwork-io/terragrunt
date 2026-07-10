package tui

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
)

const (
	// previewByteLimit caps how many leading bytes of a file are read and
	// highlighted. The detail pane only ever shows the head of a file, so a
	// larger file is previewed from its first previewByteLimit bytes rather than
	// refused; the rest is never read.
	previewByteLimit = 256 << 10

	// binarySniffLen is how many leading bytes are scanned for a NUL byte to
	// decide a file is binary and skip previewing it. Judging from the head lets a
	// large mostly-text file with a stray NUL deep inside still preview.
	binarySniffLen = 1024

	// chromaDarkStyle is the chroma syntax theme used for the file preview on
	// dark terminals.
	chromaDarkStyle = "monokai"
	// chromaLightStyle is the chroma syntax theme used for the file preview on
	// light terminals.
	chromaLightStyle = "github"

	// chromaFormatter is chroma's 256-color terminal formatter: the
	// widest-supported option that still carries the theme's colors.
	chromaFormatter = "terminal256"
)

// previewTheme selects how a file preview is colored: no color at all, or a
// syntax theme matched to the terminal background.
type previewTheme int

const (
	// previewPlain renders the raw source with no coloring.
	previewPlain previewTheme = iota
	// previewDark colors the source for a dark terminal.
	previewDark
	// previewLight colors the source for a light terminal.
	previewLight
)

// themeFor maps the color setting and terminal background to a preview theme.
func themeFor(color ColorMode, dark bool) previewTheme {
	switch {
	case color == ColorDisabled:
		return previewPlain
	case dark:
		return previewDark
	default:
		return previewLight
	}
}

// renderFilePreview reads and renders a file for the detail pane: Markdown
// through glamour, everything else through chroma syntax highlighting. width is
// the pane's interior width, used for Markdown word-wrap. Only the file's first
// previewLimit bytes are read, since the pane shows just its head. It returns a
// short dimmed placeholder for files that can't or shouldn't be previewed.
func (m Model) renderFilePreview(n *Node, width int) string {
	data, err := vfs.ReadFileLimit(m.fs, n.absPath, m.previewLimit)
	if err != nil {
		return dimStyle.Render("(unreadable)")
	}

	if isBinary(data, m.sniffLen) {
		return dimStyle.Render("(binary file)")
	}

	// The Markdown and syntax renderers, and the lipgloss display path, panic on
	// invalid UTF-8, so coerce hostile file bytes before any of them see it.
	source := strings.ToValidUTF8(string(data), "�")
	theme := themeFor(m.color, m.hasDarkBG)

	switch strings.ToLower(filepath.Ext(n.name)) {
	case ".md", ".markdown":
		return renderMarkdown(source, width, theme)
	default:
		return highlightCode(n.name, source, theme)
	}
}

// isBinary reports whether data looks like a binary file, sniffing up to its
// first sniffLen bytes for a NUL.
func isBinary(data []byte, sniffLen int) bool {
	if len(data) > sniffLen {
		data = data[:sniffLen]
	}

	return bytes.IndexByte(data, 0) >= 0
}

// renderMarkdown renders Markdown source to styled terminal output wrapped at
// width. With color off, or on any renderer error, it returns the raw source.
func renderMarkdown(source string, width int, theme previewTheme) string {
	if theme == previewPlain {
		return source
	}

	r, err := viewtui.NewMarkdownRenderer(width, theme == previewDark)
	if err != nil {
		return source
	}

	out, err := r.Render(source)
	if err != nil {
		return source
	}

	return strings.TrimRight(out, "\n")
}

// highlightCode syntax-highlights source for terminal output, choosing a lexer
// by filename and falling back to content analysis. With color off, or on any
// error, it returns the raw source.
func highlightCode(name, source string, theme previewTheme) string {
	if theme == previewPlain {
		return source
	}

	lexer := chroma.Coalesce(lexerFor(name, source))
	style := styleFor(theme)
	formatter := previewFormatter()

	iterator, err := lexer.Tokenise(nil, source)
	if err != nil {
		return source
	}

	var buf strings.Builder
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return source
	}

	return strings.TrimRight(buf.String(), "\n")
}

// lexerFor picks a chroma lexer for a file: by filename first, then by content
// analysis, falling back to the plain-text lexer when neither matches.
func lexerFor(name, source string) chroma.Lexer {
	if lexer := lexers.Match(name); lexer != nil {
		return lexer
	}

	if lexer := lexers.Analyse(source); lexer != nil {
		return lexer
	}

	return lexers.Fallback
}

// styleFor returns the chroma style matching the preview theme, falling back to
// chroma's default when the configured theme isn't registered.
func styleFor(theme previewTheme) *chroma.Style {
	name := chromaDarkStyle
	if theme == previewLight {
		name = chromaLightStyle
	}

	if style := styles.Get(name); style != nil {
		return style
	}

	return styles.Fallback
}

// previewFormatter returns chroma's terminal formatter, falling back to the
// default when it isn't registered.
func previewFormatter() chroma.Formatter {
	if formatter := formatters.Get(chromaFormatter); formatter != nil {
		return formatter
	}

	return formatters.Fallback
}
