package discovery

import (
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// applyPreventDestroyExclusions excludes units with prevent_destroy=true and their dependencies
// from being destroyed. This prevents accidental destruction of protected infrastructure.
func applyPreventDestroyExclusions(l log.Logger, units component.Units) {
	// First pass: identify units with prevent_destroy=true
	protectedUnits := make(map[string]bool)

	for _, unit := range units {
		if unit.Config().PreventDestroy != nil && *unit.Config().PreventDestroy {
			protectedUnits[unit.Path()] = true
			unit.SetFlagExcluded(true)
			l.Debugf("Unit %s is protected by prevent_destroy flag", unit.Path())
		}
	}

	if len(protectedUnits) == 0 {
		return
	}

	// Second pass: find all dependencies of protected units
	// We need to prevent destruction of any unit that a protected unit depends on
	dependencyPaths := make(map[string]bool)

	for _, unit := range units {
		if protectedUnits[unit.Path()] {
			collectDependencies(unit, dependencyPaths)
		}
	}

	// Third pass: mark dependencies as excluded
	for _, unit := range units {
		if dependencyPaths[unit.Path()] && !protectedUnits[unit.Path()] {
			unit.SetFlagExcluded(true)
			l.Debugf("Unit %s is excluded because it's a dependency of a protected unit", unit.Path())
		}
	}
}

// maxDependencyTraversalDepth bounds the depth of dependency traversal to prevent excessive recursion.
const maxDependencyTraversalDepth = 256

// collectDependencies collects dependency paths for a unit with a bounded recursion depth.
func collectDependencies(unit *component.Unit, paths map[string]bool) {
	collectDependenciesBounded(unit, paths, 0)
}

// collectDependenciesBounded recursively collects all dependency paths for a unit up to maxDependencyTraversalDepth.
func collectDependenciesBounded(unit *component.Unit, paths map[string]bool, depth int) {
	if depth >= maxDependencyTraversalDepth {
		return
	}

	for _, dep := range unit.Dependencies() {
		depUnit, ok := dep.(*component.Unit)
		if !ok {
			continue
		}

		if !paths[depUnit.Path()] {
			paths[depUnit.Path()] = true
			collectDependenciesBounded(depUnit, paths, depth+1)
		}
	}
}

// isDestroyCommand checks if the terraform command is a destroy command.
func isDestroyCommand(opts *options.TerragruntOptions) bool {
	return opts.TerraformCommand == tf.CommandNameDestroy ||
		util.ListContainsElement(opts.TerraformCliArgs, "-"+tf.CommandNameDestroy)
}
