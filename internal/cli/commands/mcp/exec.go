package mcp

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/venv"
)

const (
	// execNotesMaxDistinct caps how many distinct denial notes one tool call
	// reports; a run_cmd in a root config included by N units would otherwise
	// produce O(N) near-identical degraded lines.
	execNotesMaxDistinct = 25

	// execCmdlineMaxLen caps the command-line excerpt embedded in a note so a
	// long argv cannot balloon the degraded list.
	execCmdlineMaxLen = 300
)

// execNote is one distinct degraded-result note and the number of invocations
// it stands for.
type execNote struct {
	text  string
	count int
}

// execRecorder accumulates the degraded-result notes for a single tool call,
// deduplicating identical notes (counting repeat invocations) and capping the
// number of distinct notes.
type execRecorder struct {
	refused    map[string]bool
	entries    []execNote
	suppressed int
	mu         sync.Mutex
}

// refuse notes that a program a configuration named was not allowed to run.
// Unlike the prose notes, the names feed an approval prompt, and
// [serverDeps.forCall] takes the accepted ones back.
func (r *execRecorder) refuse(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.refused == nil {
		r.refused = map[string]bool{}
	}

	r.refused[name] = true
}

// refusedCommands returns the distinct programs a configuration named that
// this call would not run, sorted so the same tree always produces the same
// prompt.
func (r *execRecorder) refusedCommands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Sorted(maps.Keys(r.refused))
}

// notes returns the degraded-result notes recorded so far, one line per
// distinct note with a repeat count, plus a summary line for notes suppressed
// by the distinct-notes cap.
func (r *execRecorder) notes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	notes := make([]string, 0, len(r.entries)+1)

	for _, e := range r.entries {
		if e.count == 1 {
			notes = append(notes, e.text)

			continue
		}

		notes = append(notes, fmt.Sprintf("%s (%d invocations)", e.text, e.count))
	}

	if r.suppressed > 0 {
		notes = append(
			notes,
			fmt.Sprintf(
				"%d further denied or stubbed invocations of other commands omitted",
				r.suppressed,
			),
		)
	}

	return notes
}

// record notes one denied or stubbed invocation, keeping the notes sorted.
// A parse records from several goroutines at once, so insertion order varies
// between runs of the same tree; sorting gives the degraded list the stable
// order [execRecorder.refusedCommands] already gives the approval prompt.
func (r *execRecorder) record(note string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	i, found := slices.BinarySearchFunc(
		r.entries,
		note,
		func(e execNote, text string) int { return strings.Compare(e.text, text) },
	)
	if found {
		r.entries[i].count++

		return
	}

	if len(r.entries) >= execNotesMaxDistinct {
		r.suppressed++

		return
	}

	r.entries = slices.Insert(r.entries, i, execNote{text: note, count: 1})
}

// observer returns the [venv.Policy] observer that turns each refusal into
// one of this call's degraded notes. A program a configuration named also
// feeds the approval prompt, whose wording depends on mode.
func (r *execRecorder) observer(mode venv.Refusal) func(venv.Denial) {
	return func(dn venv.Denial) {
		note := truncateRunes(dn.Cmdline, execCmdlineMaxLen)

		switch dn.Kind {
		case venv.DeniedIaC:
			r.record(fmt.Sprintf("denied %q: dependency outputs degraded to mock_outputs where configured", note))
		case venv.DeniedTrustedCommand:
			r.record(fmt.Sprintf("denied %q: sources that need it were not downloaded", note))
		case venv.StubbedCommand:
			r.record(fmt.Sprintf("stubbed %q: run_cmd returned empty output", note))
		case venv.RefusedCommand:
			r.refuse(dn.Program)

			if mode == venv.RefuseWithError {
				r.record(fmt.Sprintf(
					"refused %q: apply and destroy fail a unit whose configuration names a program no --allow-cmd pattern allows",
					note,
				))

				return
			}

			r.record(fmt.Sprintf(
				"stubbed %q: --allow=exec runs Terragrunt's own commands, not programs a configuration names",
				note,
			))
		case venv.DeniedSops:
			r.record(fmt.Sprintf("refused decrypting %q: secrets stay encrypted without --allow=sops",
				truncateRunes(dn.Path, execCmdlineMaxLen)))
		}
	}
}

// callVenv returns the venv one tool call runs against. Every refusal it
// arranges is noted on d.rec, whose notes become that call's degraded field.
//
// The two denials are independent. --allow=exec governs the processes
// Terragrunt spawns, --allow=http the requests it makes itself. Neither
// constrains a spawned process, which reaches the network on its own account.
//
// A program a person approved for this call runs with any arguments, as one
// an --allow-cmd pattern matches does.
func (d *serverDeps) callVenv(
	rootVenv *venv.Venv,
	env map[string]string,
	w io.Writer,
) *venv.Venv {
	allowCmds := slices.Clone(d.allowCmds)
	for _, program := range d.approved {
		allowCmds = append(allowCmds, venv.AllowProgram(program))
	}

	return venv.Restrict(rootVenv.WithEnv(env).WithWriter(w), venv.Policy{
		Granted:   d.granted,
		AllowCmds: allowCmds,
		Refusal:   d.refusal,
		TFPath:    d.operatorTFPath(),
		Observe:   d.rec.observer(d.refusal),
	})
}
