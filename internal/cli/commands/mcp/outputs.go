package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zclconf/go-cty/cty"
)

// outputsSensitivePlaceholder replaces sensitive output values unless the
// caller asks for them with include_sensitive.
const outputsSensitivePlaceholder = "(sensitive)"

// outputsSensitivity selects what a sensitive output's value comes back as.
type outputsSensitivity int

const (
	outputsRedactSensitive outputsSensitivity = iota
	outputsRevealSensitive
)

// outputsSensitivityFor maps the tool-call argument onto what a sensitive
// value comes back as.
func outputsSensitivityFor(includeSensitive bool) outputsSensitivity {
	if includeSensitive {
		return outputsRevealSensitive
	}

	return outputsRedactSensitive
}

type getOutputsInput struct {
	WorkingDir       string `json:"working_dir"                 jsonschema:"Directory of the unit whose OpenTofu/Terraform outputs to read, relative to the server root."`
	IncludeSensitive bool   `json:"include_sensitive,omitempty" jsonschema:"Include values of outputs marked sensitive. Default redacts them."`
}

// outputEntry holds one output. Do not narrow the value to
// [json.RawMessage]. The SDK validates marshaled output against the schema it
// infers from this struct, and it infers RawMessage as an array of bytes.
type outputEntry struct {
	Value     any  `json:"value,omitempty"`
	Sensitive bool `json:"sensitive,omitempty"`
}

type getOutputsOutput struct {
	Outputs  map[string]outputEntry `json:"outputs"`
	UnitDir  string                 `json:"unit_dir"`
	Redacted []string               `json:"redacted,omitempty"`
	Degraded []string               `json:"degraded,omitempty"`
}

// outputsTFMeta is the per-output shape of `tofu output -json`. RawMessage is
// safe here because this struct never reaches the SDK's schema inference.
type outputsTFMeta struct {
	Value     json.RawMessage `json:"value"`
	Sensitive bool            `json:"sensitive"`
}

func registerGetOutputs(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_outputs",
		Description: "Read the applied OpenTofu/Terraform outputs of one unit. Use when another unit's " +
			"inputs reference dependency outputs and you need the real values. With --allow=exec this runs " +
			"'terragrunt output -json' for full fidelity; sensitive values are replaced with \"(sensitive)\" " +
			"unless include_sensitive=true. Without --allow=exec the server can only read the remote state " +
			"in-process, which the s3, gcs, and azurerm backends support: sensitivity is unknown on that path, " +
			"so nothing is redacted and the degraded list says so; a unit whose state cannot be read that way " +
			"returns empty outputs with a degraded explanation.",
		Annotations: &mcp.ToolAnnotations{
			// Under --allow=exec this drives the whole run pipeline, which
			// downloads sources, runs init, and fires hooks. That writes as
			// much as plan does, so the tool claims to be read-only only
			// when the in-process state read is all it can do.
			ReadOnlyHint:   !d.allowExec,
			IdempotentHint: true,
			OpenWorldHint:  new(d.reachesNetwork()),
		},
	}, toolHandler(l, "get_outputs", func(ctx context.Context, input getOutputsInput) (getOutputsOutput, error) {
		return runGetOutputs(ctx, l, d.forCall(nil), rootVenv, input)
	}))
}

func runGetOutputs(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input getOutputsInput,
) (getOutputsOutput, error) {
	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return getOutputsOutput{}, err
	}

	if d.allowExec {
		return outputsFetchViaRun(
			ctx,
			l,
			d,
			rootVenv,
			dir,
			outputsSensitivityFor(input.IncludeSensitive),
		)
	}

	return outputsFetchViaStateRead(ctx, l, d, rootVenv, dir)
}

