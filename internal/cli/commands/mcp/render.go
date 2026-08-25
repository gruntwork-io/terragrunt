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

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zclconf/go-cty/cty"
)

type renderConfigInput struct {
	WorkingDir     string `json:"working_dir"               jsonschema:"Directory of the unit whose terragrunt.hcl to render, relative to the server root."`
	Format         string `json:"format,omitempty"          jsonschema:"summary (default) returns key facts; full returns the entire resolved configuration as JSON."`
	RenderMetadata bool   `json:"render_metadata,omitempty" jsonschema:"With format=full, wrap each value with metadata about where it was defined."`
}

type renderSummary struct {
	Source             string   `json:"source,omitempty"`
	RemoteStateBackend string   `json:"remote_state_backend,omitempty"`
	IAMRole            string   `json:"iam_role,omitempty"`
	InputKeys          []string `json:"input_keys,omitempty"`
	Dependencies       []string `json:"dependencies,omitempty"`
	GenerateBlocks     []string `json:"generate_blocks,omitempty"`
}

// renderConfigOutput is the render_config result. Do not narrow the config to
// [json.RawMessage]. The SDK validates marshaled output against the schema it
// infers from this struct, and it infers RawMessage as an array of bytes.
type renderConfigOutput struct {
	Config     map[string]any `json:"config,omitempty"`
	Summary    *renderSummary `json:"summary,omitempty"`
	ConfigPath string         `json:"config_path"`
	Degraded   []string       `json:"degraded,omitempty"`
}

// renderFormat selects how much of the resolved configuration the tool
// returns.
type renderFormat int

const (
	renderFormatSummary renderFormat = iota
	renderFormatFull
)

const (
	renderFormatNameSummary = "summary"
	renderFormatNameFull    = "full"
)

func registerRenderConfig(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "render_config",
		Description: "Fully evaluate one unit's terragrunt.hcl, resolving locals, includes, functions, and " +
			"dependency outputs, and return it as structured data. Use it to answer 'what will this unit " +
			"actually get as inputs/source/backend'. Prefer format=summary; use format=full only when you " +
			"need every resolved value. Without --allow=exec, dependency outputs degrade to mock_outputs " +
			"and run_cmd() returns empty strings; the degraded list says exactly what was substituted.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  new(d.reachesNetwork()),
		},
	}, parseToolHandler(l, d, "render_config", rootVenv, runRenderConfig))
}

func runRenderConfig(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input renderConfigInput,
) (renderConfigOutput, error) {
	format, err := renderParseFormat(input.Format)
	if err != nil {
		return renderConfigOutput{}, err
	}

	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return renderConfigOutput{}, err
	}

	// The "render" command stamped here makes a failed dependency-output
	// fetch fall back to mock_outputs rather than failing the parse outright.
	// See config.getTerragruntOutput.
	opts, env, err := buildUnitOptions(d, rootVenv, dir, "render")
	if err != nil {
		return renderConfigOutput{}, err
	}

	if d.allowExec {
		// Dependency-output fetches on this path drive real runs (init,
		// source download) into the shared per-unit cache dirs.
		release, err := d.acquireRunSlot(ctx)
		if err != nil {
			return renderConfigOutput{}, err
		}

		defer release()
	}

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	// Parse through PrepareConfig and nothing else. It builds the parsing
	// context around the venv it is handed, which puts run_cmd() and the
	// other HCL functions on the per-call exec handler instead of the OS.
	prepared, err := prepare.PrepareConfig(ctx, l, cv, opts)
	if err != nil {
		return renderConfigOutput{}, renderExplainDenials(err, d.rec)
	}

	out := renderConfigOutput{
		ConfigPath: renderRelPath(d.launchDir, opts.TerragruntConfigPath),
		Degraded:   d.rec.notes(),
	}

	if format == renderFormatSummary {
		out.Summary = renderSummarize(prepared.Cfg)

		return out, nil
	}

	toCty := config.TerragruntConfigAsCty
	if input.RenderMetadata {
		toCty = config.TerragruntConfigAsCtyWithMetadata
	}

	ctyVal, err := toCty(prepared.Cfg)
	if err != nil {
		return renderConfigOutput{}, fmt.Errorf("converting resolved config to cty: %w", err)
	}

	cfgMap, err := ctyhelper.ParseCtyValueToMap(ctyVal)
	if err != nil {
		return renderConfigOutput{}, fmt.Errorf("serializing resolved config: %w", err)
	}

	out.Config = cfgMap

	return out, nil
}

