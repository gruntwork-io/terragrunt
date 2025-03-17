package list

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/charmbracelet/x/term"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/mgutz/ansi"
)

// Run runs the list command.
func Run(ctx context.Context, opts *Options) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	if opts.Hidden {
		d = d.WithHidden()
	}

	if opts.Dependencies || opts.External || opts.Sort == SortDAG || opts.Long {
		d = d.WithDiscoverDependencies()
	}

	if opts.External {
		d = d.WithDiscoverExternalDependencies()
	}

	cfgs, err := d.Discover(ctx, opts.TerragruntOptions)
	if err != nil {
		return errors.New(err)
	}

	switch opts.Sort {
	case SortAlpha:
		cfgs = cfgs.Sort()
	case SortDAG:
		q, err := queue.NewQueue(cfgs)
		if err != nil {
			return errors.New(err)
		}

		cfgs = q.Entries()
	}

	listedCfgs, err := discoveredToListed(cfgs, opts)
	if err != nil {
		return errors.New(err)
	}

	switch opts.Format {
	case FormatText:
		return outputText(opts, listedCfgs)
	case FormatJSON:
		return outputJSON(opts, listedCfgs)
	case FormatTree:
		return outputTree(opts, listedCfgs, opts.Sort)
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid format: " + opts.Format)
	}
}

type ListedConfigs []*ListedConfig

type ListedConfig struct {
	Type discovery.ConfigType `json:"type"`
	Path string               `json:"path"`

	Dependencies []string `json:"dependencies,omitempty"`
}

// Contains checks to see if the given path is in the listed configs.
func (l ListedConfigs) Contains(path string) bool {
	for _, config := range l {
		if config.Path == path {
			return true
		}
	}

	return false
}

// Get returns the config with the given path.
func (l ListedConfigs) Get(path string) *ListedConfig {
	for _, config := range l {
		if config.Path == path {
			return config
		}
	}

	return nil
}

func discoveredToListed(configs discovery.DiscoveredConfigs, opts *Options) (ListedConfigs, error) {
	listedCfgs := make(ListedConfigs, 0, len(configs))
	errs := []error{}

	for _, config := range configs {
		if config.External && !opts.External {
			continue
		}

		relPath, err := filepath.Rel(opts.WorkingDir, config.Path)
		if err != nil {
			errs = append(errs, errors.New(err))

			continue
		}

		listedCfg := &ListedConfig{
			Type: config.Type,
			Path: relPath,
		}

		if len(config.Dependencies) == 0 {
			listedCfgs = append(listedCfgs, listedCfg)

			continue
		}

		listedCfg.Dependencies = make([]string, len(config.Dependencies))

		for i, dep := range config.Dependencies {
			relDepPath, err := filepath.Rel(opts.WorkingDir, dep.Path)
			if err != nil {
				errs = append(errs, errors.New(err))

				continue
			}

			listedCfg.Dependencies[i] = relDepPath
		}

		listedCfgs = append(listedCfgs, listedCfg)
	}

	return listedCfgs, errors.Join(errs...)
}

// outputJSON outputs the discovered configurations in JSON format.
func outputJSON(opts *Options, configs ListedConfigs) error {
	jsonBytes, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return errors.New(err)
	}

	_, err = opts.Writer.Write(append(jsonBytes, []byte("\n")...))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// Colorizer is a colorizer for the discovered configurations.
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

