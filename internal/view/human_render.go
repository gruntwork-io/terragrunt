package view

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/mitchellh/go-wordwrap"
)

const (
	defaultWidth = 78

	// ansiReset clears any pending styling so that subsequent output is not
	// tinted by ANSI codes inherited from earlier writers (notably the
	// OpenTofu/Terraform UI helper).
	ansiReset = "\x1b[0m"
)

var (
	boldStyle       = lipgloss.NewStyle().Bold(true)
	boldRedStyle    = boldStyle.Foreground(lipgloss.Color("1"))
	boldYellowStyle = boldStyle.Foreground(lipgloss.Color("3"))
	boldBlueStyle   = boldStyle.Foreground(lipgloss.Color("4"))
)

// MissingSnippetError reports a diagnostic that points at a position in Filename without the source
// snippet the human rendering draws under it.
type MissingSnippetError struct {
	Filename string
}

func (err MissingSnippetError) Error() string {
	return fmt.Sprintf("diagnostic points into %s but has no source snippet to show", err.Filename)
}

type HumanRender struct {
	width        int
	disableColor bool
}

func NewHumanRender(v *venv.Venv, disableColor bool) Render {
	v.RequireTerminal()

	disableColor = disableColor || !v.Terminal.StderrIsTTY()

	width := v.Terminal.Width()
	if width == 0 {
		width = defaultWidth
	}

	return &HumanRender{
		disableColor: disableColor,
		width:        width,
	}
}

// styled renders text in the given style, or returns it unchanged when colors
// are disabled.
func (render *HumanRender) styled(style *lipgloss.Style, text string) string {
	if render.disableColor {
		return text
	}

	return style.Render(text)
}

func (render *HumanRender) ShowConfigPath(filenames []string) (string, error) {
	var buf bytes.Buffer

	for _, filename := range filenames {
		buf.WriteString(filename)
		buf.WriteByte('\n')
	}

	return buf.String(), nil
}

// Diagnostics formats diags one after another, as [HumanRender.Diagnostic] formats each.
//
// Returns [MissingSnippetError] when one of diags has a Range and no Snippet, before formatting any.
func (render *HumanRender) Diagnostics(diags diagnostic.Diagnostics) (string, error) {
	for _, diag := range diags {
		if diag.Range != nil && diag.Snippet == nil {
			return "", MissingSnippetError{Filename: diag.Range.Filename}
		}
	}

	var buf bytes.Buffer

	for _, diag := range diags {
		buf.WriteString(render.Diagnostic(diag))
		buf.WriteByte('\n')
	}

	return buf.String(), nil
}

// Diagnostic formats a single diagnostic message, pointing at the source it reports.
//
// These diagnostics are formatted like so:
//
//	error: Missing dependency
//	 --> app/terragrunt.hcl:2:17
//	  |
//	2 |   config_path = "../db"
//	  |                 ^^^^^^^ This directory does not exist.
//
// The first line of the detail sits beside the pointer when it fits the terminal width. The rest of the
// detail follows the source on lines that start with `=`, as does a first line that does not fit.
//
// Panics when diag has a Range and no Snippet. [HumanRender.Diagnostics] returns [MissingSnippetError]
// for such a diagnostic first, and [diagnostic.NewDiagnostic] never builds one.
func (render *HumanRender) Diagnostic(diag *diagnostic.Diagnostic) string {
	if diag.Range != nil && diag.Snippet == nil {
		panic("view: Diagnostic called with a Range and no Snippet; Diagnostics rejects that first")
	}

	var (
		buf   strings.Builder
		style = &boldStyle
	)

	switch hcl.DiagnosticSeverity(diag.Severity) {
	case hcl.DiagError:
		style = &boldRedStyle

		buf.WriteString(render.styled(style, "error"))
		buf.WriteString(render.styled(&boldStyle, ": "))
	case hcl.DiagWarning:
		style = &boldYellowStyle

		buf.WriteString(render.styled(style, "warning"))
		buf.WriteString(render.styled(&boldStyle, ": "))
	case hcl.DiagInvalid:
		fallthrough
	default:
		buf.WriteByte('\n')

		if !render.disableColor {
			buf.WriteString(ansiReset)
		}
	}

	buf.WriteString(render.styled(&boldStyle, diag.Summary))
	buf.WriteByte('\n')

	source := render.source(diag, style)

	buf.WriteString(source.text)
	buf.WriteString(render.notes(diag, source))

	return buf.String()
}

