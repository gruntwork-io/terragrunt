package component

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

const (
	UnitKind Kind = "unit"
)

// Unit represents a discovered Terragrunt unit configuration.
type Unit struct {
	cfg        *config.TerragruntConfig
	configFile string
	baseComponent
	excluded bool
}

// NewUnit creates a new Unit component with the given path.
func NewUnit(path string) *Unit {
	return &Unit{
		baseComponent: newBaseComponent(path),
		configFile:    config.DefaultTerragruntConfigPath,
	}
}

// WithReading appends a file to the list of files being read by this component.
// Useful for constructing components with all files read at once.
func (u *Unit) WithReading(files ...string) *Unit {
	u.SetReading(files...)

	return u
}

// WithConfig adds configuration to a Unit component.
func (u *Unit) WithConfig(cfg *config.TerragruntConfig) *Unit {
	u.cfg = cfg

	return u
}

// WithDiscoveryContext sets the discovery context for this unit.
func (u *Unit) WithDiscoveryContext(ctx *DiscoveryContext) *Unit {
	u.discoveryContext = ctx

	return u
}

// Config returns the parsed Terragrunt configuration for this unit.
func (u *Unit) Config() *config.TerragruntConfig {
	return u.cfg
}

// StoreConfig stores the parsed Terragrunt configuration for this unit.
func (u *Unit) StoreConfig(cfg *config.TerragruntConfig) {
	u.cfg = cfg
}

// ConfigFile returns the discovered config filename for this unit.
func (u *Unit) ConfigFile() string {
	return u.configFile
}

// SetConfigFile sets the discovered config filename for this unit.
func (u *Unit) SetConfigFile(filename string) {
	u.configFile = filename
}

// Kind returns the kind of component (always Unit for Unit).
func (u *Unit) Kind() Kind {
	return UnitKind
}

// Excluded returns whether the unit was excluded during discovery/filtering.
func (u *Unit) Excluded() bool {
	return u.excluded
}

// SetExcluded marks the unit as excluded during discovery/filtering.
func (u *Unit) SetExcluded(excluded bool) {
	u.excluded = excluded
}

// Sources returns the list of sources for this component.
func (u *Unit) Sources() []string {
	if u.cfg == nil || u.cfg.Terraform == nil || u.cfg.Terraform.Source == nil {
		return []string{}
	}

	return []string{*u.cfg.Terraform.Source}
}

// AddDependency adds a dependency to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependency(dependency Component) {
	u.baseComponent.addDependency(u, dependency)
}

// AddDependent adds a dependent to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependent(dependent Component) {
	u.baseComponent.addDependent(u, dependent)
}

// String renders this unit as a human-readable string for debugging.
//
// Example output:
//
//	Unit /path/to/unit (excluded: false, dependencies: [/dep1, /dep2])
func (u *Unit) String() string {
	u.mu.RLock()
	defer u.mu.RUnlock()

	path := u.DisplayPath()
	deps := make([]string, 0, len(u.dependencies))

	for _, dep := range u.dependencies {
		deps = append(deps, dep.DisplayPath())
	}

	return fmt.Sprintf(
		"Unit %s (excluded: %v, dependencies: [%s])",
		path, u.excluded, strings.Join(deps, ", "),
	)
}

// PlanFile returns plan file location if output folder is set.
func (u *Unit) PlanFile(rootWorkingDir, outputFolder, jsonOutputFolder, tofuCommand string) string {
	planFile := u.OutputFile(rootWorkingDir, outputFolder)

	planCommand := tofuCommand == tf.CommandNamePlan ||
		tofuCommand == tf.CommandNameShow

	// if JSON output enabled and no PlanFile specified, save plan in working dir
	if planCommand && planFile == "" && jsonOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// OutputFile returns plan file location if output folder is set.
func (u *Unit) OutputFile(rootWorkingDir, outputFolder string) string {
	return u.planFilePath(rootWorkingDir, outputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile returns plan JSON file location if JSON output folder is set.
func (u *Unit) OutputJSONFile(rootWorkingDir, jsonOutputFolder string) string {
	return u.planFilePath(rootWorkingDir, jsonOutputFolder, tf.TerraformPlanJSONFile)
}

// planFilePath computes the path for plan output files.
func (u *Unit) planFilePath(rootWorkingDir, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	// Use discoveryContext.WorkingDir as base (always populated).
	// This is critical for git-based filters where units are discovered in temporary worktrees.
	// Using rootWorkingDir would cause relative paths to escape the outputFolder.
	relPath, err := filepath.Rel(u.discoveryContext.WorkingDir, u.path)
	if err != nil {
		relPath = u.path
	}

	dir := filepath.Join(outputFolder, relPath)

	if !filepath.IsAbs(dir) {
		dir = filepath.Join(rootWorkingDir, dir)
	}

	dir = filepath.Clean(dir)

	return filepath.Join(dir, fileName)
}
