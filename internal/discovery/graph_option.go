package discovery

import "github.com/gruntwork-io/terragrunt/internal/runner/common"

type graphTargetOption struct {
	target string
}

// WithGraphTarget returns an option that, when applied to the runner stack,
// marks a graph target that discovery will use to prune the run to the target
// path and its dependents. Apply is a no-op; discovery picks this up via
// Discovery.WithOptions by asserting for GraphTarget() on options.
func WithGraphTarget(targetDir string) common.Option {
	return graphTargetOption{target: targetDir}
}

// Apply is a no-op; discovery consumes the marker via WithOptions.
func (o graphTargetOption) Apply(stack common.StackRunner) {}

// GraphTarget exposes the requested graph target for discovery to consume.
func (o graphTargetOption) GraphTarget() string {
	return o.target
}