// renderedSource is the source excerpt of a diagnostic.
type renderedSource struct {
	// text is the rendered location and source lines.
	text string
	// gutter is the width of the line number column.
	gutter int
	// labelShown reports whether the first line of the detail is beside the pointer.
	labelShown bool
}

// source renders the location of diag and the source lines it points at.
func (render *HumanRender) source(
	diag *diagnostic.Diagnostic,
	pointerStyle *lipgloss.Style,
) renderedSource {
	// [diagnostic.NewDiagnostic] leaves Range nil for an HCL diagnostic that has no
	// Subject, meaning it points at no position in the configuration.
	if diag.Range == nil {
		return renderedSource{gutter: 1}
	}

	var (
		buf      strings.Builder
		snippet  = diag.Snippet
		lastLine = snippet.StartLine + strings.Count(snippet.Code, "\n")
		gutter   = len(strconv.Itoa(lastLine))
	)

	margin := strings.Repeat(" ", gutter)

	fmt.Fprintf(
		&buf,
		"%s%s %s:%d:%d\n",
		margin,
		render.styled(&boldBlueStyle, "-->"),
		diag.Range.Filename,
		diag.Range.Start.Line,
		diag.Range.Start.Column,
	)

	bar := render.styled(&boldBlueStyle, "|")

	fmt.Fprintf(&buf, "%s %s\n", margin, bar)

	var (
		lines    = strings.Split(snippet.Code, "\n")
		pointers = sourcePointers(
			lines,
			snippet.HighlightStartOffset,
			snippet.HighlightEndOffset,
		)
		detail, _, _ = strings.Cut(diag.Detail, "\n")
		labelAt      = -1
		shown        bool
	)

	for i, pointer := range pointers {
		if pointer.carets != "" {
			labelAt = i
		}
	}

	for i, line := range lines {
		number := fmt.Sprintf("%*d", gutter, snippet.StartLine+i)

		fmt.Fprintf(&buf, "%s %s %s\n", render.styled(&boldBlueStyle, number), bar, line)

		pointer := pointers[i]
		if pointer.carets == "" {
			continue
		}

		label := ""

		if i == labelAt && render.fitsBeside(gutter, pointer, detail) {
			label = " " + detail
			shown = true
		}

		fmt.Fprintf(
			&buf,
			"%s %s %s%s\n",
			margin,
			bar,
			pointer.padding,
			render.styled(pointerStyle, pointer.carets+label),
		)
	}

	return renderedSource{text: buf.String(), gutter: gutter, labelShown: shown}
}

// fitsBeside reports whether label fits on the pointer line within the terminal width.
func (render *HumanRender) fitsBeside(gutter int, pointer sourcePointer, label string) bool {
	if label == "" {
		return false
	}

	const separators = len(" | ") + len(" ")

	used := gutter + separators + lipgloss.Width(pointer.padding) + len(pointer.carets)

	return used+lipgloss.Width(label) <= render.width
}

