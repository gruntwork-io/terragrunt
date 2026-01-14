package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/zclconf/go-cty/cty"
)

// Resolver implements runcfg.DependencyResolver.
// It wraps the config package's dependency resolution logic.
type Resolver struct{}

// NewResolver creates a new dependency resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// ResolveOutputs fetches terraform outputs from the given config path.
// This implements runcfg.DependencyResolver.
func (r *Resolver) ResolveOutputs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, targetConfigPath string) (map[string]cty.Value, error) {
	// Get the JSON output using the existing caching mechanism
	jsonBytes, err := getOutputJSONWithCaching(ctx, &ParsingContext{TerragruntOptions: opts}, l, targetConfigPath)
	if err != nil {
		return nil, err
	}

	// Convert JSON to cty value map
	return TerraformOutputJSONToCtyValueMap(targetConfigPath, jsonBytes)
}

// Verify interface compliance
var _ runcfg.DependencyResolver = (*Resolver)(nil)
