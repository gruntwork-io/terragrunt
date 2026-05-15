package list

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"

	"charm.land/lipgloss/v2/tree"
	"github.com/charmbracelet/x/term"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view/dag"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Run runs the list command.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, w io.Writer, opts *Options) error {
	d, err := discovery.NewForDiscoveryCommand(l, &discovery.DiscoveryCommandOptions{
		WorkingDir:        opts.WorkingDir,
		QueueConstructAs:  opts.QueueConstructAs,
		NoHidden:          opts.NoHidden,
		WithRequiresParse: opts.Dependencies || opts.Mode == ModeDAG,
		WithRelationships: opts.Dependencies || opts.Mode == ModeDAG,
		Filters:           opts.Filters,
		Experiments:       opts.Experiments,
	})
	if err != nil {
		return errors.New(err)
	}

	// We do worktree generation here instead of in the discovery constructor
	// so that we can defer cleanup in the same context.
	gitFilters := opts.Filters.UniqueGitFilters()

	worktrees, worktreeErr := worktrees.NewWorktrees(ctx, l, worktrees.WorktreeOpts{
		WorkingDir:     opts.WorkingDir,
		GitExpressions: gitFilters,
		Experiments:    opts.Experiments,
	})
	if worktreeErr != nil {
		return errors.Errorf("failed to create worktrees: %w", worktreeErr)
	}

	defer func() {
		cleanupErr := worktrees.Cleanup(ctx, l)
		if cleanupErr != nil {
			l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
		}
	}()

	d = d.WithWorktrees(worktrees)

	var (
		components  component.Components
		discoverErr error
	)

	// Wrap discovery with telemetry
	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "list_discover", map[string]any{
		"working_dir":  opts.WorkingDir,
		"no_hidden":    opts.NoHidden,
		"dependencies": opts.Dependencies || opts.Mode == ModeDAG,
	}, func(ctx context.Context) error {
		components, discoverErr = d.Discover(ctx, l, v, opts.TerragruntOptions)

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			span.SetAttributes(attribute.Int("component_count", len(components)))
		}

		return discoverErr
	})
	if err != nil {
		l.Debugf("Errors encountered while discovering components:\n%s", err)
	}

	switch opts.Mode {
	case ModeNormal:
		components = components.Sort()
	case ModeDAG:
		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "list_mode_dag", map[string]any{
			"working_dir":  opts.WorkingDir,
			"config_count": len(components),
		}, func(ctx context.Context) error {
			q, queueErr := queue.NewQueue(components)
			if queueErr != nil {
				return queueErr
			}

			components = q.Components()

			return nil
		})
		if err != nil {
			return errors.New(err)
		}
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid mode: " + opts.Mode)
	}

	var listedComponents dag.ListedComponents

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "list_discovered_to_listed", map[string]any{
		"working_dir":  opts.WorkingDir,
		"config_count": len(components),
	}, func(ctx context.Context) error {
		listedComponents = discoveredToListed(l, components, opts)

		if span := trace.SpanFromContext(ctx); span.IsRecording() {
			span.SetAttributes(attribute.Int("listed_count", len(listedComponents)))
		}

		return nil
	})
	if err != nil {
		return errors.New(err)
	}

	switch opts.Format {
	case FormatText:
		return outputText(l, w, listedComponents)
	case FormatTree:
		return outputTree(l, w, opts, listedComponents, opts.Mode)
	case FormatLong:
		return outputLong(l, w, opts, listedComponents)
	case FormatDot:
		return outputDot(w, listedComponents)
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid format: " + opts.Format)
	}
}