// notes renders what follows the source: the part of the detail that is not beside the pointer, then the
// function call and the expression values that led to the diagnostic.
func (render *HumanRender) notes(diag *diagnostic.Diagnostic, source renderedSource) string {
	var notes []string

	detail := diag.Detail
	if source.labelShown {
		_, detail, _ = strings.Cut(detail, "\n")
	}

	if detail != "" {
		notes = append(notes, detail)
	}

	notes = append(notes, snippetNotes(diag.Snippet)...)
	if len(notes) == 0 {
		return ""
	}

	var (
		buf    strings.Builder
		margin = strings.Repeat(" ", source.gutter)
		indent = margin + "   "
	)

	if diag.Snippet != nil {
		fmt.Fprintf(&buf, "%s %s\n", margin, render.styled(&boldBlueStyle, "|"))
	}

	for _, note := range notes {
		prefix := margin + " " + render.styled(&boldBlueStyle, "=") + " "

		for _, line := range render.wrap(note, len(indent)) {
			buf.WriteString(strings.TrimRight(prefix+line, " "))
			buf.WriteByte('\n')

			prefix = indent
		}
	}

	return buf.String()
}

// snippetNotes returns the function call and the expression values that snippet records, one per note.
func snippetNotes(snippet *diagnostic.Snippet) []string {
	if snippet == nil {
		return nil
	}

	var notes []string

	if call := snippet.FunctionCall; call != nil && call.Signature != nil {
		notes = append(notes, "while calling "+callSignature(call))
	}

	values := slices.Clone(snippet.Values)
	slices.SortFunc(values, func(a, b diagnostic.ExpressionValue) int {
		return strings.Compare(a.Traversal, b.Traversal)
	})

	for _, value := range values {
		notes = append(notes, value.Traversal+" "+value.Statement)
	}

	return notes
}

// wrap splits text into lines that fit the terminal width after an indent. A line that starts with a
// space is preformatted and stays as it is.
func (render *HumanRender) wrap(text string, indent int) []string {
	width := render.width - indent

	var lines []string

	for line := range strings.SplitSeq(text, "\n") {
		if width <= 0 || strings.HasPrefix(line, " ") {
			lines = append(lines, line)

			continue
		}

		lines = append(lines, strings.Split(wordwrap.WrapString(line, uint(width)), "\n")...)
	}

	return lines
}

// callSignature returns the name a function was called as with its parameter names.
func callSignature(call *diagnostic.FunctionCall) string {
	params := make([]string, 0, len(call.Signature.Params)+1)

	for _, param := range call.Signature.Params {
		params = append(params, param.Name)
	}

	if param := call.Signature.VariadicParam; param != nil {
		params = append(params, param.Name+"...")
	}

	return call.CalledAs + "(" + strings.Join(params, ", ") + ")"
}

// sourcePointer underlines part of a source line. It has no carets for a line that is not pointed at.
type sourcePointer struct {
	padding string
	carets  string
}

// sourcePointers returns, for each line, the pointer that underlines the part of it between the offsets
// start and end into the joined lines. An end before start points at start.
func sourcePointers(lines []string, start, end int) []sourcePointer {
	pointers := make([]sourcePointer, len(lines))
	lineStart := 0
	end = max(end, start)

	for i, line := range lines {
		lineEnd := lineStart + len(line)
		from := min(max(start, lineStart), lineEnd)
		to := min(max(end, lineStart), lineEnd)

		pointsPastEnd := from == to && start >= lineStart && start <= lineEnd

		if trimmed := strings.TrimLeft(line[from-lineStart:to-lineStart], " \t"); trimmed != "" {
			from = to - len(trimmed)
		}

		if from < to || pointsPastEnd {
			carets := max(lipgloss.Width(line[from-lineStart:to-lineStart]), 1)
			pointers[i] = sourcePointer{
				padding: pointerPadding(line[:from-lineStart]),
				carets:  strings.Repeat("^", carets),
			}
		}

		lineStart = lineEnd + len("\n")
	}

	return pointers
}

// pointerPadding returns whitespace as wide as prefix. Tabs stay tabs so the carets line up at any tab width.
func pointerPadding(prefix string) string {
	var padding strings.Builder

	for _, r := range prefix {
		if r == '\t' {
			padding.WriteRune(r)

			continue
		}

		padding.WriteString(strings.Repeat(" ", lipgloss.Width(string(r))))
	}

	return padding.String()
}
