package mcp

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// runAllCauseMaxLen caps the failure-cause excerpt embedded per unit so a noisy
// tofu error cannot balloon the structured result into a log dump.
const runAllCauseMaxLen = 300

type runAllInput struct {
	WorkingDir string   `json:"working_dir,omitempty" jsonschema:"Directory whose units to run. Defaults to the server root."`
	Filter     []string `json:"filter,omitempty"      jsonschema:"Terragrunt filter queries to narrow which units are run."`
}

type runAllUnitResult struct {
	Reason     *string `json:"reason,omitempty"`
	Cause      *string `json:"cause,omitempty"`
	Path       string  `json:"path"`
	Result     string  `json:"result"`
	DurationMs int64   `json:"duration_ms"`
}

type runAllOutput struct {
	Units     []runAllUnitResult `json:"units"`
	Degraded  []string           `json:"degraded,omitempty"`
	Succeeded int                `json:"succeeded"`
	Failed    int                `json:"failed"`
	Excluded  int                `json:"excluded"`
	EarlyExit int                `json:"early_exit"`
}

func registerPlan(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "plan",
		Description: "Run 'terragrunt run --all plan' across the units under a directory and return a " +
			"per-unit result summary (succeeded/failed/excluded, with failure causes). Use to verify a " +
			"proposed change before a human applies it. Requires the server to be started with " +
			"--allow=exec, and runs are serialized, so concurrent plan calls queue. This can download module " +
			"sources, run init, and reach backends/registries.",
		Annotations: &mcp.ToolAnnotations{
			// Not read-only: plan writes caches, lockfile scratch, and plan
			// files on disk, and clients should gate it accordingly.
			IdempotentHint: true,
			// Unconditional rather than following the HTTP policy: plan
			// requires --allow=exec, and a spawned command reaches the
			// network whether or not Terragrunt itself may.
			OpenWorldHint: new(true),
		},
	}, toolHandler(l, "plan", func(ctx context.Context, input runAllInput) (runAllOutput, error) {
		return runAllCommand(ctx, l, d.forCall(nil), rootVenv, input, tf.CommandNamePlan)
	}))
}

// runAllCommand drives `run --all <command>` across the units under the call's
// working dir and summarizes the per-unit results. Whether a given command is
// allowed to reach this point is the caller's decision: plan is registered
// whenever exec is, apply and destroy only under --dangerously-allow-apply.
func runAllCommand(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input runAllInput,
	command string,
) (runAllOutput, error) {
	if !d.allowExec {
		return runAllOutput{}, fmt.Errorf(
			"%s requires subprocess execution; restart the MCP server with --allow=exec",
			command,
		)
	}

	release, err := d.acquireRunSlot(ctx)
	if err != nil {
		return runAllOutput{}, err
	}

	defer release()

	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return runAllOutput{}, err
	}

	opts, env, err := buildDirOptions(l, d, rootVenv, dir, input.Filter)
	if err != nil {
		return runAllOutput{}, err
	}

	opts.TerraformCommand = command
	opts.OriginalTerraformCommand = command
	opts.TerraformCliArgs = iacargs.New(command)
	opts.RunAll = true
	// Stack generation lives in runall.Run, which this bypasses. Running an
	// already-generated tree stays a read of the agent's working directory;
	// regenerating .terragrunt-stack dirs would rewrite it underneath them.
	opts.NoStackGenerate = true

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	// runall.Run is where the CLI creates git-filter worktrees, and this
	// bypasses it, so it creates them itself.
	var runnerOpts []runner.Option

	w, cleanup, err := setupGitFilterWorktrees(ctx, l, d, cv, opts)
	if err != nil {
		return runAllOutput{}, err
	}

	if w != nil {
		defer cleanup()

		runnerOpts = append(runnerOpts, runner.WithWorktrees(w))
	}

	// Explicit WithDisableColor: the usual stdout.IsRedirected probe reads
	// the real stdout, which is meaningless under the MCP transport.
	r := report.NewReport().WithWorkingDir(dir).WithDisableColor().WithFormat(report.FormatJSON)

	rnr, err := runner.New(ctx, l, cv, opts, runnerOpts...)
	if err != nil {
		return runAllOutput{}, err
	}

	// A non-nil runErr with a populated report means unit failures, which
	// the per-unit summary already carries; only fail the tool call when
	// nothing ran at all (discovery or build failure).
	runErr := runall.RunAllOnStack(ctx, l, cv, opts, rnr, r)
	if runErr != nil && len(r.Runs) == 0 {
		return runAllOutput{}, fmt.Errorf("%s failed before any unit ran: %w", command, runErr)
	}

	out := runAllSummarize(r, dir)
	out.Degraded = d.rec.notes()

	return out, nil
}

