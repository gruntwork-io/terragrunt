package mcp

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	dagview "github.com/gruntwork-io/terragrunt/internal/view/dag"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// runOrderDirection selects which dependency edges gate a unit's group. Apply
// waits on the unit's dependencies, destroy on its dependents.
type runOrderDirection string

const (
	runOrderApply   runOrderDirection = "apply"
	runOrderDestroy runOrderDirection = "destroy"
)

// Renderings run_order can return alongside the blocking edges.
const (
	runOrderFormatEdges = "edges"
	runOrderFormatTree  = "tree"
	runOrderFormatDot   = "dot"
)

// runOrderParseCommand maps the tool-call command argument onto the command to
// evaluate exclude blocks against and the direction its dependencies run in.
// Everything except destroy runs dependencies first, which is why plan and
// apply share a direction while still covering different units.
func runOrderParseCommand(name string) (string, runOrderDirection, error) {
	switch name {
	case "", tf.CommandNamePlan:
		return tf.CommandNamePlan, runOrderApply, nil
	case tf.CommandNameApply:
		return tf.CommandNameApply, runOrderApply, nil
	case tf.CommandNameDestroy:
		return tf.CommandNameDestroy, runOrderDestroy, nil
	}

	return "", "", fmt.Errorf("unsupported command %q: expected %q, %q or %q",
		name, tf.CommandNamePlan, tf.CommandNameApply, tf.CommandNameDestroy)
}

// runOrderParseFormat maps the tool-call format argument onto a rendering. An
// empty argument means edges alone.
func runOrderParseFormat(name string) (string, error) {
	switch name {
	case "", runOrderFormatEdges:
		return runOrderFormatEdges, nil
	case runOrderFormatTree, runOrderFormatDot:
		return name, nil
	}

	return "", fmt.Errorf("unsupported format %q: expected %q, %q or %q",
		name, runOrderFormatEdges, runOrderFormatTree, runOrderFormatDot)
}

type runOrderInput struct {
	WorkingDir string   `json:"working_dir,omitempty" jsonschema:"Directory whose units to order. Defaults to the server root."`
	Command    string   `json:"command,omitempty"     jsonschema:"The command the order is for: plan (default), apply, or destroy. plan and apply are ordered the same way, dependencies first; destroy runs dependents first. The command also decides which exclude blocks apply, so plan and apply can cover different units."`
	Format     string   `json:"format,omitempty"      jsonschema:"edges (default) returns each unit and what blocks it; tree and dot additionally render the graph the way the list command does, for a person to read."`
	Filter     []string `json:"filter,omitempty"      jsonschema:"Terragrunt filter queries to narrow the set of units."`
}