func discoveredToListed(l log.Logger, components component.Components, opts *Options) dag.ListedComponents {
	listedComponents := make(dag.ListedComponents, 0, len(components))

	for _, c := range components {
		excluded := false

		if opts.QueueConstructAs != "" {
			if unit, ok := c.(*component.Unit); ok {
				if cfg := unit.Config(); cfg != nil && cfg.Exclude != nil {
					if cfg.Exclude.IsActionListed(opts.QueueConstructAs) {
						if opts.Format != FormatDot {
							continue
						}

						excluded = true
					}
				}
			}
		}

		base := opts.WorkingDir
		if c.DiscoveryContext() != nil && c.DiscoveryContext().WorkingDir != "" {
			base = c.DiscoveryContext().WorkingDir
		}

		listedCfg := &dag.ListedComponent{
			Type:     c.Kind(),
			Path:     discovery.RelPathOrAbs(l, base, c.Path(), "component"),
			Excluded: excluded,
		}

		if len(c.Dependencies()) == 0 {
			listedComponents = append(listedComponents, listedCfg)

			continue
		}

		listedCfg.Dependencies = make([]*dag.ListedComponent, len(c.Dependencies()))

		desc := fmt.Sprintf("dependency of unit %q", c.Path())
		for i, dep := range c.Dependencies() {
			depBase := opts.WorkingDir
			if dep.DiscoveryContext() != nil && dep.DiscoveryContext().WorkingDir != "" {
				depBase = dep.DiscoveryContext().WorkingDir
			}

			depExcluded := false

			if opts.QueueConstructAs != "" {
				if depUnit, ok := dep.(*component.Unit); ok {
					if depCfg := depUnit.Config(); depCfg != nil && depCfg.Exclude != nil {
						if depCfg.Exclude.IsActionListed(opts.QueueConstructAs) {
							depExcluded = true
						}
					}
				}
			}

			listedCfg.Dependencies[i] = &dag.ListedComponent{
				Type:     dep.Kind(),
				Path:     discovery.RelPathOrAbs(l, depBase, dep.Path(), desc),
				Excluded: depExcluded,
			}
		}

		sort.SliceStable(listedCfg.Dependencies, func(i, j int) bool {
			return listedCfg.Dependencies[i].Path < listedCfg.Dependencies[j].Path
		})

		listedComponents = append(listedComponents, listedCfg)
	}

	return listedComponents
}

// outputText outputs the discovered components in text format.
func outputText(l log.Logger, w io.Writer, components dag.ListedComponents) error {
	colorizer := dag.NewColorizer(shouldColor(l))

	return renderTabular(w, components, colorizer)
}

// outputLong outputs the discovered components in long format.
func outputLong(l log.Logger, w io.Writer, opts *Options, components dag.ListedComponents) error {
	colorizer := dag.NewColorizer(shouldColor(l))

	return renderLong(w, opts, components, colorizer)
}

// shouldColor returns true if the output should be colored.
func shouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !stdout.IsRedirected()
}

// renderLong renders the components in a long format.
func renderLong(w io.Writer, opts *Options, components dag.ListedComponents, c *dag.Colorizer) error {
	var buf strings.Builder

	longestPathLen := getLongestPathLen(components)

	buf.WriteString(buildLongHeadings(opts, c, longestPathLen))

	for _, component := range components {
		buf.WriteString(c.ColorizeType(component.Type))
		buf.WriteString(" " + c.Colorize(component))

		if opts.Dependencies && len(component.Dependencies) > 0 {
			colorizedDeps := make([]string, 0, len(component.Dependencies))

			for _, dep := range component.Dependencies {
				colorizedDeps = append(colorizedDeps, c.Colorize(dep))
			}

			const extraDependenciesPadding = 2

			dependenciesPadding := (longestPathLen - len(component.Path)) + extraDependenciesPadding
			for range dependenciesPadding {
				buf.WriteString(" ")
			}

			buf.WriteString(strings.Join(colorizedDeps, ", "))
		}

		buf.WriteString("\n")
	}

	_, err := w.Write([]byte(buf.String()))

	return errors.New(err)
}

// buildLongHeadings renders the headings for the long format.
func buildLongHeadings(opts *Options, c *dag.Colorizer, longestPathLen int) string {
	var buf strings.Builder

	buf.WriteString(c.ColorizeHeading("Type  Path"))

	if opts.Dependencies {
		const extraDependenciesPadding = 2

		dependenciesPadding := (longestPathLen - len("Path")) + extraDependenciesPadding
		for range dependenciesPadding {
			buf.WriteString(" ")
		}

		buf.WriteString(c.ColorizeHeading("Dependencies"))
	}

	buf.WriteString("\n")

	return buf.String()
}

// renderTabular renders the components in a tabular format.
func renderTabular(w io.Writer, components dag.ListedComponents, c *dag.Colorizer) error {
	var buf strings.Builder

	maxCols, colWidth := getMaxCols(components)

	for i, component := range components {
		if i > 0 && i%maxCols == 0 {
			buf.WriteString("\n")
		}

		buf.WriteString(c.Colorize(component))

		// Add padding until the length of maxCols
		padding := colWidth - len(component.Path)
		for range padding {
			buf.WriteString(" ")
		}
	}

	buf.WriteString("\n")

	_, err := w.Write([]byte(buf.String()))

	return errors.New(err)
}