// outputsFetchViaRun runs the unit's `output -json` through the full
// single-unit run path, which yields the raw tofu JSON and with it the
// per-output sensitivity flags the redaction needs.
func outputsFetchViaRun(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	dir string,
	sensitivity outputsSensitivity,
) (getOutputsOutput, error) {
	// This path drives a real run (source download, auto-init) into the
	// shared per-unit cache dirs.
	release, err := d.acquireRunSlot(ctx)
	if err != nil {
		return getOutputsOutput{}, err
	}

	defer release()

	opts, env, err := buildUnitOptions(d, rootVenv, dir, tf.CommandNameOutput, "-json")
	if err != nil {
		return getOutputsOutput{}, err
	}

	// `output` is a force-forwarded command, so its raw JSON streams to the
	// call venv's primary writer.
	buf := &bytes.Buffer{}

	cv := d.callVenv(rootVenv, env, buf)

	ctx = freshCallContext(ctx)

	// No-op with the fresh options' empty AuthProviderCmd, but keeps the
	// credential flow identical to the CLI single-unit run path.
	credsGetter := creds.NewGetter()
	if err := credsGetter.ObtainAndUpdateEnvIfNecessary(
		ctx,
		l,
		cv,
		externalcmd.NewProvider(
			l,
			opts.AuthProviderCmd,
			configbridge.ShellRunOptsFromOpts(env, opts),
		),
	); err != nil {
		return getOutputsOutput{}, err
	}

	parseCtx, pctx := configbridge.NewParsingContext(ctx, l, cv, opts)

	cfg, err := config.ReadTerragruntConfig(parseCtx, l, pctx, pctx.ParserOptions)
	if err != nil {
		return getOutputsOutput{}, err
	}

	runCfg := cfg.ToRunConfig(l, cv.FS)

	if err := run.Run(
		parseCtx,
		l,
		cv,
		configbridge.NewRunOptions(opts),
		report.NewReport(),
		runCfg,
		credsGetter,
	); err != nil {
		return getOutputsOutput{}, err
	}

	raw, err := tf.ExtractFirstJSONObject(buf.Bytes())
	if err != nil {
		return getOutputsOutput{}, fmt.Errorf("parsing output JSON for %s: %w", dir, err)
	}

	var metas map[string]outputsTFMeta
	if err := json.Unmarshal(raw, &metas); err != nil {
		return getOutputsOutput{}, fmt.Errorf("decoding output JSON for %s: %w", dir, err)
	}

	out := getOutputsOutput{
		UnitDir: dir,
		Outputs: make(map[string]outputEntry, len(metas)),
	}

	for name, meta := range metas {
		entry := outputEntry{Sensitive: meta.Sensitive}

		if meta.Sensitive && sensitivity == outputsRedactSensitive {
			entry.Value = outputsSensitivePlaceholder
			out.Outputs[name] = entry
			out.Redacted = append(out.Redacted, name)

			continue
		}

		if len(meta.Value) > 0 {
			var v any
			if err := json.Unmarshal(meta.Value, &v); err != nil {
				return getOutputsOutput{}, fmt.Errorf(
					"decoding output %q of %s: %w",
					name,
					dir,
					err,
				)
			}

			entry.Value = v
		}

		out.Outputs[name] = entry
	}

	slices.Sort(out.Redacted)

	out.Degraded = d.rec.notes()

	return out, nil
}

