package list

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/charmbracelet/x/term"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
)

// Run runs the list command.
func Run(ctx context.Context, l log.Logger, opts *Options) error {
	d, err := discovery.NewForDiscoveryCommand(discovery.DiscoveryCommandOptions{
		WorkingDir:       opts.WorkingDir,
		QueueConstructAs: opts.QueueConstructAs,
		NoHidden:         !opts.Hidden,
		Dependencies:     shouldDiscoverDependencies(opts),
		External:         opts.External,
		FilterQueries:    opts.FilterQueries,
		Experiments:      opts.Experiments,
	})
	if err != nil {
		return errors.New(err)
	}

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		// We do worktree generation here instead of in the discovery constructor
		// so that we can defer cleanup in the same context.
		filters, parseErr := filter.ParseFilterQueries(opts.FilterQueries)
		if parseErr != nil {
			return fmt.Errorf("failed to parse filters: %w", parseErr)
		}

		gitFilters := filters.UniqueGitFilters()

		worktrees, worktreeErr := worktrees.NewWorktrees(ctx, l, opts.WorkingDir, gitFilters)
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
	}

	var (
		components  component.Components
		discoverErr error
	)

	// Wrap discovery with telemetry
	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "list_discover", map[string]any{
		"working_dir":  opts.WorkingDir,
		"hidden":       opts.Hidden,
		"dependencies": shouldDiscoverDependencies(opts),
		"external":     opts.External,
	}, func(ctx context.Context) error {
		components, discoverErr = d.Discover(ctx, l, opts.TerragruntOptions)
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

	var listedComponents ListedComponents

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "list_discovered_to_listed", map[string]any{
		"working_dir":  opts.WorkingDir,
		"config_count": len(components),
	}, func(ctx context.Context) error {
		var convErr error

		listedComponents, convErr = discoveredToListed(components, opts)

		return convErr
	})
	if err != nil {
		return errors.New(err)
	}

	switch opts.Format {
	case FormatText:
		return outputText(l, opts, listedComponents)
	case FormatTree:
		return outputTree(l, opts, listedComponents, opts.Mode)
	case FormatLong:
		return outputLong(l, opts, listedComponents)
	case FormatDot:
		return outputDot(l, opts, listedComponents)
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid format: " + opts.Format)
	}
}

// shouldDiscoverDependencies returns true if we should discover dependencies.
func shouldDiscoverDependencies(opts *Options) bool {
	if opts.Dependencies {
		return true
	}

	if opts.External {
		return true
	}

	if opts.Mode == ModeDAG {
		return true
	}

	return false
}

type ListedComponents []*ListedComponent

type ListedComponent struct {
	Type         component.Kind
	Path         string
	Dependencies []*ListedComponent
	Excluded     bool
}

// Contains checks to see if the given path is in the listed components.
func (l ListedComponents) Contains(path string) bool {
	for _, c := range l {
		if c.Path == path {
			return true
		}
	}

	return false
}

// Get returns the component with the given path.
func (l ListedComponents) Get(path string) *ListedComponent {
	for _, c := range l {
		if c.Path == path {
			return c
		}
	}

	return nil
}