func (c *Colorizer) Colorize(config *ListedConfig) string {
	path := config.Path

	// Get the directory and base name using filepath
	dir, base := filepath.Split(path)

	if dir == "" {
		// No directory part, color the whole path
		switch config.Type {
		case discovery.ConfigTypeUnit:
			return c.unitColorizer(path)
		case discovery.ConfigTypeStack:
			return c.stackColorizer(path)
		default:
			return path
		}
	}

	// Color the components differently
	coloredPath := c.pathColorizer(dir)

	switch config.Type {
	case discovery.ConfigTypeUnit:
		return coloredPath + c.unitColorizer(base)
	case discovery.ConfigTypeStack:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

func (c *Colorizer) ColorizeType(t discovery.ConfigType) string {
	switch t {
	case discovery.ConfigTypeUnit:
		// This extra space is to keep unit and stack
		// output equally spaced.
		return c.unitColorizer("unit ")
	case discovery.ConfigTypeStack:
		return c.stackColorizer("stack")
	default:
		return string(t)
	}
}

func (c *Colorizer) ColorizeHeading(dep string) string {
	return c.headingColorizer(dep)
}

// outputText outputs the discovered configurations in text format.
func outputText(opts *Options, configs ListedConfigs) error {
	colorizer := NewColorizer(shouldColor(opts))

	if opts.Long {
		return renderLong(opts, configs, colorizer)
	}

	return renderTabular(opts, configs, colorizer)
}

// shouldColor returns true if the output should be colored.
func shouldColor(opts *Options) bool {
	return !(opts.TerragruntOptions.Logger.Formatter().DisabledColors() || isStdoutRedirected())
}

// renderLong renders the configurations in a long format.
func renderLong(opts *Options, configs ListedConfigs, c *Colorizer) error {
	longestPathLen := getLongestPathLen(configs)

	err := renderLongHeadings(opts, c, longestPathLen)
	if err != nil {
		return errors.New(err)
	}

	for _, config := range configs {
		_, err := opts.Writer.Write([]byte(c.ColorizeType(config.Type)))
		if err != nil {
			return errors.New(err)
		}

		_, err = opts.Writer.Write([]byte(" " + c.Colorize(config)))
		if err != nil {
			return errors.New(err)
		}

		if len(config.Dependencies) > 0 {
			colorizedDeps := []string{}

			for _, dep := range config.Dependencies {
				depCfg := configs.Get(dep)
				if depCfg != nil {
					colorizedDeps = append(colorizedDeps, c.Colorize(depCfg))
				}
			}

			const extraDependenciesPadding = 2

			dependenciesPadding := (longestPathLen - len(config.Path)) + extraDependenciesPadding
			for range dependenciesPadding {
				_, err := opts.Writer.Write([]byte(" "))
				if err != nil {
					return errors.New(err)
				}
			}

			_, err = opts.Writer.Write([]byte(strings.Join(colorizedDeps, ", ")))
			if err != nil {
				return errors.New(err)
			}
		}

		_, err = opts.Writer.Write([]byte("\n"))
		if err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// renderLongHeadings renders the headings for the long format.
func renderLongHeadings(opts *Options, c *Colorizer, longestPathLen int) error {
	_, err := opts.Writer.Write([]byte(c.ColorizeHeading("Type  Path")))
	if err != nil {
		return errors.New(err)
	}

	const extraDependenciesPadding = 2

	dependenciesPadding := (longestPathLen - len("Path")) + extraDependenciesPadding
	for range dependenciesPadding {
		_, err := opts.Writer.Write([]byte(" "))
		if err != nil {
			return errors.New(err)
		}
	}

	_, err = opts.Writer.Write([]byte(c.ColorizeHeading("Dependencies\n")))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// renderTabular renders the configurations in a tabular format.
func renderTabular(opts *Options, configs ListedConfigs, c *Colorizer) error {
	maxCols, colWidth := getMaxCols(configs)

	for i, config := range configs {
		if i > 0 && i%maxCols == 0 {
			_, err := opts.Writer.Write([]byte("\n"))
			if err != nil {
				return errors.New(err)
			}
		}

		_, err := opts.Writer.Write([]byte(c.Colorize(config)))
		if err != nil {
			return errors.New(err)
		}

		// Add padding until the length of maxCols
		padding := colWidth - len(config.Path)
		for range padding {
			_, err := opts.Writer.Write([]byte(" "))
			if err != nil {
				return errors.New(err)
			}
		}
	}

	_, err := opts.Writer.Write([]byte("\n"))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// outputTree outputs the discovered configurations in tree format.
func outputTree(opts *Options, configs ListedConfigs, sort string) error {
	s := NewTreeStyler(shouldColor(opts))

	return renderTree(opts, configs, s, sort)
}

type TreeStyler struct {
	shouldColor bool
	entryStyle  lipgloss.Style
	rootStyle   lipgloss.Style
	itemStyle   lipgloss.Style
}

func NewTreeStyler(shouldColor bool) *TreeStyler {
	return &TreeStyler{
		shouldColor: shouldColor,
		entryStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("63")).MarginRight(1),
		rootStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("35")),
		itemStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
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
		RootStyle(s.rootStyle).
		ItemStyle(s.itemStyle)
}

// generateTree creates a tree structure from ListedConfigs
func generateTree(configs ListedConfigs) *tree.Tree {
	root := tree.Root(".")
	nodes := make(map[string]*tree.Tree)

	for _, config := range configs {
		parts := preProcessPath(config.Path)
		if len(parts.segments) == 0 || (len(parts.segments) == 1 && parts.segments[0] == ".") {
			continue
		}

		currentPath := "."
		currentNode := root

		for _, segment := range parts.segments {
			nextPath := filepath.Join(currentPath, segment)
			if _, exists := nodes[nextPath]; !exists {
				newNode := tree.New().Root(segment)
				nodes[nextPath] = newNode
				currentNode.Child(newNode)
			}

			currentNode = nodes[nextPath]
			currentPath = nextPath
		}
	}

	return root
}

// generateDAGTree creates a tree structure from ListedConfigs.
// It assumes that the configs are already sorted in DAG order.
// As such, it will first construct root nodes for each config
// without a dependency in the listed configs. Then, it will
// connect the remaining nodes to their dependencies, which
// should be doable in a single pass through the configs.
// There may be duplicate entries for dependency nodes, as
// a node may be a dependency for multiple configs.
// That's OK.
func generateDAGTree(configs ListedConfigs) *tree.Tree {
	root := tree.Root(".")

	rootNodes := make(map[string]*tree.Tree)
	dependencyNodes := make(map[string]*tree.Tree)

	for _, config := range configs {
		// We can assume that the configs are already sorted in DAG order,
		// so we can assume that it's fine to start working on dependencies
		// immediately after adding the root nodes.
		if len(config.Dependencies) == 0 || !configs.Contains(config.Path) {
			rootNodes[config.Path] = tree.New().Root(config.Path)

			continue
		}

		// If the config has dependencies, we need to add them to the correct tree.
		// So that we don't lose track of sub-dependencies, we also
		// need to add non-root dependency nodes to the dependencyNodes map.
		for _, dependency := range config.Dependencies {
			if _, exists := rootNodes[dependency]; exists {
				dependencyNode := tree.New().Root(config.Path)
				rootNodes[dependency].Child(dependencyNode)
				dependencyNodes[config.Path] = dependencyNode

				continue
			}

			// Theoretically, this should always be true, because we've already
			// added the dependency when we added the root node.
			if _, exists := dependencyNodes[dependency]; exists {
				newDependencyNode := tree.New().Root(config.Path)
				dependencyNodes[dependency].Child(newDependencyNode)
				dependencyNodes[config.Path] = newDependencyNode

				continue
			}
		}
	}

	for _, node := range rootNodes {
		root.Child(node)
	}

	return root
}

// pathParts holds the pre-processed parts of a config path.
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

// renderTree renders the configurations in a tree format.
func renderTree(opts *Options, configs ListedConfigs, s *TreeStyler, sort string) error {
	var t *tree.Tree

	if sort == SortDAG {
		t = generateDAGTree(configs)
	} else {
		t = generateTree(configs)
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
func getMaxCols(configs ListedConfigs) (int, int) {
	maxCols := 0

	terminalWidth := getTerminalWidth()
	longestPathLen := getLongestPathLen(configs)

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
// longest path in the list of configurations.
func getLongestPathLen(configs ListedConfigs) int {
	longest := 0

	for _, config := range configs {
		if len(config.Path) > longest {
			longest = len(config.Path)
		}
	}

	return longest
}

// isStdoutRedirected returns true if the stdout is redirected.
func isStdoutRedirected() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}
