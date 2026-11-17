package venv

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
)

// ErrExecDenied is the error every process a [Restrict] venv refuses to start
// wraps, when [CapabilityExec] is not granted.
var ErrExecDenied = errors.New("subprocess execution denied: the exec capability is not granted")

// ErrSopsDenied is the error every decrypt a [Restrict] venv refuses wraps,
// when [CapabilitySops] is not granted.
var ErrSopsDenied = errors.New("SOPS decryption denied: the sops capability is not granted")

// ErrCmdNotAllowed is the error a program a configuration names fails with
// under [RefuseWithError], when no [AllowCmd] allows it.
var ErrCmdNotAllowed = errors.New("the configuration names a program no allow-cmd pattern allows")

// ErrAllowCmdRequiresExec is what [Policy.Validate] returns for a policy with
// allow-cmd patterns and no [CapabilityExec], whose patterns would allow
// nothing.
var ErrAllowCmdRequiresExec = errors.New(
	"allow-cmd patterns require the exec capability: without it nothing runs, allowed or not",
)

// ErrListenDenied is what a [Restrict] venv's listener returns when
// [CapabilityExec] is not granted.
var ErrListenDenied = errors.New("listening denied: the exec capability is not granted")

// Refusal selects what a program a configuration names gets back when exec is
// granted but no [AllowCmd] allows the program.
type Refusal int

const (
	// RefuseWithStub answers with empty output and a zero exit, so code that
	// only reads a configuration can still evaluate it.
	RefuseWithStub Refusal = iota
	// RefuseWithError fails the command with [ErrCmdNotAllowed], so a run that
	// changes infrastructure stops rather than proceeding on a stubbed answer.
	RefuseWithError
)

// Policy is what a [Restrict] venv may do.
type Policy struct {
	// Granted holds the capabilities the venv keeps. Every other capability
	// [RestrictCapabilities] lists is refused.
	Granted Capabilities
	// Observe receives each refusal as it happens, and may be called from
	// several goroutines at once. A nil Observe discards them.
	Observe func(Denial)
	// TFPath is a tofu or terraform binary the operator named explicitly.
	// Without exec, an invocation of it fails like one of tofu, whatever its
	// file is called. Leave it empty when the operator named none.
	TFPath string
	// AllowCmds lets the programs a configuration names run, through
	// run_cmd(), a hook, or an auth-provider command. They take effect only
	// when [CapabilityExec] is granted.
	AllowCmds []AllowCmd
	// Refusal is what a program a configuration names gets back when exec is
	// granted and no entry in AllowCmds matches it.
	Refusal Refusal
}

// Validate reports a policy that grants less than it reads as granting.
//
// Returns [ErrAllowCmdRequiresExec] when p has allow-cmd patterns and no
// [CapabilityExec].
func (p Policy) Validate() error {
	if len(p.AllowCmds) > 0 && !p.Granted.Has(CapabilityExec) {
		return ErrAllowCmdRequiresExec
	}

	return nil
}

// DenialKind says how a [Restrict] venv answered something it refused.
type DenialKind int

const (
	// DeniedIaC is a tofu or terraform invocation, or one of [Policy.TFPath],
	// that failed with [ErrExecDenied]. A dependency fetch falls back to
	// mock_outputs where they are configured.
	DeniedIaC DenialKind = iota
	// DeniedTrustedCommand is a command Terragrunt starts on its own account,
	// such as the git that downloads a source, that failed with
	// [ErrExecDenied].
	DeniedTrustedCommand
	// StubbedCommand is a program a configuration named, answered with
	// empty output and a zero exit because exec is not granted.
	StubbedCommand
	// RefusedCommand is a program a configuration named that no [AllowCmd]
	// allows while exec is granted, answered according to [Policy.Refusal].
	RefusedCommand
	// DeniedSops is a decrypt that failed with [ErrSopsDenied].
	DeniedSops
)

// Denial is one thing a [Restrict] venv refused.
type Denial struct {
	// Program is the program an invocation named, reduced the way
	// [AllowCmd.Matches] compares it, so it can be passed to [AllowProgram].
	// It is empty for [DeniedSops].
	Program string
	// Cmdline is the invocation with credentials removed from URL arguments
	// and terminal control characters removed. It is empty for [DeniedSops].
	Cmdline string
	// Path is the file a refused decrypt named, with terminal control
	// characters removed. It is empty for every kind but [DeniedSops].
	Path string
	// Kind says how the venv answered.
	Kind DenialKind
}