func discoveredToListed(components component.Components, opts *Options) (ListedComponents, error) {
	listedComponents := make(ListedComponents, 0, len(components))
	errs := []error{}

	for _, c := range components {
		if c.External() && !opts.External {
			continue
		}

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

		var (
			relPath string
			err     error
		)

		if c.DiscoveryContext() != nil && c.DiscoveryContext().WorkingDir != "" {
			relPath, err = filepath.Rel(c.DiscoveryContext().WorkingDir, c.Path())
		} else {
			relPath, err = filepath.Rel(opts.WorkingDir, c.Path())
		}

		if err != nil {
			errs = append(errs, errors.New(err))

			continue
		}

		listedCfg := &ListedComponent{
			Type:     c.Kind(),
			Path:     relPath,
			Excluded: excluded,
		}

		if len(c.Dependencies()) == 0 {
			listedComponents = append(listedComponents, listedCfg)

			continue
		}

		listedCfg.Dependencies = make([]*ListedComponent, len(c.Dependencies()))

		for i, dep := range c.Dependencies() {
			var relDepPath string

			if dep.DiscoveryContext() != nil && dep.DiscoveryContext().WorkingDir != "" {
				relDepPath, err = filepath.Rel(dep.DiscoveryContext().WorkingDir, dep.Path())
			} else {
				relDepPath, err = filepath.Rel(opts.WorkingDir, dep.Path())
			}

			if err != nil {
				errs = append(errs, errors.New(err))

				continue
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

			listedCfg.Dependencies[i] = &ListedComponent{
				Type:     dep.Kind(),
				Path:     relDepPath,
				Excluded: depExcluded,
			}
		}

		sort.SliceStable(listedCfg.Dependencies, func(i, j int) bool {
			return listedCfg.Dependencies[i].Path < listedCfg.Dependencies[j].Path
		})

		listedComponents = append(listedComponents, listedCfg)
	}

	return listedComponents, errors.Join(errs...)
}

// Colorizer is a colorizer for the discovered components.
type Colorizer struct {
	unitColorizer    func(string) string
	stackColorizer   func(string) string
	headingColorizer func(string) string
	pathColorizer    func(string) string
}

// NewColorizer creates a new Colorizer.
func NewColorizer(shouldColor bool) *Colorizer {
	if !shouldColor {
		return &Colorizer{
			unitColorizer:    func(s string) string { return s },
			stackColorizer:   func(s string) string { return s },
			headingColorizer: func(s string) string { return s },
			pathColorizer:    func(s string) string { return s },
		}
	}

	return &Colorizer{
		unitColorizer:    ansi.ColorFunc("blue+bh"),
		stackColorizer:   ansi.ColorFunc("green+bh"),
		headingColorizer: ansi.ColorFunc("yellow+bh"),
		pathColorizer:    ansi.ColorFunc("white+d"),
	}
}

func (c *Colorizer) Colorize(listedComponent *ListedComponent) string {
	path := listedComponent.Path

	// Get the directory and base name using filepath
	dir, base := filepath.Split(path)

	if dir == "" {
		// No directory part, color the whole path
		switch listedComponent.Type {
		case component.UnitKind:
			return c.unitColorizer(path)
		case component.StackKind:
			return c.stackColorizer(path)
		default:
			return path
		}
	}

	// Color the components differently
	coloredPath := c.pathColorizer(dir)

	switch listedComponent.Type {
	case component.UnitKind:
		return coloredPath + c.unitColorizer(base)
	case component.StackKind:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

func (c *Colorizer) ColorizeType(t component.Kind) string {
	switch t {
	case component.UnitKind:
		// This extra space is to keep unit and stack
		// output equally spaced.
		return c.unitColorizer("unit ")
	case component.StackKind:
		return c.stackColorizer("stack")
	default:
		return string(t)
	}
}

func (c *Colorizer) ColorizeHeading(dep string) string {
	return c.headingColorizer(dep)
}

// outputText outputs the discovered components in text format.
func outputText(l log.Logger, opts *Options, components ListedComponents) error {
	colorizer := NewColorizer(shouldColor(l))

	return renderTabular(opts, components, colorizer)
}

// outputLong outputs the discovered components in long format.
func outputLong(l log.Logger, opts *Options, components ListedComponents) error {
	colorizer := NewColorizer(shouldColor(l))

	return renderLong(opts, components, colorizer)
}

// shouldColor returns true if the output should be colored.
func shouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !stdout.IsRedirected()
}

// renderLong renders the components in a long format.
func renderLong(opts *Options, components ListedComponents, c *Colorizer) error {
	var buf strings.Builder

	longestPathLen := getLongestPathLen(components)

	buf.WriteString(buildLongHeadings(opts, c, longestPathLen))

	for _, component := range components {
		buf.WriteString(c.ColorizeType(component.Type))
		buf.WriteString(" " + c.Colorize(component))

		if opts.Dependencies && len(component.Dependencies) > 0 {
			colorizedDeps := []string{}

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

	_, err := opts.Writer.Write([]byte(buf.String()))

	return errors.New(err)
}

// buildLongHeadings renders the headings for the long format.
func buildLongHeadings(opts *Options, c *Colorizer, longestPathLen int) string {
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
func renderTabular(opts *Options, components ListedComponents, c *Colorizer) error {
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

	_, err := opts.Writer.Write([]byte(buf.String()))

	return errors.New(err)
}

// outputTree outputs the discovered components in tree format.
func outputTree(l log.Logger, opts *Options, components ListedComponents, sort string) error {
	s := NewTreeStyler(shouldColor(l))

	return renderTree(opts, components, s, sort)
}

// outputDot outputs the discovered components in GraphViz DOT format.
func outputDot(_ log.Logger, opts *Options, components ListedComponents) error {
	return renderDot(opts, components)
}

type TreeStyler struct {
	entryStyle  lipgloss.Style
	rootStyle   lipgloss.Style
	colorizer   *Colorizer
	shouldColor bool
}

func NewTreeStyler(shouldColor bool) *TreeStyler {
	colorizer := NewColorizer(shouldColor)

	return &TreeStyler{
		shouldColor: shouldColor,
		entryStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MarginRight(1),
		rootStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("35")),
		colorizer:   colorizer,
	}
}

func (s *TreeStyler) Style(t *tree.Tree) *tree.Tree {
	t = t.
		Enumerator(tree.RoundedEnumerator)

	if !s.shouldColor {
		return t
	}

	return t.
		EnumeratorStyle(s.entryStyle).
		RootStyle(s.rootStyle)
}

// generateTree creates a tree structure from ListedComponents
func generateTree(components ListedComponents, s *TreeStyler) *tree.Tree {
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

				tmpCfg := &ListedComponent{
					Type: componentType,
					Path: segment,
				}

				newNode := tree.New().Root(s.colorizer.Colorize(tmpCfg))
				nodes[nextPath] = newNode
				currentNode.Child(newNode)
			}

			currentNode = nodes[nextPath]
			currentPath = nextPath
		}
	}

	return root
}

// generateDAGTree creates a tree structure from ListedComponents.
// It assumes that the components are already sorted in DAG order.
// As such, it will first construct root nodes for each component
// without a dependency in the listed components. Then, it will
// connect the remaining nodes to their dependencies, which
// should be doable in a single pass through the components.
// There may be duplicate entries for dependency nodes, as
// a node may be a dependency for multiple components.
// That's OK.
func generateDAGTree(components ListedComponents, s *TreeStyler) *tree.Tree {
	root := tree.Root(".")

	rootNodes := make(map[string]*tree.Tree)
	dependencyNodes := make(map[string]*tree.Tree)

	// First pass: create all root nodes
	for _, c := range components {
		if len(c.Dependencies) == 0 || !components.Contains(c.Path) {
			rootNodes[c.Path] = tree.New().Root(s.colorizer.Colorize(c))
		}
	}

	// Second pass: connect dependencies
	for _, c := range components {
		if len(c.Dependencies) == 0 {
			continue
		}

		// Sort dependencies to ensure deterministic order
		sortedDeps := make([]string, len(c.Dependencies))
		for i, dep := range c.Dependencies {
			sortedDeps[i] = dep.Path
		}

		sort.Strings(sortedDeps)

		for _, dependency := range sortedDeps {
			if _, exists := rootNodes[dependency]; exists {
				dependencyNode := tree.New().Root(s.colorizer.Colorize(c))
				rootNodes[dependency].Child(dependencyNode)
				dependencyNodes[c.Path] = dependencyNode

				continue
			}

			if _, exists := dependencyNodes[dependency]; exists {
				newDependencyNode := tree.New().Root(s.colorizer.Colorize(c))
				dependencyNodes[dependency].Child(newDependencyNode)
				dependencyNodes[c.Path] = newDependencyNode
			}
		}
	}

	// Sort root nodes to ensure deterministic order
	sortedRootPaths := make([]string, 0, len(rootNodes))
	for path := range rootNodes {
		sortedRootPaths = append(sortedRootPaths, path)
	}

	sort.Strings(sortedRootPaths)

	// Add root nodes in sorted order
	for _, path := range sortedRootPaths {
		root.Child(rootNodes[path])
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
func renderTree(opts *Options, components ListedComponents, s *TreeStyler, _ string) error {
	var t *tree.Tree

	if opts.Mode == ModeDAG {
		t = generateDAGTree(components, s)
	} else {
		t = generateTree(components, s)
	}

	t = s.Style(t)

	_, err := opts.Writer.Write([]byte(t.String() + "\n"))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// getMaxCols returns the maximum number of columns
// that can be displayed in the terminal.
// It also returns the width of each column.
// The width is the longest path length + 2 for padding.
func getMaxCols(components ListedComponents) (int, int) {
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

	w, _, err := term.GetSize(os.Stdout.Fd())
	if err == nil {
		width = w
	}

	return width
}

// getLongestPathLen returns the length of the
// longest path in the list of components.
func getLongestPathLen(components ListedComponents) int {
	longest := 0

	for _, c := range components {
		if len(c.Path) > longest {
			longest = len(c.Path)
		}
	}

	return longest
}

// renderDot renders the components in GraphViz DOT format.
func renderDot(opts *Options, components ListedComponents) error {
	var buf strings.Builder

	buf.WriteString("digraph {\n")

	sortedComponents := make(ListedComponents, len(components))
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

		buf.WriteString(fmt.Sprintf("\t\"%s\" %s;\n", component.Path, style))

		for _, dep := range component.Dependencies {
			buf.WriteString(fmt.Sprintf("\t\"%s\" -> \"%s\";\n", component.Path, dep.Path))
		}
	}

	buf.WriteString("}\n")

	_, err := opts.Writer.Write([]byte(buf.String()))

	return errors.New(err)
}
