package cas_test

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// stubHash is the commit hash lsRemoteStub reports.
const stubHash = "0123456789abcdef0123456789abcdef01234567"

// lsRemoteStub answers `git ls-remote` from memory, counting calls and,
// when gate is set, holding each call until gate closes or the command's
// context ends (or gate alone when ignoreCancel is set, standing in for a
// git that has stopped listening). Every other git subcommand is answered
// with empty output.
//
// The answer names the ref it resolved the way a repository with no
// shadowing names would: a version-looking request is a tag, HEAD is
// HEAD, and anything else is a branch. resolvedRef, when set, overrides
// that for every answer. panicValue, when set, is raised once the gate
// has been passed, standing in for a bug inside the probe.
type lsRemoteStub struct {
	gate         chan struct{}
	panicValue   any
	hash         string
	resolvedRef  string
	calls        atomic.Int32
	ignoreCancel bool
}

// venv returns an in-memory bundle whose git is the stub.
func (s *lsRemoteStub) venv() *venv.Venv {
	return venvtest.New().WithExec(newStubGitExec(
		func(ctx context.Context, inv vexec.Invocation) vexec.Result {
			if len(inv.Args) == 0 || inv.Args[0] != "ls-remote" {
				return vexec.Result{}
			}

			s.calls.Add(1)

			if s.gate != nil && s.ignoreCancel {
				<-s.gate
			}

			if s.gate != nil && !s.ignoreCancel {
				select {
				case <-s.gate:
				case <-ctx.Done():
					return vexec.Result{Err: ctx.Err()}
				}
			}

			if s.panicValue != nil {
				panic(s.panicValue)
			}

			return vexec.Result{Stdout: []byte(s.hash + "\t" + s.resolved(inv.Args[len(inv.Args)-1]) + "\n")}
		},
	))
}

// resolved returns the ref name the stub reports for the requested ref.
func (s *lsRemoteStub) resolved(ref string) string {
	if s.resolvedRef != "" {
		return s.resolvedRef
	}

	if ref == "HEAD" {
		return ref
	}

	if cas.IsSemverTag("refs/tags/" + ref) {
		return "refs/tags/" + ref
	}

	return "refs/heads/" + ref
}

// gitExecRecorder wraps a real exec, counting git subcommands by name.
// When gate is set, every ls-remote waits on it before running, and
// firstLsRemote closes the first time one is prepared, so a test can
// hold the network step open while it lines up more callers.
type gitExecRecorder struct {
	inner         vexec.Exec
	gate          chan struct{}
	firstLsRemote chan struct{}
	counts        map[string]int
	once          sync.Once
	mu            sync.Mutex
}

// newGitExecRecorder wraps inner. A nil gate leaves ls-remote ungated.
func newGitExecRecorder(inner vexec.Exec, gate chan struct{}) *gitExecRecorder {
	return &gitExecRecorder{
		inner:         inner,
		gate:          gate,
		firstLsRemote: make(chan struct{}),
		counts:        make(map[string]int),
	}
}

func (r *gitExecRecorder) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}

	r.mu.Lock()
	r.counts[sub]++
	r.mu.Unlock()

	cmd := r.inner.Command(ctx, name, args...)

	if sub != "ls-remote" {
		return cmd
	}

	r.once.Do(func() { close(r.firstLsRemote) })

	if r.gate == nil {
		return cmd
	}

	return &gatedCmd{Cmd: cmd, ctx: ctx, gate: r.gate}
}

func (r *gitExecRecorder) LookPath(file string) (string, error) {
	return r.inner.LookPath(file)
}

// count returns how many times the git subcommand sub was prepared.
func (r *gitExecRecorder) count(sub string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.counts[sub]
}

// gatedCmd delays Run until gate closes, giving up when the command's
// context ends first.
type gatedCmd struct {
	vexec.Cmd
	ctx  context.Context
	gate chan struct{}
}

func (c *gatedCmd) Run() error {
	select {
	case <-c.gate:
		return c.Cmd.Run()
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}
