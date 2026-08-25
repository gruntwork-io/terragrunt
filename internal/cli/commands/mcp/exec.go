package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
)

// ErrExecDenied is the error every subprocess denial wraps. Tool handlers
// match it with errors.Is to convert hard failures into actionable guidance.
var ErrExecDenied = errors.New(
	"subprocess execution disabled; restart the MCP server with --allow=exec",
)

// ErrSopsDenied is the error every refused decrypt wraps, so a tool handler
// can turn it into guidance rather than letting it read as a broken file.
var ErrSopsDenied = errors.New("SOPS decryption disabled; restart the MCP server with --allow=sops")

// execAllowedBinaries are the programs Terragrunt runs to do its own work.
// --allow=exec grants these and nothing else, so a program a configuration
// names, through run_cmd(), a hook, or an auth-provider command, never runs.
// Those are the reachable path from a repository the agent was pointed at to
// code running on this machine, and granting the ability to plan should not
// grant that.
var execAllowedBinaries = []string{"tofu", "terraform", "git"}

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

// callVenv returns the venv one tool call runs against. Every refusal it
// arranges is noted on d.rec, whose notes become that call's degraded field.
//
// The two denials are independent. --allow=exec governs the processes
// Terragrunt spawns, --allow=http the requests it makes itself. Neither
// constrains a spawned process, which reaches the network on its own account.
func (d *serverDeps) callVenv(
	rootVenv *venv.Venv,
	env map[string]string,
	w io.Writer,
) *venv.Venv {
	v := rootVenv.WithEnv(env).WithWriter(w)

	if !d.allowHTTP {
		v = v.WithHTTP(vhttp.NewNoNetworkClient())
	}

	if !d.allowSops {
		v = v.WithSops(vsops.NewMemDecrypter(denySopsHandler(d.rec)))
	}

	if !d.allowExec {
		return v.WithHandler(denyAllHandler(d.rec))
	}

	return v.WithExec(allowlistExec{
		Exec:    v.Exec,
		denied:  vexec.NewMemExec(configCommandHandler(d.rec)),
		allowed: d.spawnable(),
		tfPath:  d.operatorTFPath(),
	})
}

func (d *serverDeps) spawnable() []string {
	names := make([]string, 0, len(execAllowedBinaries)+len(d.allowCommands)+len(d.approved))
	names = append(names, execAllowedBinaries...)
	names = append(names, d.allowCommands...)
	names = append(names, d.approved...)

	return names
}

type allowlistExec struct {
	vexec.Exec
	denied  vexec.Exec
	tfPath  string
	allowed []string
}

func (e allowlistExec) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	if execIsAllowed(e.allowed, e.tfPath, name) {
		return e.Exec.Command(ctx, name, args...)
	}

	return e.denied.Command(ctx, name, args...)
}

// execIsAllowed reports whether name is one of the allowed programs, resolved
// through PATH rather than through the tree being served.
//
// A name carrying any path component is refused outright, even one whose base
// is allowed, because matching on the base alone lets a configuration ship a
// file named git beside itself and reach the real executor with ./git.
//
// The one pathed name that runs is the binary the operator named with
// --tf-path, which is compared whole. A path a configuration chose never
// reaches here as tfPath; see [applyOperatorTFPath].
func execIsAllowed(allowed []string, tfPath, name string) bool {
	if tfPath != "" && name == tfPath {
		return true
	}

	if strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, '/') {
		return false
	}

	return slices.Contains(allowed, execBinaryName(name))
}

// denySopsHandler returns a [vsops.Handler] that decrypts nothing. It fails
// rather than answering with empty cleartext, because a configuration that
// reads a secret and gets an empty string computes a wrong value silently,
// where an error says what happened.
//
// Refusing also keeps the real decrypter from publishing credentials into the
// process environment, which it does for the length of a decrypt.
func denySopsHandler(rec *execRecorder) vsops.Handler {
	return func(_ map[string]string, path, _ string) ([]byte, error) {
		rec.record(fmt.Sprintf("refused decrypting %q: secrets stay encrypted without --allow=sops",
			truncateRunes(tui.SanitizeLabel(path), execCmdlineMaxLen)))

		return nil, fmt.Errorf("%w: %s", ErrSopsDenied, path)
	}
}

// configCommandHandler answers a program a configuration named. It returns
// empty output and a zero exit for the same reason the deny-all handler does
// for run_cmd: an error there aborts the whole HCL parse, which would turn a
// refused command into an unreadable configuration.
func configCommandHandler(rec *execRecorder) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		rec.refuse(execBinaryName(inv.Name))
		rec.record(fmt.Sprintf(
			"stubbed %q: --allow=exec runs Terragrunt's own commands, not programs a configuration names",
			truncateRunes(execCmdline(&inv), execCmdlineMaxLen),
		))

		return vexec.Result{}
	}
}

// execBinaryName reduces an invocation name to the bare program, so a Windows
// .exe matches the same allowlist entry as the name without it.
func execBinaryName(name string) string {
	return strings.TrimSuffix(filepath.Base(name), ".exe")
}

// execCmdline renders an invocation for a note. The dir is deliberately left
// out: the same command evaluated from a root config included by N units
// differs only by dir, and per-dir notes would defeat the recorder's dedup.
func execCmdline(inv *vexec.Invocation) string {
	return tui.SanitizeLabel(strings.TrimSpace(inv.Name + " " + strings.Join(inv.Args, " ")))
}

// denyAllHandler returns a [vexec.Handler] that never spawns a process. Each
// shape of invocation gets the answer that degrades its caller least. A tofu
// invocation fails with [ErrExecDenied], so a dependency fetch falls back to
// mock_outputs. A run_cmd invocation returns empty output and a zero exit,
// because a run_cmd error aborts the whole HCL parse. A git invocation answers
// with the dir it was called from, so the repo-root HCL functions keep working.
//
// Nothing here allocates a PTY. Only `tofu console` asks for one, only when
// stdin is a TTY, and no tool on this server runs console.
func denyAllHandler(rec *execRecorder) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		cmdline := execCmdline(&inv)
		note := truncateRunes(cmdline, execCmdlineMaxLen)

		switch execBinaryName(inv.Name) {
		case "tofu", "terraform", "opentofu":
			rec.record(
				fmt.Sprintf(
					"denied %q: dependency outputs degraded to mock_outputs where configured",
					note,
				),
			)

			return vexec.Result{Err: fmt.Errorf("%w: %s", ErrExecDenied, cmdline)}
		case "git":
			rec.record(
				fmt.Sprintf("stubbed %q: repo-root paths approximated by the invocation dir", note),
			)

			return vexec.Result{Stdout: []byte(inv.Dir + "\n")}
		default:
			rec.record(fmt.Sprintf("stubbed %q: run_cmd returned empty output", note))

			return vexec.Result{}
		}
	}
}
