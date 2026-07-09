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
	// previewByteLimit caps how much of a file is read and highlighted, bounding
	// the cost of previewing a large file.
	previewByteLimit = 256 << 10

	// binarySniffLen is how many leading bytes are scanned for a NUL byte to
	// decide a file is binary and skip previewing it.
	binarySniffLen = 1024

	// chromaDarkStyle and chromaLightStyle are the chroma syntax themes used for
	// the file preview on dark and light terminals.
	chromaDarkStyle  = "monokai"
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
func themeFor(color, dark bool) previewTheme {
	switch {
	case !color:
		return previewPlain
	case dark:
		return previewDark
	default:
		return previewLight
	}
}

// renderFilePreview reads and renders a file for the detail pane: Markdown
// through glamour, everything else through chroma syntax highlighting. width is
// the pane's interior width, used for Markdown word-wrap. theme mirrors the
// color setting and terminal background. It returns a short dimmed placeholder
// for files that can't or shouldn't be previewed.
func renderFilePreview(fs vfs.FS, n *Node, width int, theme previewTheme) string {
	info, err := fs.Stat(n.absPath)
	if err != nil {
		return dimStyle.Render("(unreadable)")
	}

	if info.Size() > previewByteLimit {
		return dimStyle.Render("(file too large to preview)")
	}

	data, err := vfs.ReadFile(fs, n.absPath)
	if err != nil {
		return dimStyle.Render("(unreadable)")
	}

	if isBinary(data) {
		return dimStyle.Render("(binary file)")
	}

	source := string(data)

	switch strings.ToLower(filepath.Ext(n.name)) {
	case ".md", ".markdown":
		return renderMarkdown(source, width, theme)
	default:
		return highlightCode(n.name, source, theme)
	}
}

// isBinary reports whether data looks like a binary file, sniffing the leading
// bytes for a NUL.
func isBinary(data []byte) bool {
	if len(data) > binarySniffLen {
		data = data[:binarySniffLen]
	}

	return bytes.IndexByte(data, 0) >= 0
}

// renderMarkdown renders Markdown source to styled terminal output wrapped at
// width. With color off, or on any renderer error, it returns the raw source.
func renderMarkdown(source string, width int, theme previewTheme) string {
	if theme == previewPlain || width <= 0 {
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

	lexer := lexers.Match(name)
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}

	if lexer == nil {
		lexer = lexers.Fallback
	}

	lexer = chroma.Coalesce(lexer)

	styleName := chromaDarkStyle
	if theme == previewLight {
		styleName = chromaLightStyle
	}

	style := styles.Get(styleName)
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get(chromaFormatter)
	if formatter == nil {
		formatter = formatters.Fallback
	}

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