// outputsFetchViaStateRead reads the unit's outputs without spawning a
// subprocess, via the dependency-fetch machinery's direct s3 state read. When
// that path is not viable the tool returns empty outputs with a degraded
// explanation rather than failing. Nothing better is available, because
// mock_outputs are declared by dependents, so a unit has no mock of its own.
func outputsFetchViaStateRead(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	dir string,
) (getOutputsOutput, error) {
	// The "render" tag makes any dependency-output evaluation the machinery
	// performs fall back to mock_outputs instead of hard-failing the parse.
	opts, env, err := buildUnitOptions(d, rootVenv, dir, "render")
	if err != nil {
		return getOutputsOutput{}, err
	}

	out := getOutputsOutput{
		UnitDir: dir,
		Outputs: map[string]outputEntry{},
	}

	// Every in-process read is a request to the state's backend, so a denied
	// network rules it out before the tree is copied or the remote_state
	// block probed.
	if !d.allowHTTP {
		out.Degraded = []string{
			"outputs not fetched: reading remote state in-process needs outbound HTTP; restart the MCP server " +
				"with --allow=http, or with --allow=exec to run the real output command",
		}

		return out, nil
	}

	// The direct state read is gated behind an experiment; enable it on a
	// per-call clone so the enablement never leaks into other tool calls.
	opts.Experiments = outputsCloneExperimentsWithStateFetch(d.baseOpts.Experiments)

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	parseCtx, pctx := configbridge.NewParsingContext(ctx, l, cv, opts)

	viable, reason := outputsStateReadViable(parseCtx, l, pctx, opts.TerragruntConfigPath)
	if !viable {
		out.Degraded = append(d.rec.notes(), fmt.Sprintf(
			"outputs not fetched: %s; restart the MCP server with --allow=exec for real outputs, or use render_config on dependent units to see their mock_outputs",
			reason,
		))

		return out, nil
	}

	unit := &config.Unit{Name: filepath.Base(dir), Path: dir}

	outputMap, err := unit.ReadOutputs(parseCtx, l, pctx, dir)
	if err != nil {
		// A denied subprocess means the machinery gave up on the in-process
		// path (e.g. a nested dependency needed a real fetch); degrade
		// instead of surfacing a sandbox artifact as a config bug.
		if errors.Is(err, ErrExecDenied) {
			out.Degraded = append(d.rec.notes(), fmt.Sprintf(
				"outputs not fetched: reading them required a subprocess (%v); restart the MCP server with --allow=exec",
				err,
			))

			return out, nil
		}

		return getOutputsOutput{}, err
	}

	vals, err := ctyhelper.ParseCtyValueToMap(cty.ObjectVal(outputMap))
	if err != nil {
		return getOutputsOutput{}, fmt.Errorf("converting outputs of %s: %w", dir, err)
	}

	for name, v := range vals {
		out.Outputs[name] = outputEntry{Value: v}
	}

	out.Degraded = append(
		d.rec.notes(),
		"outputs read in-process from the unit's remote state: sensitivity flags are unavailable on this path, so no values were redacted",
	)

	return out, nil
}

// outputsStateReadViable reports whether the unit's remote_state block allows
// the in-process state read, deferring the per-backend rules to
// [config.ShouldFetchDependencyOutputFromState] so this never disagrees with
// the backends that optimization actually supports. The returned reason is
// user-facing and only set when not viable.
func outputsStateReadViable(
	ctx context.Context,
	l log.Logger,
	pctx *config.ParsingContext,
	configPath string,
) (bool, string) {
	probeCtx := pctx.
		WithDecodeList(config.RemoteStateBlock, config.TerragruntFlags, config.EngineBlock).
		WithDiagnosticsSuppressed(l)

	cfg, err := config.PartialParseConfigFile(ctx, probeCtx, l, configPath, nil)
	if err != nil {
		// A remote_state block that references dependency outputs cannot be
		// parsed standalone, which also rules out the direct state read.
		return false, fmt.Sprintf(
			"could not parse the unit's remote_state block without a full run: %v",
			err,
		)
	}

	remoteState := cfg.RemoteState
	if remoteState == nil || remoteState.Config == nil || remoteState.BackendName == "" {
		return false, "the unit declares no remote_state block, so there is no in-process state read path"
	}

	if remoteState.DisableDependencyOptimization {
		return false, "the unit's remote_state block sets disable_dependency_optimization"
	}

	if !config.ShouldFetchDependencyOutputFromState(pctx, remoteState) {
		return false, fmt.Sprintf(
			"no in-process read path for a %q remote_state block configured this way",
			remoteState.BackendName,
		)
	}

	return true, ""
}

// outputsCloneExperimentsWithStateFetch deep-copies exps and enables the
// dependency-fetch-output-from-state experiment on the copy, making the direct
// state read viable for this call without mutating the server-wide settings.
func outputsCloneExperimentsWithStateFetch(exps experiment.Experiments) experiment.Experiments {
	cloned := make(experiment.Experiments, 0, len(exps))

	for _, exp := range exps {
		expCopy := *exp
		cloned = append(cloned, &expCopy)
	}

	if exp := cloned.Find(experiment.DependencyFetchOutputFromState); exp != nil {
		exp.Enabled = true
	}

	return cloned
}