// outputTree outputs the discovered components in tree format.
func outputTree(l log.Logger, w io.Writer, opts *Options, components dag.ListedComponents, sort string) error {
	s := dag.NewTreeStyler(shouldColor(l))

	return renderTree(w, opts, components, s, sort)
}

// outputDot outputs the discovered components in GraphViz DOT format.
func outputDot(w io.Writer, components dag.ListedComponents) error {
	return renderDot(w, components)
}

// generateTree creates a tree structure from dag.ListedComponents
func generateTree(components dag.ListedComponents, s *dag.TreeStyler) *tree.Tree {
	root := tree.Root(".")
	nodes := make(map[string]*tree.Tree)

	for _, c := range components {
		parts := preProcessPath(c.Path)
		if len(parts.segments) == 0 || (len(parts.segments) == 1 && parts.segments[0] == ".") {
			continue
		}

		currentPath := "."
		currentNode := root

		for i, segment := range parts.segments {
			nextPath := filepath.Join(currentPath, segment)
			if _, exists := nodes[nextPath]; !exists {
				componentType := component.StackKind

				if c.Type == component.UnitKind && i == len(parts.segments)-1 {
					componentType = component.UnitKind
				}

				tmpCfg := &dag.ListedComponent{
					Type: componentType,
					Path: segment,
				}

				newNode := tree.New().Root(s.Colorizer().Colorize(tmpCfg))
				nodes[nextPath] = newNode
				currentNode.Child(newNode)
			}

			currentNode = nodes[nextPath]
			currentPath = nextPath
		}
	}

	return root
}

// pathParts holds the pre-processed parts of a component path.
type pathParts struct {
	dir      string
	base     string
	segments []string
}

// preProcessPath splits a path into its components.
func preProcessPath(path string) pathParts {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	segments := strings.Split(path, string(os.PathSeparator))

	return pathParts{
		dir:      dir,
		base:     base,
		segments: segments,
	}
}

// renderTree renders the components in a tree format.
func renderTree(w io.Writer, opts *Options, components dag.ListedComponents, s *dag.TreeStyler, _ string) error {
	var t *tree.Tree

	if opts.Mode == ModeDAG {
		t = dag.GenerateDAGTree(components, s)
	} else {
		t = generateTree(components, s)
	}

	t = s.Style(t)

	_, err := w.Write([]byte(t.String() + "\n"))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// getMaxCols returns the maximum number of columns
// that can be displayed in the terminal.
// It also returns the width of each column.
// The width is the longest path length + 2 for padding.
func getMaxCols(components dag.ListedComponents) (int, int) {
	maxCols := 0

	terminalWidth := getTerminalWidth()
	longestPathLen := getLongestPathLen(components)

	const padding = 2

	colWidth := longestPathLen + padding

	if longestPathLen > 0 {
		maxCols = terminalWidth / colWidth
	}

	if maxCols == 0 {
		maxCols = 1
	}

	return maxCols, colWidth
}

// getTerminalWidth returns the width of the terminal.
func getTerminalWidth() int {
	// Default to 80 if we can't get the terminal width.
	width := 80

	cols, _, err := term.GetSize(os.Stdout.Fd())
	if err == nil {
		width = cols
	}

	return width
}

// getLongestPathLen returns the length of the
// longest path in the list of components.
func getLongestPathLen(components dag.ListedComponents) int {
	longest := 0

	for _, c := range components {
		if len(c.Path) > longest {
			longest = len(c.Path)
		}
	}

	return longest
}

// renderDot renders the components in GraphViz DOT format.
func renderDot(w io.Writer, components dag.ListedComponents) error {
	var buf strings.Builder

	buf.WriteString("digraph {\n")

	sortedComponents := make(dag.ListedComponents, len(components))
	copy(sortedComponents, components)
	sort.Slice(sortedComponents, func(i, j int) bool {
		return sortedComponents[i].Path < sortedComponents[j].Path
	})

	for _, component := range sortedComponents {
		if len(component.Dependencies) > 1 {
			sort.Slice(component.Dependencies, func(i, j int) bool {
				return component.Dependencies[i].Path < component.Dependencies[j].Path
			})
		}
	}

	for _, component := range sortedComponents {
		style := ""
		if component.Excluded {
			style = "[color=red]"
		}

		fmt.Fprintf(&buf, "\t\"%s\" %s;\n", component.Path, style)

		for _, dep := range component.Dependencies {
			fmt.Fprintf(&buf, "\t\"%s\" -> \"%s\";\n", component.Path, dep.Path)
		}
	}

	buf.WriteString("}\n")

	_, err := w.Write([]byte(buf.String()))

	return errors.New(err)
}
