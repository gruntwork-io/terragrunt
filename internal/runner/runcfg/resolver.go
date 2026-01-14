package runcfg

import (
	"context"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/zclconf/go-cty/cty"
)

// DependencyResolver resolves outputs from dependent terragrunt configurations.
// This interface is implemented by the config package to break cyclic imports.
type DependencyResolver interface {
	// ResolveOutputs fetches terraform outputs from the given config path.
	// It returns a map of output names to their cty values.
	ResolveOutputs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, targetConfigPath string) (map[string]cty.Value, error)
}

// ConfigReader reads and parses terragrunt configuration files.
// This interface is implemented by the config package to break cyclic imports.
type ConfigReader interface {
	// ReadConfig reads and parses a terragrunt config file, returning a RunConfig.
	ReadConfig(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (*RunConfig, error)
}

// DependentModule represents a module that depends on the current working directory.
type DependentModule interface {
	Path() string
}

// DependentUnitsFinder finds modules that depend on a given working directory.
// This interface is implemented by the runner package to break cyclic imports.
type DependentUnitsFinder interface {
	// FindDependentModules returns a list of modules that have the given working directory as a dependency.
	FindDependentModules(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *RunConfig) []DependentModule
}

// Report is a minimal interface for execution reporting.
// This breaks the import cycle between runcfg and the report package.
type Report interface{}

// TerragruntRunner runs terragrunt commands.
// This interface is implemented by the run package to break cyclic imports
// between config and runner packages.
type TerragruntRunner interface {
	// Run executes terragrunt with the given options.
	Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r Report) error
}