// Restrict returns a copy of v that refuses every capability p does not grant,
// for evaluating a configuration the caller does not trust.
//
// Without [CapabilityExec], no process starts. Each invocation gets the answer
// that degrades its caller least, as the [DenialKind] values describe, and
// listening fails with [ErrListenDenied]. With it, the commands Terragrunt
// runs itself start, and a program a configuration names starts only when an
// entry in [Policy.AllowCmds] matches it.
//
// Without [CapabilityHTTP], the requests Terragrunt makes itself fail. Without
// [CapabilitySops], decrypts fail with [ErrSopsDenied]. Without
// [CapabilityEnv], the copy starts from an empty environment. With it, the
// copy shares v's environment map, so a caller that restricts one Venv for
// several runs clones it first with [Venv.WithEnvCloned]. The copy always
// reads an empty stdin and never opens a browser.
//
// Restrict does not confine the filesystem: an HCL function like file() reads
// anything v.FS can. Nor does it constrain a process it lets start, or code
// that reads the process environment directly, such as go-getter and the
// cloud SDKs.
func Restrict(v *Venv, p Policy) *Venv {
	observe := p.Observe
	if observe == nil {
		observe = func(Denial) {}
	}

	r := v.WithStdin(strings.NewReader("")).WithBrowser(vbrowser.NewNoBrowserOpener())

	if !p.Granted.Has(CapabilityEnv) {
		r = r.WithEnv(map[string]string{})
	}

	if !p.Granted.Has(CapabilityHTTP) {
		r = r.WithHTTP(vhttp.NewNoNetworkClient())
	}

	if !p.Granted.Has(CapabilitySops) {
		r = r.WithSops(vsops.NewMemDecrypter(denySopsHandler(observe)))
	}

	if !p.Granted.Has(CapabilityExec) {
		r = r.WithHandler(denyAllHandler(observe, p.TFPath))
		r.Listen = func(context.Context, string, string) (net.Listener, error) {
			return nil, ErrListenDenied
		}

		return r
	}

	return r.WithExec(&allowlistExec{
		Exec:      r.Exec,
		denied:    vexec.NewMemExec(configCommandHandler(observe, p.Refusal)),
		allowCmds: p.AllowCmds,
	})
}

// allowlistExec runs the commands Terragrunt runs itself, and the programs a
// configuration names that an allow-cmd pattern matches. It hands everything
// else to denied.
type allowlistExec struct {
	vexec.Exec
	denied    vexec.Exec
	allowCmds []AllowCmd
}

// Command implements [vexec.Exec].
func (e *allowlistExec) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	if vexec.IsTrustedCommand(ctx) || e.permits(name, args) {
		return e.Exec.Command(ctx, name, args...)
	}

	return e.denied.Command(ctx, name, args...)
}

// permits reports whether an invocation a configuration named may reach the
// real executor. Terragrunt's own tofu, terraform, and git arrive marked (see
// [vexec.WithTrustedCommand]) and never reach here; this decides the programs
// a configuration names, through run_cmd(), a hook, or an auth-provider
// command.
//
// A tofu or git a configuration names is not one of Terragrunt's own, so it is
// refused like any other program unless a pattern allows it.
func (e *allowlistExec) permits(name string, args []string) bool {
	return slices.ContainsFunc(e.programs(name), func(program string) bool {
		return slices.ContainsFunc(e.allowCmds, func(c AllowCmd) bool { return c.Matches(program, args) })
	})
}

// programs returns every program a pattern may name to cover an invocation. A
// bare name answers to itself. A name with a path answers to that path, and to
// the bare program only when the path is exactly where PATH resolves it,
// because matching on the base alone lets a configuration ship a file named
// git beside itself and reach the real executor with ./git.
func (e *allowlistExec) programs(name string) []string {
	program := execProgram(name)
	base := execBinaryName(name)

	if program == base {
		return []string{program}
	}

	if path, err := e.Exec.LookPath(base); err != nil || path != name {
		return []string{program}
	}

	return []string{program, base}
}