// runOrderUnit is one unit and what must finish before it may start, which is
// its dependencies for a plan or an apply and its dependents for a destroy.
type runOrderUnit struct {
	Path      string   `json:"path"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

type runOrderOutput struct {
	Command  string            `json:"command"`
	Order    runOrderDirection `json:"order"`
	Units    []runOrderUnit    `json:"units"`
	Graph    string            `json:"graph,omitempty"`
	Degraded []string          `json:"degraded,omitempty"`
}

func registerRunOrder(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "run_order",
		Description: "Compute what blocks what for the units under a directory. Each unit is returned with " +
			"the units that must finish before it may start, which for command=destroy are its dependents " +
			"rather than its dependencies. Terragrunt starts a unit as soon as the units blocking it have " +
			"finished. Units come back sorted by path rather than in the order they run. " +
			"Ask about the command you actually intend to run, because plan and apply are " +
			"ordered alike but exclude blocks can drop different units from each. Pass format=tree or " +
			"format=dot for a rendering to show a person. Use before proposing a " +
			"multi-unit change to know blast-radius ordering, or set order=destroy to get teardown order. " +
			"Ordering comes from parsed dependency edges; when the server runs without --allow=exec, configs " +
			"using run_cmd() may parse degraded and lose edges, so check the degraded list.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  new(d.reachesNetwork()),
		},
	}, parseToolHandler(l, d, "run_order", rootVenv, runRunOrder))
}

func runRunOrder(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input runOrderInput,
) (runOrderOutput, error) {
	format, err := runOrderParseFormat(input.Format)
	if err != nil {
		return runOrderOutput{}, err
	}

	command, direction, err := runOrderParseCommand(input.Command)
	if err != nil {
		return runOrderOutput{}, err
	}

	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return runOrderOutput{}, err
	}

	opts, env, err := buildDirOptions(l, d, rootVenv, dir, input.Filter)
	if err != nil {
		return runOrderOutput{}, err
	}

	// The command is stamped here and again as QueueConstructAs below.
	// Exclude-block evaluation matches actions against TerraformCommand,
	// where an empty command never matches, and the queue reads its up/down
	// semantics from the DiscoveryContext that QueueConstructAs stamps.
	// Stamping plan for an apply would answer with the wrong set of units,
	// since an exclude block naming one action does not match the other.
	opts.TerraformCommand = command

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	disc, err := discovery.NewForDiscoveryCommand(l, cv.FS, &discovery.DiscoveryCommandOptions{
		WorkingDir:        dir,
		DiscoveryBoundary: opts.DiscoveryBoundary,
		QueueConstructAs:  command,
		Filters:           opts.Filters,
		WithRequiresParse: true,
		WithRelationships: true,
	})
	if err != nil {
		return runOrderOutput{}, err
	}

	w, cleanup, err := setupGitFilterWorktrees(ctx, l, d, cv, opts)
	if err != nil {
		return runOrderOutput{}, err
	}

	if w != nil {
		defer cleanup()

		disc = disc.WithWorktrees(w)
	}

	components, degraded := discoverDegraded(ctx, l, cv, opts, disc, "run_order")

	entries, qErr := runOrderQueueEntries(components)
	if qErr != nil {
		// Discovery runs with cycle-breaking, so this is defensive; the
		// returned entries still describe what blocks what.
		l.Debugf("run_order tool: queue construction error: %v", qErr)

		degraded = append(degraded, fmt.Sprintf("queue construction reported an error: %v", qErr))
	}

	included := runOrderIncludedEntries(entries)

	out := runOrderOutput{
		Command:  command,
		Order:    direction,
		Units:    runOrderUnits(l, dir, included, direction),
		Degraded: append(degraded, d.rec.notes()...),
	}

	if format != runOrderFormatEdges {
		graph, err := runOrderRenderGraph(out.Units, format)
		if err != nil {
			return runOrderOutput{}, err
		}

		out.Graph = graph
	}

	return out, nil
}

// runOrderRenderGraph draws the blocking edges the way the list command draws
// them, so a person reading an agent's answer sees the shape they already know
// from the CLI rather than a third representation invented here.
func runOrderRenderGraph(units []runOrderUnit, format string) (string, error) {
	listed := make(dagview.ListedComponents, 0, len(units))
	byPath := make(map[string]*dagview.ListedComponent, len(units))

	for _, unit := range units {
		lc := &dagview.ListedComponent{Type: component.UnitKind, Path: unit.Path}
		byPath[unit.Path] = lc
		listed = append(listed, lc)
	}

	for i, unit := range units {
		for _, blocker := range unit.BlockedBy {
			if dep, ok := byPath[blocker]; ok {
				listed[i].Dependencies = append(listed[i].Dependencies, dep)

				continue
			}

			listed[i].Dependencies = append(listed[i].Dependencies, &dagview.ListedComponent{
				Type: component.UnitKind,
				Path: blocker,
			})
		}
	}

	b := &strings.Builder{}

	if format == runOrderFormatDot {
		if err := dagview.RenderDot(b, listed); err != nil {
			return "", fmt.Errorf("rendering the run order as dot: %w", err)
		}

		return b.String(), nil
	}

	return dagview.GenerateDAGTree(listed, dagview.NewTreeStyler(false)).String(), nil
}

// runOrderQueueEntries builds the run queue for the discovered components.
// A construction error (cycle) is returned alongside whatever entries were
// assembled so the caller can degrade instead of failing.
func runOrderQueueEntries(components component.Components) (queue.Entries, error) {
	q, err := queue.NewQueue(components)
	if q == nil {
		return nil, err
	}

	return q.Entries, err
}

// runOrderIncludedEntries drops non-unit entries and units an exclude block
// removed. An excluded unit is not degradation. A real run skips it too.
func runOrderIncludedEntries(entries queue.Entries) []*queue.Entry {
	included := make([]*queue.Entry, 0, len(entries))

	for _, e := range entries {
		unit, ok := e.Component.(*component.Unit)
		if !ok {
			continue
		}

		if unit.Excluded() {
			continue
		}

		included = append(included, e)
	}

	return included
}

// runOrderUnits maps the included entries to their output form. The blocking
// edges are the dependencies for an apply and the dependents for a destroy,
// matching what Terragrunt waits on in each direction.
func runOrderUnits(
	l log.Logger,
	workingDir string,
	included []*queue.Entry,
	direction runOrderDirection,
) []runOrderUnit {
	units := make([]runOrderUnit, 0, len(included))

	for _, e := range included {
		u := runOrderUnit{
			Path: discovery.RelPathForComponent(
				l,
				e.Component,
				workingDir,
				e.Component.Path(),
				"unit",
			),
		}

		blockers := e.Component.Dependencies()
		if direction == runOrderDestroy {
			blockers = e.Component.Dependents()
		}

		for _, blocker := range blockers {
			u.BlockedBy = append(
				u.BlockedBy,
				discovery.RelPathForComponent(
					l,
					blocker,
					workingDir,
					blocker.Path(),
					"blocking unit",
				),
			)
		}

		slices.Sort(u.BlockedBy)

		units = append(units, u)
	}

	slices.SortFunc(units, func(a, b runOrderUnit) int {
		return strings.Compare(a.Path, b.Path)
	})

	return units
}
