package view

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/hashicorp/hcl/v2"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/term"
)

const (
	defaultWidth = 78

	// ansiReset clears any pending styling so that subsequent output is not
	// tinted by ANSI codes inherited from earlier writers (notably the
	// Terraform UI helper).
	ansiReset = "\x1b[0m"
)

//nolint:gochecknoglobals
var (
	redStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellowStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	boldStyle          = lipgloss.NewStyle().Bold(true)
	boldRedStyle       = boldStyle.Foreground(lipgloss.Color("1"))
	boldYellowStyle    = boldStyle.Foreground(lipgloss.Color("3"))
	darkGrayStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	underlineWhiteText = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("7"))
)

type HumanRender struct {
	width        int
	disableColor bool
}

func NewHumanRender(disableColor bool) Render {
	disableColor = disableColor || !term.IsTerminal(int(os.Stderr.Fd()))

	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
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

func (render *HumanRender) Diagnostics(diags diagnostic.Diagnostics) (string, error) {
	var buf bytes.Buffer

	for _, diag := range diags {
		str, err := render.Diagnostic(diag)
		if err != nil {
			return "", err
		}

		if str != "" {
			buf.WriteString(str)
			buf.WriteByte('\n')
		}
	}

	return buf.String(), nil
}

// Diagnostic formats a single diagnostic message.
func (render *HumanRender) Diagnostic(diag *diagnostic.Diagnostic) (string, error) {
	var buf bytes.Buffer

	// these leftRule* variables are markers for the beginning of the lines
	// containing the diagnostic that are intended to help sighted users
	// better understand the information hierarchy when diagnostics appear
	// alongside other information or alongside other diagnostics.
	//
	// Without this, it seems (based on folks sharing incomplete messages when
	// asking questions, or including extra content that's not part of the
	// diagnostic) that some readers have trouble easily identifying which
	// text belongs to the diagnostic and which does not.
	var (
		leftRuleLine, leftRuleStart, leftRuleEnd string
		leftRuleWidth                            int // in visual character cells
	)

	// TODO: Remove lint suppression
	switch hcl.DiagnosticSeverity(diag.Severity) { //nolint:exhaustive
	case hcl.DiagError:
		buf.WriteString(render.styled(&boldRedStyle, "Error: "))
		leftRuleLine = render.styled(&redStyle, "│") + " "
		leftRuleStart = render.styled(&redStyle, "╷")
		leftRuleEnd = render.styled(&redStyle, "╵")
		leftRuleWidth = 2
	case hcl.DiagWarning:
		buf.WriteString(render.styled(&boldYellowStyle, "Warning: "))
		leftRuleLine = render.styled(&yellowStyle, "│") + " "
		leftRuleStart = render.styled(&yellowStyle, "╷")
		leftRuleEnd = render.styled(&yellowStyle, "╵")
		leftRuleWidth = 2
	default:
		// Clear out any coloring that might be applied by Terraform's UI helper,
		// so our result is not context-sensitive.
		buf.WriteByte('\n')

		if !render.disableColor {
			buf.WriteString(ansiReset)
		}
	}

	// We don't wrap the summary, since we expect it to be terse, and since
	// this is where we put the text of a native Go error it may not always
	// be pure text that lends itself well to word-wrapping.
	if _, err := fmt.Fprintf(&buf, "%s\n\n", render.styled(&boldStyle, diag.Summary)); err != nil {
		return "", err
	}

	sourceSnippets, err := render.SourceSnippets(diag)
	if err != nil {
		return "", err
	}

	buf.WriteString(sourceSnippets)

	if diag.Detail != "" {
		paraWidth := render.width - leftRuleWidth - 1 // leave room for the left rule
		if paraWidth > 0 {
			lines := strings.SplitSeq(diag.Detail, "\n")
			for line := range lines {
				if !strings.HasPrefix(line, " ") {
					line = wordwrap.WrapString(line, uint(paraWidth))
				}

				if _, err := fmt.Fprintf(&buf, "%s\n", line); err != nil {
					return "", err
				}
			}
		} else {
			if _, err := fmt.Fprintf(&buf, "%s\n", diag.Detail); err != nil {
				return "", err
			}
		}
	}

	// Before we return, we'll finally add the left rule prefixes to each
	// line so that the overall message is visually delimited from what's
	// around it. We'll do that by scanning over what we already generated
	// and adding the prefix for each line.
	var ruleBuf strings.Builder

	sc := bufio.NewScanner(&buf)

	ruleBuf.WriteString(leftRuleStart)
	ruleBuf.WriteByte('\n')

	for sc.Scan() {
		prefix := leftRuleLine

		line := sc.Text()
		if line == "" {
			// Don't print the space after the line if there would be nothing
			// after it anyway.
			prefix = strings.TrimSpace(prefix)
		}

		ruleBuf.WriteString(prefix)
		ruleBuf.WriteString(line)
		ruleBuf.WriteByte('\n')
	}

	ruleBuf.WriteString(leftRuleEnd)

	return ruleBuf.String(), nil
}

func (render *HumanRender) SourceSnippets(diag *diagnostic.Diagnostic) (string, error) {
	if diag.Range == nil || diag.Snippet == nil {
		// This should generally not happen, as long as sources are always
		// loaded through the main loader. We may load things in other
		// ways in weird cases, so we'll tolerate it at the expense of
		// a not-so-helpful error message.
		return fmt.Sprintf(
			"  on %s line %d:\n  (source code not available)\n",
			diag.Range.Filename,
			diag.Range.Start.Line,
		), nil
	}

	var (
		buf     = new(bytes.Buffer)
		snippet = diag.Snippet
		code    = snippet.Code
	)

	var contextStr string
	if snippet.Context != "" {
		contextStr = ", in " + snippet.Context
	}

	if _, err := fmt.Fprintf(
		buf,
		"  on %s line %d%s:\n",
		diag.Range.Filename,
		diag.Range.Start.Line,
		contextStr,
	); err != nil {
		return "", err
	}

	// Split the snippet and render the highlighted section with underlines
	start := snippet.HighlightStartOffset
	end := snippet.HighlightEndOffset

	// Only buggy diagnostics can have an end range before the start, but
	// we need to ensure we don't crash here if that happens.
	if end < start {
		end = min(start+1, len(code))
	}

	// If either start or end is out of range for the code buffer then
	// we'll cap them at the bounds just to avoid a panic, although
	// this would happen only if there's a bug in the code generating
	// the snippet objects.
	if start < 0 {
		start = 0
	} else if start > len(code) {
		start = len(code)
	}

	if end < 0 {
		end = 0
	} else if end > len(code) {
		end = len(code)
	}

	before, highlight, after := code[0:start], code[start:end], code[end:]
	code = before + render.styled(&underlineWhiteText, highlight) + after

	// Split the snippet into lines and render one at a time
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if _, err := fmt.Fprintf(
			buf, "%4d: %s\n",
			snippet.StartLine+i,
			line,
		); err != nil {
			return "", err
		}
	}

	if len(snippet.Values) > 0 ||
		(snippet.FunctionCall != nil && snippet.FunctionCall.Signature != nil) {
		// The diagnostic may also have information about the dynamic
		// values of relevant variables at the point of evaluation.
		// This is particularly useful for expressions that get evaluated
		// multiple times with different values, such as blocks using
		// "count" and "for_each", or within "for" expressions.
		values := make([]diagnostic.ExpressionValue, len(snippet.Values))
		copy(values, snippet.Values)
		sort.Slice(values, func(i, j int) bool {
			return values[i].Traversal < values[j].Traversal
		})

		fmt.Fprintf(buf, "    %s\n", render.styled(&darkGrayStyle, "├────────────────"))

		if callInfo := snippet.FunctionCall; callInfo != nil && callInfo.Signature != nil {
			if _, err := fmt.Fprintf(buf, "    %s while calling %s(",
				render.styled(&darkGrayStyle, "│"),
				render.styled(&boldStyle, callInfo.CalledAs),
			); err != nil {
				return "", err
			}

			for i, param := range callInfo.Signature.Params {
				if i > 0 {
					buf.WriteString(", ")
				}

				buf.WriteString(param.Name)
			}

			if param := callInfo.Signature.VariadicParam; param != nil {
				if len(callInfo.Signature.Params) > 0 {
					buf.WriteString(", ")
				}

				buf.WriteString(param.Name)
				buf.WriteString("...")
			}

			buf.WriteString(")\n")
		}

		for _, value := range values {
			if _, err := fmt.Fprintf(buf, "    %s %s %s\n",
				render.styled(&darkGrayStyle, "│"),
				render.styled(&boldStyle, value.Traversal),
				value.Statement,
			); err != nil {
				return "", err
			}
		}
	}

	buf.WriteByte('\n')

	return buf.String(), nil
}