// denySopsHandler returns a [vsops.Handler] that decrypts nothing. It fails
// rather than answering with empty cleartext, because a configuration that
// reads a secret and gets an empty string computes a wrong value silently,
// where an error says what happened.
//
// Refusing also keeps the real decrypter from publishing credentials into the
// process environment, which it does for the length of a decrypt.
func denySopsHandler(observe func(Denial)) vsops.Handler {
	return func(_ map[string]string, path, _ string) ([]byte, error) {
		observe(Denial{Kind: DeniedSops, Path: tui.SanitizeLabel(path)})

		return nil, fmt.Errorf("%w: %s", ErrSopsDenied, path)
	}
}

// configCommandHandler answers a program a configuration named that no
// pattern allowed. Under [RefuseWithStub] it returns empty output and a zero
// exit, because an error there aborts the whole HCL parse, which would turn a
// refused command into an unreadable configuration.
//
// Under [RefuseWithError] it fails instead. A stub reads as success, so a
// before_hook meant to block an apply would let it through, and a run_cmd
// would feed an empty string into the inputs being applied.
func configCommandHandler(observe func(Denial), mode Refusal) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		cmdline := execCmdline(&inv)

		observe(Denial{Kind: RefusedCommand, Program: execProgram(inv.Name), Cmdline: cmdline})

		if mode == RefuseWithError {
			return vexec.Result{Err: fmt.Errorf("%w: %s", ErrCmdNotAllowed, cmdline), ExitCode: 1}
		}

		return vexec.Result{}
	}
}

// denyAllHandler returns a [vexec.Handler] that never spawns a process. Each
// shape of invocation gets the answer the matching [DenialKind] describes.
//
// A run_cmd invocation returns empty output and a zero exit, because a
// run_cmd error aborts the whole HCL parse.
//
// Nothing here allocates a PTY. Only `tofu console` asks for one, only when
// stdin is a TTY, and a restricted venv's stdin never is.
func denyAllHandler(observe func(Denial), tfPath string) vexec.Handler {
	return func(ctx context.Context, inv vexec.Invocation) vexec.Result {
		denial := Denial{Program: execProgram(inv.Name), Cmdline: execCmdline(&inv)}

		switch binary := execBinaryName(inv.Name); {
		case inv.Name == tfPath, binary == "tofu", binary == "terraform", binary == "opentofu":
			denial.Kind = DeniedIaC
			observe(denial)

			return vexec.Result{Err: fmt.Errorf("%w: %s", ErrExecDenied, denial.Cmdline)}
		case vexec.IsTrustedCommand(ctx):
			denial.Kind = DeniedTrustedCommand
			observe(denial)

			return vexec.Result{Err: fmt.Errorf("%w: %s", ErrExecDenied, denial.Cmdline)}
		default:
			denial.Kind = StubbedCommand
			observe(denial)

			return vexec.Result{}
		}
	}
}

// execBinaryName reduces an invocation name to the bare program, so a Windows
// .exe matches the same pattern as the name without it.
func execBinaryName(name string) string {
	return strings.TrimSuffix(filepath.Base(name), ".exe")
}

// execProgram reduces an invocation name to the program a pattern names. A
// bare name loses any .exe suffix. A name with a path becomes its lexically
// cleaned, slash-separated path, without any .exe suffix. A relative path
// that cleans down to a bare name keeps a leading ./, so the git a tree ships
// beside itself never reads as the git PATH finds.
func execProgram(name string) string {
	if filepath.Base(name) == name {
		return strings.TrimSuffix(name, ".exe")
	}

	path := filepath.ToSlash(filepath.Clean(name))
	if !strings.Contains(path, "/") {
		path = "./" + path
	}

	return strings.TrimSuffix(path, ".exe")
}

// execCmdline renders an invocation for a [Denial] or an error, with the
// credentials taken out of every argument that is a URL. The dir is left out:
// the same command evaluated from a root config included by N units differs
// only by dir, and an observer deduplicating denials would see N of them.
func execCmdline(inv *vexec.Invocation) string {
	words := make([]string, 0, len(inv.Args)+1)
	words = append(words, inv.Name)

	for _, arg := range inv.Args {
		words = append(words, redactArg(arg))
	}

	return tui.SanitizeLabel(strings.TrimSpace(strings.Join(words, " ")))
}

// redactArg returns arg with any credential it carries removed. An arg with
// neither a scheme nor a query is returned as written: it carries none, and
// rendering a local path as a URL would escape it.
func redactArg(arg string) string {
	if !strings.Contains(arg, "://") && !strings.Contains(arg, "?") {
		return arg
	}

	return redact.NewURL(arg).String()
}
