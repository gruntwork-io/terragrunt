package generate

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
)

func (c *Coordinator) discoverDependencyStacks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	roots []string,
) (bool, error) {
	if _, generating := ctx.Value(generationOwnerKey{}).(generationOwner); generating {
		return false, nil
	}

	references, err := c.discoverStackReferences(ctx, l, v, roots)
	if err != nil {
		return false, err
	}

	before := c.generationCount()
	for _, path := range references.stacks {
		if err := ctx.Err(); err != nil {
			return false, err
		}

		if c.stackRequested(path) {
			continue
		}

		if err := config.GenerateStackDependency(ctx, l, v, path); err != nil {
			return false, err
		}

		if err := ctx.Err(); err != nil {
			return false, err
		}

		// Generation can replace the references just collected; discover them again before proceeding.
		if c.generationCount() != before {
			return true, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}

	for _, err := range references.parseErrors {
		logDeferredDiagnostics(l, err)
	}

	return false, errors.Join(references.parseErrors...)
}

func (c *Coordinator) generationCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.jobs)
}

func (c *Coordinator) stackRequested(dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, requested := c.jobs[filepath.Join(dir, config.DefaultStackFile)]
	return requested
}

type stackReferences struct {
	stacks      []string
	parseErrors []error
}

func (c *Coordinator) discoverStackReferences(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	roots []string,
) (*stackReferences, error) {
	pending := slices.Clone(roots)
	seen := make(map[string]bool)
	references := &stackReferences{}
	var units []string

	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		path, err := util.CanonicalResolvedPath(v.FS, pending[0], "")
		pending = pending[1:]
		if err != nil {
			return nil, err
		}

		if seen[path] {
			continue
		}

		seen[path] = true

		if vfs.Exists(v.FS, filepath.Join(path, config.DefaultStackFile)) {
			references.stacks = append(references.stacks, path)
			if !c.stackRequested(path) {
				continue
			}

			generated, err := inthclparse.UnitPathsFromStackDir(ctx, v.FS, path, &inthclparse.StackDirArgs{
				FuncsFor: configbridge.StackFuncFactory(ctx, l, v, c.opts),
			})
			if err != nil {
				return nil, err
			}

			pending = append(pending, generated...)

			continue
		}

		if vfs.IsDir(v.FS, path) {
			path = filepath.Join(path, config.DefaultTerragruntConfigPath)
		}

		if !vfs.Exists(v.FS, path) {
			continue
		}

		units = append(units, path)
		dependencies, err := discovery.UnitDependencyPaths(ctx, l, v, c.opts, path, config.DiagnosticsSuppressed)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			var diagnostics hcl.Diagnostics
			if !errors.As(err, &diagnostics) {
				return nil, err
			}

			references.parseErrors = append(references.parseErrors, err)
			continue
		}

		pending = append(pending, dependencies...)
	}

	// Explicitly referenced ancestors may replace stale unit references. Generate them first.
	rewritesUnits := make(map[string]bool, len(references.stacks))
	for _, stack := range references.stacks {
		for _, unit := range units {
			if vfs.Within(v.FS, stack, unit) {
				rewritesUnits[stack] = true
				break
			}
		}
	}

	slices.SortFunc(references.stacks, func(a, b string) int {
		if rewritesUnits[a] != rewritesUnits[b] {
			if rewritesUnits[a] {
				return -1
			}

			return 1
		}

		return strings.Compare(a, b)
	})

	return references, nil
}

func logDeferredDiagnostics(l log.Logger, err error) {
	switch err := err.(type) {
	case hcl.Diagnostics:
		for _, diagnostic := range err {
			l.Errorf("%s", diagnostic.Error())
		}
	case interface{ Unwrap() []error }:
		for _, cause := range err.Unwrap() {
			logDeferredDiagnostics(l, cause)
		}
	case interface{ Unwrap() error }:
		logDeferredDiagnostics(l, err.Unwrap())
	}
}
