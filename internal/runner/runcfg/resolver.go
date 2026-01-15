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

// DependentUnit represents a unit that depends on a given unit.
type DependentUnit interface {
	Path() string
}

// Report is a minimal interface for execution reporting.
// This breaks the import cycle between runcfg and the report package.
type Report any