// renderParseFormat maps the tool-call format argument onto [renderFormat].
// An empty argument means summary.
func renderParseFormat(name string) (renderFormat, error) {
	switch name {
	case "", renderFormatNameSummary:
		return renderFormatSummary, nil
	case renderFormatNameFull:
		return renderFormatFull, nil
	}

	return renderFormatSummary, fmt.Errorf(
		"unsupported format %q: expected %q or %q",
		name,
		renderFormatNameSummary,
		renderFormatNameFull,
	)
}

// renderExplainDenials turns a parse failure the per-call denials caused into
// guidance the agent can act on. A dependency with neither applied outputs nor
// mock_outputs surfaces as an HCL evaluation error long after the denied `tofu
// output` call, so a recorded denial marks the failure as an artifact of the
// sandbox as much as [ErrExecDenied] in the chain does.
func renderExplainDenials(err error, rec *execRecorder) error {
	if errors.Is(err, ErrExecDenied) {
		return fmt.Errorf(
			"rendering this unit needs subprocess execution; restart the MCP server with --allow=exec: %w",
			err,
		)
	}

	if errors.Is(err, ErrSopsDenied) {
		return fmt.Errorf(
			"rendering this unit needs a SOPS decrypt; restart the MCP server with --allow=sops: %w",
			err,
		)
	}

	if errors.Is(err, vhttp.ErrNoNetwork) {
		return fmt.Errorf(
			"rendering this unit needs outbound HTTP; restart the MCP server with --allow=http: %w",
			err,
		)
	}

	notes := rec.notes()
	if len(notes) == 0 {
		return err
	}

	// Reached whenever the chain no longer carries the sentinel, which is most
	// of the time: an HCL function failure is rebuilt as a diagnostic and the
	// Go error it wrapped is lost. Prescribing a capability here would name
	// the wrong one, so the notes speak instead, each already saying which
	// one would have allowed it.
	return fmt.Errorf(
		"rendering failed after a denied invocation (denied: %s): %w",
		strings.Join(notes, "; "), err)
}

// renderSummarize distills the resolved configuration into the facts agents
// usually need, instead of the full config body.
func renderSummarize(cfg *config.TerragruntConfig) *renderSummary {
	s := &renderSummary{
		IAMRole:        cfg.IamRole,
		InputKeys:      slices.Sorted(maps.Keys(cfg.Inputs)),
		GenerateBlocks: slices.Sorted(maps.Keys(cfg.GenerateConfigs)),
	}

	if cfg.Terraform != nil && cfg.Terraform.Source != nil {
		s.Source = *cfg.Terraform.Source
	}

	if cfg.RemoteState != nil && cfg.RemoteState.Config != nil {
		s.RemoteStateBackend = cfg.RemoteState.BackendName
	}

	for i := range cfg.TerragruntDependencies {
		s.Dependencies = append(s.Dependencies, renderDependencyRef(&cfg.TerragruntDependencies[i]))
	}

	return s
}

// renderDependencyRef renders one dependency block as "name: config_path",
// degrading to the bare name when the config_path value is not a plain
// string.
func renderDependencyRef(dep *config.Dependency) string {
	if !dep.ConfigPath.IsNull() && dep.ConfigPath.Type() == cty.String {
		return fmt.Sprintf("%s: %s", dep.Name, dep.ConfigPath.AsString())
	}

	return dep.Name
}

// renderRelPath relativizes target against the server launch dir so
// config_path identifies the unit within the served tree, falling back to
// the absolute path when it cannot be expressed relatively.
func renderRelPath(launchDir, target string) string {
	rel, err := filepath.Rel(launchDir, target)
	if err != nil {
		return target
	}

	return rel
}