// runAllSummarize builds the per-unit result summary from the report's runs,
// relativizing paths against workingDir and tallying the result counters.
func runAllSummarize(r *report.Report, workingDir string) runAllOutput {
	// WriteJSON sorts on write; the raw Runs slice is in completion order.
	r.SortRuns()

	out := runAllOutput{
		Units: make([]runAllUnitResult, 0, len(r.Runs)),
	}

	for _, unitRun := range r.Runs {
		res := runAllUnitResult{
			Path:   runAllRunPath(unitRun, workingDir),
			Result: string(unitRun.Result),
		}

		if !unitRun.Ended.IsZero() {
			res.DurationMs = unitRun.Ended.Sub(unitRun.Started).Milliseconds()
		}

		if unitRun.Reason != nil {
			reason := string(*unitRun.Reason)
			res.Reason = &reason
		}

		if unitRun.Cause != nil {
			cause := truncateRunes(string(*unitRun.Cause), runAllCauseMaxLen)
			res.Cause = &cause
		}

		switch unitRun.Result {
		case report.ResultSucceeded:
			out.Succeeded++
		case report.ResultFailed:
			out.Failed++
		case report.ResultExcluded:
			out.Excluded++
		case report.ResultEarlyExit:
			out.EarlyExit++
		}

		out.Units = append(out.Units, res)
	}

	return out
}

// runAllRunPath relativizes a run's path against its discovery working dir when
// set (git-filter worktrees run under a different root), falling back to the
// tool call's working dir; paths outside the base stay absolute.
func runAllRunPath(unitRun *report.Run, workingDir string) string {
	base := workingDir
	if unitRun.DiscoveryWorkingDir != "" {
		base = unitRun.DiscoveryWorkingDir
	}

	if unitRun.Path == base {
		return filepath.Base(unitRun.Path)
	}

	rel, err := filepath.Rel(base, unitRun.Path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return unitRun.Path
	}

	return rel
}

// registerMutatingTools adds the tools that change infrastructure. It is
// called only when the server was started with --dangerously-allow-apply, so
// a client that was not granted the capability is never told these tools
// exist, rather than being told about tools that would refuse. Both go
// through the approval round trip in [runMutatingCommand].
func registerMutatingTools(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	for _, tool := range []struct {
		name        string
		command     string
		description string
	}{
		{
			name:    "apply",
			command: tf.CommandNameApply,
			description: "Run 'terragrunt run --all apply' across the units under a directory and return a " +
				"per-unit result summary. THIS CHANGES REAL INFRASTRUCTURE: it creates, modifies, and " +
				"can replace resources. The call does not apply anything on its own: it names the units " +
				"it would touch and asks the person operating this client to accept, and only their " +
				"acceptance starts the apply. It does not plan them, so call the plan tool first if you " +
				"need to know what would change. A client that cannot ask them is refused. Runs are " +
				"serialized, so concurrent calls queue.",
		},
		{
			name:    "destroy",
			command: tf.CommandNameDestroy,
			description: "Run 'terragrunt run --all destroy' across the units under a directory and return a " +
				"per-unit result summary. THIS DESTROYS REAL INFRASTRUCTURE, in dependency order, and the " +
				"data in the resources it removes is gone. The call does not destroy anything on its own: " +
				"it first asks the person operating this client to approve the exact set of units, and " +
				"only their acceptance starts the destroy. A client that cannot ask them is refused. " +
				"Narrow the blast radius with the filter argument before calling.",
		},
	} {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        tool.name,
			Description: tool.description,
			Annotations: &mcp.ToolAnnotations{
				DestructiveHint: new(true),
				// Re-running is not a no-op: it re-reconciles against
				// whatever the world looks like at that moment.
				IdempotentHint: false,
				OpenWorldHint:  new(true),
			},
		}, multiRoundTripToolHandler(
			l,
			tool.name,
			func(ctx context.Context, req *mcp.CallToolRequest, input runAllInput) (*mcp.CallToolResult, runAllOutput, error) {
				return runMutatingCommand(
					ctx,
					l,
					d.forCall(nil),
					rootVenv,
					req,
					input,
					tool.command,
				)
			},
		))
	}
}
