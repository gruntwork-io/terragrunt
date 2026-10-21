package mcp

import (
	"context"
	"errors"
	"fmt"
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
	WorkingDir  string   `json:"working_dir,omitempty" jsonschema:"Directory whose units run. Defaults to server root."`
	Filter      []string `json:"filter,omitempty"      jsonschema:"Filter queries that narrow which units run."`
	Parallelism int      `json:"parallelism,omitempty" jsonschema:"Maximum units to run at once. Omit for no limit."`
}

// planInput is a [runAllInput] for the plan tool, which can also keep its
// plan files.
type planInput struct {
	runAllInput

	IncludeSensitive bool `json:"include_sensitive,omitempty" jsonschema:"Return plan_json, holding cleartext secrets."`
}

// ErrToolParallelismInvalid is returned when a tool call asks for a
// parallelism below one.
var ErrToolParallelismInvalid = errors.New("parallelism must be positive")

// validate refuses arguments a run cannot honor, before a person is asked to
// approve the run or any unit starts.
func (in runAllInput) validate() error {
	if in.Parallelism < 0 {
		return fmt.Errorf("%w, got %d", ErrToolParallelismInvalid, in.Parallelism)
	}

	return nil
}

type runAllUnitResult struct {
	Reason          *string         `json:"reason,omitempty"`
	Cause           *string         `json:"cause,omitempty"`
	Changes         *tf.PlanChanges `json:"changes,omitempty"`
	Path            string          `json:"path"`
	Result          string          `json:"result"`
	Stdout          string          `json:"stdout,omitempty"`
	PlanJSON        string          `json:"plan_json,omitempty"`
	absPath         string
	DurationMs      int64 `json:"duration_ms"`
	StdoutTruncated bool  `json:"stdout_truncated,omitempty"`
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
			"proposed change before a human applies it. Each unit that planned has changes, the counts of " +
			"resources to add/change/destroy/import/forget and of outputs that change. Each unit that " +
			"printed anything has stdout, the path of a file with its stdout, color stripped: the plan as " +
			"OpenTofu/Terraform prints it, sensitive values redacted. Read it to see which resources " +
			"change and how. A stdout file keeps the last 8 MiB (stdout_truncated says when the start " +
			"was cut). With include_sensitive=true, each unit that planned also has plan_json, the path " +
			"of the plan's JSON representation (what show -json prints), which lists every resource " +
			"change with its before and after values in cleartext, sensitive ones included; without it, " +
			"the plan files are deleted once their changes are counted. The files stay until the server " +
			"stops. Requires " +
			"the server to be started with --allow=exec, and runs are serialized, so concurrent plan " +
			"calls queue. This can download module sources, run init, and reach backends/registries.",
		Annotations: &mcp.ToolAnnotations{
			// Not read-only: plan writes caches, lockfile scratch, and plan
			// files on disk, and clients should gate it accordingly.
			IdempotentHint: true,
			// Unconditional rather than following the HTTP policy: plan
			// requires --allow=exec, and a spawned command reaches the
			// network whether or not Terragrunt itself may.
			OpenWorldHint: new(true),
		},
	}, toolHandler(l, "plan", func(ctx context.Context, input planInput) (runAllOutput, error) {
		return runAllCommand(
			ctx,
			l,
			d.forCall(nil),
			rootVenv,
			input.runAllInput,
			tf.CommandNamePlan,
			outputsSensitivityFor(input.IncludeSensitive),
		)
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
	sensitivity outputsSensitivity,
) (runAllOutput, error) {
	if !d.allowExec {
		return runAllOutput{}, fmt.Errorf(
			"%s requires subprocess execution; restart the MCP server with --allow=exec",
			command,
		)
	}

	if err := input.validate(); err != nil {
		return runAllOutput{}, err
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

	if input.Parallelism > 0 {
		opts.Parallelism = input.Parallelism
	}

	opts.TerraformCommand = command
	opts.OriginalTerraformCommand = command
	opts.TerraformCliArgs = iacargs.New(command)
	opts.RunAll = true
	// Stack generation lives in runall.Run, which this bypasses. Running an
	// already-generated tree stays a read of the agent's working directory;
	// regenerating .terragrunt-stack dirs would rewrite it underneath them.
	opts.NoStackGenerate = true

	capture, err := newStdoutCapture(rootVenv.FS, d.stdoutDir, command)
	if err != nil {
		return runAllOutput{}, err
	}

	ran := false

	defer func() {
		capture.close(l)

		if ran {
			return
		}

		if err := capture.discard(); err != nil {
			l.Warnf("Removing the stdout directory of a %s that ran no unit: %v", command, err)
		}
	}()

	opts.ForwardTFStdout = true
	opts.TerraformCliArgs.AppendFlag(tf.FlagNameNoColor)

	if command == tf.CommandNamePlan {
		opts.OutputFolder = capture.planDir()
		opts.JSONOutputFolder = capture.planDir()
	}

	cv := d.callVenv(rootVenv, env, capture)

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

	runErr := runall.RunAllOnStack(ctx, l, cv, opts, rnr, r)

	capture.close(l)

	// A non-nil runErr with a populated report means unit failures, which
	// the per-unit summary already carries; only fail the tool call when
	// nothing ran at all (discovery or build failure).
	if runErr != nil && len(r.Runs) == 0 {
		return runAllOutput{}, fmt.Errorf("%s failed before any unit ran: %w", command, runErr)
	}

	ran = true

	out := runAllSummarize(r, dir)
	out.Degraded = d.rec.notes()
	out.Degraded = append(out.Degraded, capture.annotate(out.Units)...)

	if command != tf.CommandNamePlan {
		return out, nil
	}

	jsonFiles := make(map[string]string, len(rnr.Stack.Units))
	for _, u := range rnr.Stack.Units {
		jsonFiles[filepath.Clean(u.Path())] = u.OutputJSONFile(
			opts.RootWorkingDir,
			opts.JSONOutputFolder,
		)
	}

	out.Degraded = append(
		out.Degraded,
		annotatePlanChanges(rootVenv.FS, out.Units, jsonFiles)...)

	if sensitivity == outputsRevealSensitive {
		return out, nil
	}

	for i := range out.Units {
		out.Units[i].PlanJSON = ""
	}

	if err := rootVenv.FS.RemoveAll(capture.planDir()); err != nil {
		out.Degraded = append(out.Degraded, fmt.Sprintf("the plan files could not be removed: %v", err))
	}

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
			Path:    runAllRunPath(unitRun, workingDir),
			Result:  string(unitRun.Result),
			absPath: filepath.Clean(unitRun.Path),
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
				"need to know what would change. A client that cannot ask them is refused. Each unit that " +
				"printed anything has stdout, the path of a file with its stdout, color stripped, with " +
				"the apply summary and the unit's outputs, sensitive values redacted. A stdout file keeps " +
				"the last 8 MiB (stdout_truncated says when the start was cut) and stays until the server " +
				"stops. Runs are serialized, so concurrent calls queue.",
		},
		{
			name:    "destroy",
			command: tf.CommandNameDestroy,
			description: "Run 'terragrunt run --all destroy' across the units under a directory and return a " +
				"per-unit result summary. THIS DESTROYS REAL INFRASTRUCTURE, in dependency order, and the " +
				"data in the resources it removes is gone. The call does not destroy anything on its own: " +
				"it first asks the person operating this client to approve the exact set of units, and " +
				"only their acceptance starts the destroy. A client that cannot ask them is refused. " +
				"Narrow the blast radius with the filter argument before calling. Each unit that printed " +
				"anything has stdout, the path of a file with its stdout, color stripped. A stdout file " +
				"keeps the last 8 MiB (stdout_truncated says when the start was cut) and stays until the " +
				"server stops.",
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
