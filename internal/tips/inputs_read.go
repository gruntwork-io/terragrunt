package tips

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/patch"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
)

// InputsRead is a call to a file-reading function in an inputs attribute, as the
// InputsReadNotTracked tip shows it.
type InputsRead struct {
	// File is the configuration file the call is in, spelled as the tip prints it.
	File string
	// Function is the name of the function called.
	Function string
	// Source is the content of File.
	Source []byte
	// Range is the expression the tip points at in Source: the path argument of a templatefile
	// call, or the whole call of any other function. Its Filename is File.
	Range hcl.Range
	// Place is the kind of configuration the call is in.
	Place InputsReadPlace
}

// InputsReadPlace is the kind of configuration an inputs read is in, which picks the advice the
// tip gives for it.
type InputsReadPlace int

const (
	// InputsReadInUnit is a read in the inputs attribute of a unit config or of a file it includes.
	InputsReadInUnit InputsReadPlace = iota
	// InputsReadInAutoInclude is a read in the inputs attribute of an autoinclude block in a stack file.
	InputsReadInAutoInclude
)

// maxListedInputsReads caps the reads one tip lists, so a repository that reads a file in
// every unit's inputs does not fill the terminal.
const maxListedInputsReads = 10

// GiveInputsReadTip emits the InputsReadNotTracked tip listing reads, rendered for the terminal
// v reports and without color when l has colors disabled. It does nothing when reads is empty.
func GiveInputsReadTip(l log.Logger, v *venv.Venv, reads []InputsRead, allTips Tips) {
	if len(reads) == 0 {
		return
	}

	render := view.NewHumanRender(v, l.Formatter().DisabledColors())

	msg, err := InputsReadMessage(render, reads)
	if err != nil {
		l.Debugf("Skipping the %s tip: %v", InputsReadNotTracked, err)

		return
	}

	allTips.Find(InputsReadNotTracked).EvaluateWith(l, msg)
}

// InputsReadMessage returns the InputsReadNotTracked message for reads: each of the first
// maxListedInputsReads of them as a diagnostic render draws pointing at the call, a count of the
// rest, then a link to the docs, as paragraphs.
func InputsReadMessage(render view.Render, reads []InputsRead) (string, error) {
	var b strings.Builder

	b.WriteString("Terragrunt does not track files read in `inputs`, " +
		"so reading and Git filters miss the units and stacks that read them.\n")

	for i := range min(len(reads), maxListedInputsReads) {
		read := &reads[i]

		diag, err := diagnostic.NewDiagnostic(&hcl.File{Bytes: read.Source}, &hcl.Diagnostic{
			Severity: hcl.DiagInvalid,
			Summary:  read.Place.summary(),
			Detail:   read.advice(),
			Subject:  &read.Range,
		})
		if err != nil {
			return "", err
		}

		rendered, err := render.Diagnostics(diagnostic.Diagnostics{diag})
		if err != nil {
			return "", err
		}

		fmt.Fprintf(&b, "\n%s\n", strings.Trim(rendered, "\n"))
	}

	if rest := len(reads) - maxListedInputsReads; rest > 0 {
		fmt.Fprintf(&b, "\n... and %d more\n", rest)
	}

	b.WriteString("\nSee https://docs.terragrunt.com/features/filter/attributes#reading-based-expressions")

	return b.String(), nil
}

// summary is the heading of the diagnostic for a read in place.
func (place InputsReadPlace) summary() string {
	if place == InputsReadInAutoInclude {
		return "Untracked file read in an `autoinclude` block's `inputs`"
	}

	return "Untracked file read in `inputs`"
}

// advice is the detail of the diagnostic for read: where to move what it points at. A templatefile
// call may render dependency outputs, which locals cannot reference, so the call stays in inputs and
// only its path moves, marked as read.
func (read *InputsRead) advice() string {
	locals := "`locals`"
	if read.Place == InputsReadInAutoInclude {
		locals = "the stack's `locals`"
	}

	if read.Function == patch.FuncNameTemplateFile {
		return fmt.Sprintf(
			"Mark the template as read with `mark_as_read` in %s, then pass the local as the path here.",
			locals,
		)
	}

	return fmt.Sprintf("Read the file in %s and reference the local here.", locals)
}
