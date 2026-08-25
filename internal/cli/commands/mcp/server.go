package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverDeps is the state a tool call runs against. The venv is deliberately
// not a field. A handler that reached for one here could spawn through the
// root venv and miss the per-call deny-all wrapper, so each handler is instead
// handed the venv it may use.
type serverDeps struct {
	baseOpts      *options.TerragruntOptions
	rec           *execRecorder
	runSem        chan struct{}
	launchDir     string
	allowCommands []string
	approved      []string
	allowExec     bool
	allowHTTP     bool
	allowSops     bool
	allowApply    bool
}

// forCall returns a copy of d scoped to one tool call, with its own recorder
// for what that call refuses and the config-named commands a person approved
// for it. The run semaphore and the rest stay shared, so calls still take
// turns.
func (d *serverDeps) forCall(approved []string) *serverDeps {
	c := *d
	c.rec = &execRecorder{}
	c.approved = approved

	return &c
}

// operatorTFPath returns the binary the operator named with --tf-path, or the
// empty string when they left it alone.
func (d *serverDeps) operatorTFPath() string {
	if !d.baseOpts.TFPathExplicitlySet {
		return ""
	}

	return d.baseOpts.TFPath
}

// reachesNetwork reports whether a tool call can leave the machine, either
// because Terragrunt makes the request or because a subprocess it spawns does.
// Even the tools that only read can, because parsing evaluates locals: a
// run_cmd() spawns, and a dependency fetch inits, downloads, and reads a state
// backend.
func (d *serverDeps) reachesNetwork() bool {
	return d.allowExec || d.allowHTTP
}

// acquireRunSlot blocks until this call may drive a real run. The SDK
// dispatches tool calls concurrently, and real runs race each other on the
// shared per-unit .terragrunt-cache download dirs and provider caches, so
// they take turns. Callers must defer the returned release func.
func (d *serverDeps) acquireRunSlot(ctx context.Context) (func(), error) {
	select {
	case d.runSem <- struct{}{}:
		return func() { <-d.runSem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run claims the console for the Model Context Protocol and serves on it
// until the client disconnects or ctx is canceled.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, opts *Options) error {
	// The protocol owns the console streams, so everything else moves off them
	// first. Machinery and subprocess output join the logs on the error
	// stream, and a spawned command reads an empty stdin rather than
	// inheriting the client's requests.
	errWriter := v.Writers.ErrWriter

	served := v.WithStdin(strings.NewReader("")).WithWriter(errWriter).WithErrWriter(errWriter)

	return Serve(ctx, l, served, opts, v.Stdin, v.Writers.Writer)
}

// Serve registers the tool set against the Terragrunt tree rooted at the
// working dir, then speaks the Model Context Protocol until the client
// disconnects or ctx is canceled. A caller other than [Run] must already have
// pointed the venv's writers and stdin away from the two streams it passes
// here, or the server will write its logs into its own protocol stream.
func Serve(
	ctx context.Context,
	l log.Logger,
	rootVenv *venv.Venv,
	opts *Options,
	in io.Reader,
	out io.Writer,
) error {
	launchDir, err := vfs.EvalSymlinks(rootVenv.FS, opts.WorkingDir)
	if err != nil {
		return fmt.Errorf("resolving launch dir %s: %w", opts.WorkingDir, err)
	}

	granted, err := ParseCapabilities(opts.Allow)
	if err != nil {
		return err
	}

	deps := &serverDeps{
		launchDir:     launchDir,
		baseOpts:      opts.TerragruntOptions,
		allowExec:     granted.Has(CapabilityExec),
		allowHTTP:     granted.Has(CapabilityHTTP),
		allowSops:     granted.Has(CapabilitySops),
		allowApply:    opts.AllowApply,
		allowCommands: opts.AllowCommands,
		runSem:        make(chan struct{}, 1),
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "terragrunt",
		Version: version.GetVersion(),
	}, &mcp.ServerOptions{
		Instructions: serverInstructions(deps),
	})

	registerDiscover(srv, l, deps, rootVenv)
	registerRenderConfig(srv, l, deps, rootVenv)
	registerValidate(srv, l, deps, rootVenv)
	registerRunOrder(srv, l, deps, rootVenv)
	registerGetOutputs(srv, l, deps, rootVenv)
	registerPlan(srv, l, deps, rootVenv)

	if opts.AllowApply {
		registerMutatingTools(srv, l, deps, rootVenv)
	}

	return serve(ctx, l, srv, in, out)
}

// serve blocks until the client closes stdin or ctx cancels, and treats both
// as a clean exit. It wraps the streams rather than handing them over, because
// the transport closes what it is given, and the console is not this command's
// to close.
func serve(
	ctx context.Context,
	l log.Logger,
	srv *mcp.Server,
	stdin io.Reader,
	stdout io.Writer,
) error {
	l.Infof("Terragrunt MCP server listening on stdio")

	err := srv.Run(ctx, &mcp.IOTransport{
		Reader: io.NopCloser(stdin),
		Writer: nopWriteCloser{stdout},
	})

	// A client that closes stdin is disconnecting, not failing, and the SDK
	// surfaces that as io.EOF; anything else is a real transport failure.
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		return err
	}

	return nil
}

