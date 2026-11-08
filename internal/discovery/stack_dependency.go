package discovery

import "github.com/gruntwork-io/terragrunt/internal/vfs"

// StackDependencyAllowed checks whether target falls within the generation limits.
// opts.WorkingDir must be the original command directory so relative limits
// do not move when visiting a dependency.
func StackDependencyAllowed(fsys vfs.FS, opts StackGenerateOptions, target string) (bool, error) {
	dirs, limited := positiveBoundaryDirs(opts, false)
	if !limited {
		return true, nil
	}
	allowed := false
	for _, dir := range dirs {
		resolved, err := resolveDiscoveryBoundary(fsys, opts.WorkingDir, dir, boundaryEnclosureFor(opts.Filters))
		if err != nil {
			return false, err
		}
		allowed = allowed || vfs.Within(fsys, resolved, target)
	}
	return allowed, nil
}
