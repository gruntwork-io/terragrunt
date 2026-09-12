package mcp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// freshCallContext gives one tool call its own HCL, dependency-output,
// repo-root, and run_cmd caches. RunAction installs those once on the action
// context every call shares. Without this reset, a repo root the deny-all git
// stub approximated on one call would be reused by later calls, degrading
// them without the note that says so.
func freshCallContext(ctx context.Context) context.Context {
	return cache.ContextWithCache(config.WithConfigValues(ctx))
}

// resolveWorkingDir turns a tool-call working_dir argument into an absolute,
// symlink-resolved directory confined under the server launch dir. Empty
// input means the launch dir itself.
func resolveWorkingDir(fsys vfs.FS, launchDir, requested string) (string, error) {
	if requested == "" {
		return launchDir, nil
	}

	dir := requested
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(launchDir, dir)
	}

	dir, err := vfs.EvalSymlinks(fsys, dir)
	if err != nil {
		return "", fmt.Errorf("resolving working_dir %q: %w", requested, err)
	}

	// launchDir is already symlink-resolved, so the Rel comparison is
	// symlink-safe on both sides.
	rel, err := filepath.Rel(launchDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("working_dir %q escapes the MCP server root %s", requested, launchDir)
	}

	info, err := fsys.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("working_dir %q: %w", requested, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("working_dir %q is not a directory", requested)
	}

	return dir, nil
}

// buildDirOptions returns fresh, tree-scoped TerragruntOptions for the tools
// that walk a directory. Hand the returned env map to [serverDeps.callVenv],
// so credentials land in the same map the parse reads.
func buildDirOptions(
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	workingDir string,
	filterQueries []string,
) (*options.TerragruntOptions, map[string]string, error) {
	opts := options.NewTerragruntOptions(rootVenv.Exec)
	applyOperatorTFPath(d, opts)

	// A filter that follows the graph otherwise walks out to the git repository
	// root, so a unit outside the tree the server was launched in can come back
	// in a result. The boundary keeps those out, which makes the server root
	// bound what a tool reports the way [resolveWorkingDir] already bounds what
	// a tool call may target.
	//
	// It bounds reporting rather than reading: a unit across the boundary still
	// has its configuration read, so units inside can be ordered against it and
	// fetch its outputs.
	opts.DiscoveryBoundary = d.launchDir
	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir
	opts.NonInteractive = true
	opts.Experiments = d.baseOpts.Experiments
	opts.StrictControls = d.baseOpts.StrictControls
	opts.TerragruntVersion = d.tgVersion

	if !d.allowExec {
		// Discovery's auth-provider probe is a subprocess; don't waste a
		// stub round-trip on it when exec is denied.
		opts.DiscoveryAuthProviderCmd = false
	}

	if len(filterQueries) > 0 {
		parsed, err := filter.ParseFilterQueries(l, filterQueries)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing filter queries: %w", err)
		}

		opts.Filters = parsed
	}

	return opts, maps.Clone(rootVenv.Env), nil
}

// buildUnitOptions returns fresh TerragruntOptions rooted at one unit's
// terragrunt.hcl.
func buildUnitOptions(
	d *serverDeps,
	rootVenv *venv.Venv,
	unitDir, command string,
	cliArgs ...string,
) (*options.TerragruntOptions, map[string]string, error) {
	configPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	exists, err := vfs.FileExists(rootVenv.FS, configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("checking %s: %w", configPath, err)
	}

	if !exists {
		return nil, nil, fmt.Errorf(
			"no %s found in %s",
			config.DefaultTerragruntConfigPath,
			unitDir,
		)
	}

	opts, err := options.NewTerragruntOptionsWithConfigPath(rootVenv.Exec, configPath)
	if err != nil {
		return nil, nil, err
	}

	applyOperatorTFPath(d, opts)
	opts.DiscoveryBoundary = d.launchDir
	opts.NonInteractive = true
	opts.Experiments = d.baseOpts.Experiments
	opts.StrictControls = d.baseOpts.StrictControls
	opts.TerragruntVersion = d.tgVersion
	opts.TerraformCommand = command
	opts.OriginalTerraformCommand = command
	opts.TerraformCliArgs = iacargs.New(append([]string{command}, cliArgs...)...)
	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath

	return opts, maps.Clone(rootVenv.Env), nil
}

// applyOperatorTFPath carries a --tf-path the operator set into options a tool
// call builds fresh, so the server runs the binary they chose.
//
// Only an explicit setting travels. A configuration can name a binary too,
// through terraform_binary, and that one wins whenever the operator left the
// flag alone. Treating it as the operator's choice would let a tree under the
// server root pick the program the exec allowlist then trusts by path.
func applyOperatorTFPath(d *serverDeps, opts *options.TerragruntOptions) {
	tfPath := d.operatorTFPath()
	if tfPath == "" {
		return
	}

	opts.TFPath = tfPath
	opts.TFPathExplicitlySet = true
}

// setupGitFilterWorktrees creates git worktrees for filter queries containing
// git range expressions and returns them with the func that removes them,
// which callers defer. Without git filters both are nil.
func setupGitFilterWorktrees(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) (*worktrees.Worktrees, func(), error) {
	gitFilters := opts.Filters.UniqueGitFilters()
	if len(gitFilters) == 0 {
		return nil, nil, nil
	}

	if !d.allowExec {
		// The deny-all git stub answers every invocation with the dir it was
		// called from, so letting this through would produce empty worktrees
		// and a filter result that silently matched nothing.
		return nil, nil, errors.New(
			"git-range filters require the MCP server to be started with --allow=exec",
		)
	}

	w, err := worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
		WorkingDir:     opts.WorkingDir,
		GitExpressions: gitFilters,
		Experiments:    opts.Experiments,
	})
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		if err := w.Cleanup(ctx, l, v); err != nil {
			l.Errorf("cleaning up git worktrees: %v", err)
		}
	}

	return w, cleanup, nil
}

// discoverDegraded runs discovery in its suppressed-parse mode, where
// components and an error come back together, and turns the error into a
// degraded note so a parse failure degrades the tool's result instead of
// failing the call. The components come back sorted.
func discoverDegraded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	disc *discovery.Discovery,
	tool string,
) (component.Components, []string) {
	components, err := disc.Discover(ctx, l, v, opts)

	var degraded []string

	if err != nil {
		l.Debugf("%s tool: suppressed discovery errors: %v", tool, err)

		degraded = append(
			degraded,
			fmt.Sprintf("discovery reported suppressed parse errors: %v", err),
		)
	}

	return components.Sort(), degraded
}