// nopWriteCloser is the write-side counterpart to [io.NopCloser], which the
// standard library only provides for readers.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// serverInstructions returns the preamble an MCP client reads when it
// connects. It describes each capability on its own, because each is granted
// on its own.
func serverInstructions(d *serverDeps) string {
	b := strings.Builder{}

	b.WriteString(
		"All tools operate on the Terragrunt tree rooted at the server launch directory. ",
	)

	for _, c := range []struct {
		on, off string
		granted bool
	}{
		{
			granted: d.allowExec,
			on: "The server was started with --allow=exec, so the tools that run real tofu " +
				"commands work. That covers tofu, terraform, and git only. A program a configuration " +
				"names, through run_cmd(), a hook, or an auth-provider command, is refused unless the " +
				"operator allowed it. The read-only tools offer such a program for approval rather than " +
				"answering without it, so a call that comes back asking is not a failure: pass the request " +
				"to the person operating this client. A command that does run reaches the network on its " +
				"own account, whatever the HTTP policy below says. ",
			off: "Subprocess execution is disabled. The server cannot spawn tofu, git, or " +
				"run_cmd processes at all, and the plan tool needs the server restarted with " +
				"--allow=exec. ",
		},
		{
			granted: d.allowHTTP,
			on: "The server was started with --allow=http, so Terragrunt may reach state " +
				"backends, registries, and module sources itself. ",
			off: "Outbound HTTP is disabled. Every request Terragrunt itself makes fails, so " +
				"state backends, registries, and module sources are unreachable. A unit whose terraform " +
				"source is remote cannot be planned at all, and reports the failed download as its cause. " +
				"Without --allow=exec, get_outputs reads remote state itself, which needs --allow=http and " +
				"works for the backends the direct state read supports; any other unit reports that it " +
				"fetched nothing. ",
		},
		{
			granted: d.allowSops,
			on: "The server was started with --allow=sops, so a configuration that calls " +
				"sops_decrypt_file gets its cleartext, and that cleartext reaches you. ",
			off: "SOPS decryption is disabled. A configuration calling sops_decrypt_file fails " +
				"rather than handing you the cleartext, so a unit that reads secrets that way cannot be " +
				"rendered until the server is restarted with --allow=sops. ",
		},
	} {
		if c.granted {
			b.WriteString(c.on)

			continue
		}

		b.WriteString(c.off)
	}

	b.WriteString("A result a restriction affected carries a 'degraded' list naming what was " +
		"substituted, such as mocked dependency outputs or stubbed run_cmd calls. ")

	if d.allowApply {
		b.WriteString(
			"The server was started with --dangerously-allow-apply, so the apply and destroy " +
				"tools are served. They change and tear down real infrastructure, and neither acts on your " +
				"say-so alone. Each first returns an input request naming the units it would touch, and runs " +
				"only once the person operating this client accepts it. Forward that request to them rather " +
				"than answering it yourself, and treat a decline as final.",
		)

		return b.String()
	}

	b.WriteString(
		"This server has no apply or destroy tool, so nothing here applies or destroys. " +
			"A program the configuration names can still do whatever it likes once it is allowed or " +
			"approved.",
	)

	return b.String()
}

// toolHandler adapts the run function of a tool that answers in one round
// trip into an SDK handler, with panic recovery.
func toolHandler[In, Out any](
	l log.Logger,
	name string,
	run func(ctx context.Context, input In) (Out, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, out Out, err error) {
		defer recoverToolPanic(l, name, &out, &err)

		out, err = run(ctx, input)

		return nil, out, err
	}
}

// multiRoundTripToolHandler is [toolHandler] for a tool that may answer with
// an input request instead of a result.
func multiRoundTripToolHandler[In, Out any](
	l log.Logger,
	name string,
	run func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, out Out, err error) {
		defer recoverToolPanic(l, name, &out, &err)

		return run(ctx, req, input)
	}
}

// recoverToolPanic turns a panic in a tool handler into an error on that one
// call. The SDK dispatches every tool call on its own goroutine and never
// recovers, so a panic anywhere in the parse/cty surface would otherwise kill
// the whole stdio session instead of failing the call that caused it.
func recoverToolPanic[Out any](l log.Logger, name string, out *Out, err *error) {
	r := recover()
	if r == nil {
		return
	}

	l.Errorf("%s tool call panicked: %v\n%s", name, r, debug.Stack())

	var zero Out

	*out = zero
	*err = fmt.Errorf("internal error: the %s tool call panicked: %v", name, r)
}

// truncateRunes caps s at maxRunes runes, marking the cut with an ellipsis,
// so noisy subprocess output cannot balloon a structured result.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	return string(runes[:maxRunes]) + "…"
}
