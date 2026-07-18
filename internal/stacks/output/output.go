// Package output provides functionality for collecting and collating the
// unit outputs for all units in a stack hierarchy.
package output

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/zclconf/go-cty/cty"
)

// UnitOutputError is returned when reading terraform outputs for a stack unit fails.
type UnitOutputError struct {
	Err      error
	UnitName string
	UnitDir  string
}

func (e UnitOutputError) Error() string {
	return fmt.Sprintf("failed to read outputs for unit %s in %s: %v", e.UnitName, e.UnitDir, e.Err)
}

func (e UnitOutputError) Unwrap() error {
	return e.Err
}

// StackOutput collects and returns the OpenTofu/Terraform output values for all declared units in a stack hierarchy.
//
// This function is a central component of Terragrunt's stack output system, providing a mechanism to
// aggregate and organize outputs from multiple deployments in a hierarchical structure. It's particularly
// useful when working with complex infrastructure composed of multiple interconnected OpenTofu/Terraform units.
//
// The function performs several key operations:
//
//  1. Discovers all stack definition files (terragrunt.stack.hcl) in the working directory and its subdirectories.
//  2. For each stack file, parses the configuration and extracts the declared stacks and units.
//  3. For each unit, reads its OpenTofu/Terraform outputs from the corresponding directory within .terragrunt-stack.
//  4. Constructs a hierarchical map of outputs by organizing units according to their position in the stack hierarchy.
//     Units are keyed using dot notation that reflects the stack path (e.g., "parent.child.unit").
//  5. Orders stack names from the highest level (shortest path) to deepest nested (longest path).
//  6. Nests the flat output map into a hierarchical structure and converts it to a cty.Value object.
//
// The returned cty.Value object contains a structured representation of all outputs, preserving the
// nested relationship between stacks and units. This makes it easy to access outputs from specific
// parts of the infrastructure while maintaining awareness of the overall architecture.
//
// For telemetry and debugging purposes, the function logs various events at the debug level, including
// when outputs are added for specific units and stack keys.
//
// Parameters:
//   - ctx: Context for the operation, which may include telemetry collection.
//   - opts: TerragruntOptions containing configuration settings and the working directory path.
//
// Returns:
//   - cty.Value: A hierarchical object containing all outputs from the stack units, organized by stack path.
//   - error: An error if any operation fails during discovery, parsing, output collection, or conversion.
//
// Errors can occur during stack file listing, value reading, stack config parsing, output reading,
// or when converting the final output structure to cty.Value format.
func StackOutput(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	opts *options.TerragruntOptions,
) (cty.Value, error) {
	l.Debugf("Generating output from %s", opts.WorkingDir)

	// Create worktrees internally if filter-flag experiment is enabled and git filters are present
	wts, err := buildWorktreesIfNeeded(ctx, l, opts)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to create worktrees: %w", err)
	}

	if wts != nil {
		defer func() {
			if cleanupErr := wts.Cleanup(ctx, l); cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()
	}

	// Single discovery walk returns both stack files and excluded unit paths.
	foundFiles, excludedPaths, err := generate.ListStackFilesWithExcludes(ctx, l, v, opts, wts)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to list stack files in %s: %w", opts.WorkingDir, err)
	}

	implicitMerge := opts.Experiments.Evaluate(experiment.StackOutputImplicit)

	if len(foundFiles) == 0 && !implicitMerge {
		l.Warnf("No stack files found in %s Nothing to generate.", opts.WorkingDir)
		return cty.NilVal, nil
	}

	outputs := xsync.NewMap[string, map[string]cty.Value]()
	declaredStacks := make(map[string]string)
	declaredUnits := make(map[string]*config.Unit)
	parsedStackFiles := make(map[string]*config.StackConfig, len(foundFiles))

	maxWorkers := max(1, min(opts.Parallelism, runtime.GOMAXPROCS(0)))

	// reuse the project worker pool so error aggregation matches other concurrent commands
	wp := worker.NewWorkerPool(maxWorkers)
	defer wp.Stop()

	waitWorkerErrors := func(mainErr error) error {
		workerErr := wp.Wait()
		if workerErr == nil {
			return mainErr
		}

		if mainErr == nil {
			return workerErr
		}

		return errors.Join(mainErr, workerErr)
	}

	for _, path := range foundFiles {
		dir := filepath.Dir(path)

		ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)
		pctx = pctx.WithVenv(v)

		values, valuesErr := config.ReadValues(ctx, pctx, l, dir)
		if valuesErr != nil {
			return cty.NilVal, waitWorkerErrors(fmt.Errorf("failed to read values from %s: %w", dir, valuesErr))
		}

		stackFile, stackErr := config.ReadStackConfigFile(ctx, l, pctx, path, values)
		if stackErr != nil {
			return cty.NilVal, waitWorkerErrors(fmt.Errorf("failed to read stack file %s: %w", path, stackErr))
		}

		parsedStackFiles[path] = stackFile

		targetDir := filepath.Join(dir, config.StackDir)

		for _, stack := range stackFile.Stacks {
			declaredStacks[filepath.Join(targetDir, stack.Path)] = stack.Name
			l.Debugf("Registered stack %s at path %s", stack.Name, filepath.Join(targetDir, stack.Path))
		}

		for _, unit := range stackFile.Units {
			unitDir := unit.GeneratedPath(dir)

			// Excluded units are fully omitted from the final output, matching stack run behavior.
			if _, excluded := excludedPaths[filepath.Clean(unitDir)]; excluded {
				l.Debugf("Skipping output for excluded unit %s in %s", unit.Name, unitDir)
				continue
			}

			key := filepath.Join(targetDir, unit.Path)
			declaredUnits[key] = unit

			wp.Submit(func() error {
				out, err := readUnitOutput(ctx, l, pctx, unit, unitDir)
				if err != nil {
					return err
				}

				outputs.Store(key, out)

				return nil
			})
		}
	}

	if err := waitWorkerErrors(nil); err != nil {
		return cty.NilVal, err
	}

	// unitOutputs holds each unit's dotted stack address, grouped by the
	// position of the stack root that materialized it ("" = the working
	// directory itself). Without the stack-output-implicit experiment every
	// unit lands in the "" group, producing the historical flat layout.
	unitOutputs := make(map[string]map[string]map[string]cty.Value)

	// Build stack list separated by stacks, find all nested stacks, and build
	// a dotted path. If no stack is found, use the unit name.
	for path, unit := range declaredUnits {
		output, found := outputs.Load(path)
		if !found {
			l.Debugf("No output found for %s", path)
			continue
		}

		// Implement more logic to find all stacks in which the path is located
		stackNames := []string{}
		nameToPath := make(map[string]string) // Map to track which path each stack name came from

		for stackPath, stackName := range declaredStacks {
			if strings.Contains(path, stackPath) {
				stackNames = append(stackNames, stackName)
				nameToPath[stackName] = stackPath
			}
		}

		// Sort by stack path length so the outermost stack (shortest path) comes
		// first; this preserves the parent.child nesting order in the joined key.
		slices.SortFunc(stackNames, func(a, b string) int {
			return len(nameToPath[a]) - len(nameToPath[b])
		})

		stackKey := unit.Name
		if len(stackNames) > 0 {
			stackKey = strings.Join(stackNames, ".") + "." + unit.Name
		}

		// With the stack-output-implicit experiment, each stack's tree is
		// nested under the literal path of its stack directory relative to the
		// working directory, so explicit keys live in the same path namespace
		// as implicit units and cannot collide with them. Stacks in the
		// working directory itself keep the established top-level shape.
		position := ""

		if implicitMerge {
			var posErr error

			position, posErr = stackPositionPrefix(opts.WorkingDir, path)
			if posErr != nil {
				return cty.NilVal, posErr
			}
		}

		if unitOutputs[position] == nil {
			unitOutputs[position] = make(map[string]map[string]cty.Value)
		}

		unitOutputs[position][stackKey] = output

		l.Debugf("Added output for stack key %s at position %q", stackKey, position)
	}

	final := make(map[string]any)

	// Nest each position group. The "" group sorts first and is copied to the
	// top level, keeping the historical shape; the other groups hang under
	// their literal position keys.
	for _, position := range slices.Sorted(maps.Keys(unitOutputs)) {
		nested, err := nestUnitOutputs(unitOutputs[position])
		if err != nil {
			return cty.NilVal, fmt.Errorf("failed to nest unit outputs: %w", err)
		}

		if position == "" {
			maps.Copy(final, nested)
			continue
		}

		if _, exists := final[position]; exists {
			l.Warnf(
				"Skipping stack outputs at %s: the key is already taken by a unit of a stack in the working directory",
				position,
			)

			continue
		}

		final[position] = nested
	}

	if implicitMerge {
		// Loose units living outside any declared stack are aggregated too,
		// keyed by relative path. Skip the units the discovery excluded plus
		// the units materialized by the explicit stacks above
		// (<stack-dir>/.terragrunt-stack/<unit-path>), so they are not counted
		// twice. Nested stacks are covered because the recursive stack-file
		// walk parses them into parsedStackFiles like any other.
		skipPaths := make(map[string]struct{}, len(excludedPaths))
		maps.Copy(skipPaths, excludedPaths)

		for stackPath, stackFile := range parsedStackFiles {
			targetDir := filepath.Join(filepath.Dir(stackPath), config.StackDir)

			for _, unit := range stackFile.Units {
				skipPaths[filepath.Clean(filepath.Join(targetDir, unit.Path))] = struct{}{}
			}
		}

		implicitOutputs, err := implicitStackOutput(ctx, l, opts, skipPaths)
		if err != nil {
			return cty.NilVal, err
		}

		for key, val := range implicitOutputs {
			// Defensive: only reachable when a directory holds both a
			// terragrunt.hcl and a terragrunt.stack.hcl, making the same
			// position an implicit unit and an explicit stack root at once.
			if _, exists := final[key]; exists {
				l.Warnf("Skipping implicit output for %s: an explicit stack already defines this key", key)
				continue
			}

			final[key] = val
		}
	}

	// No stack files and nothing discovered: keep the NilVal contract so the
	// printers emit nothing rather than an empty object.
	if len(foundFiles) == 0 && len(final) == 0 {
		return cty.NilVal, nil
	}

	ctyResult, err := config.GoTypeToCty(final)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to convert unit output to cty value: %w", err)
	}

	return ctyResult, nil
}

// stackPositionPrefix returns the path of the stack root that materialized the
// given unit, relative to the working directory with forward slashes: the
// portion of the unit's relative path before the first .terragrunt-stack
// segment. It returns "" when the stack file lives in the working directory
// itself (no prefix) or the unit path does not match the expected layout.
// Nested stacks inherit the outermost stack's position because their units are
// materialized inside its .terragrunt-stack tree.
func stackPositionPrefix(workingDir, unitDir string) (string, error) {
	rel, err := filepath.Rel(workingDir, unitDir)
	if err != nil {
		return "", fmt.Errorf("failed to determine relative path of unit %s: %w", unitDir, err)
	}

	segments := strings.Split(filepath.ToSlash(rel), "/")

	if i := slices.Index(segments, config.StackDir); i > 0 {
		return strings.Join(segments[:i], "/"), nil
	}

	return "", nil
}

// nestUnitOutputs transforms a flat map of unit outputs into a nested hierarchical structure.
//
// This function is a critical part of Terragrunt/Opentofu's stack output system, converting flat key-value pairs
// with dot notation into a proper nested object hierarchy. It processes each flattened key (e.g., "parent.child.unit")
// by splitting it into path segments and recursively building the corresponding nested structure.
//
// The algorithm works as follows:
//  1. For each entry in the flat map, split its key by dots to get the path segments
//  2. Iteratively traverse the nested structure, creating intermediate maps as needed
//  3. When reaching the final path segment, convert the map of cty.Values to a Go interface{}
//     representation and store it at that location
//  4. Continue until all flat entries have been properly nested
//
// This approach preserves the hierarchical relationship between stacks and units while making
// the data structure easier to navigate and query programmatically.
//
// Parameters:
//   - flat: A map where keys are dot-separated paths (e.g., "parent.child.unit") and values are
//     maps of cty.Value representing the OpenTofu/Terraform outputs for each unit
//
// Returns:
//   - map[string]any: A nested map structure reflecting the hierarchy implied by the dot notation
//   - error: An error if conversion fails, particularly when building the nested structure
//
// Errors can occur during cty.Value conversion or when attempting to traverse the nested structure
// if the path contains contradictory type information (e.g., a path segment is both a leaf and a branch).
func nestUnitOutputs(flat map[string]map[string]cty.Value) (map[string]any, error) {
	nested := make(map[string]any)

	for flatKey, value := range flat {
		parts := strings.Split(flatKey, ".")
		current := nested

		for i, part := range parts {
			if i == len(parts)-1 {
				ctyValue, err := config.ConvertValuesMapToCtyVal(value)
				if err != nil {
					return nil, fmt.Errorf("failed to convert unit output to cty value: %s %w", flatKey, err)
				}

				current[part] = ctyValue
			} else {
				if _, exists := current[part]; !exists { // Traverse or create next level
					current[part] = make(map[string]any)
				}

				var ok bool

				current, ok = current[part].(map[string]any)

				if !ok {
					return nil, fmt.Errorf("failed to traverse unit output: %v %s", flat, part)
				}
			}
		}
	}

	return nested, nil
}

// readUnitOutput returns the tofu/terraform outputs for a unit.
func readUnitOutput(
	ctx context.Context,
	l log.Logger,
	pctx *config.ParsingContext,
	unit *config.Unit,
	unitDir string,
) (map[string]cty.Value, error) {
	var output map[string]cty.Value

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "unit_output", map[string]any{
		"unit_name":   unit.Name,
		"unit_source": unit.Source,
		"unit_path":   unit.Path,
	}, func(ctx context.Context, l log.Logger) error {
		var outputErr error

		output, outputErr = unit.ReadOutputs(ctx, l, pctx, unitDir)

		return outputErr
	})
	if err != nil {
		return nil, UnitOutputError{UnitName: unit.Name, UnitDir: unitDir, Err: err}
	}

	return output, nil
}

// implicitStackOutput collects the outputs of units that are not declared in
// any terragrunt.stack.hcl, each keyed by its path relative to opts.WorkingDir
// (e.g. "foo/bar"). excludedPaths holds the unit directories to skip: units
// excluded by `exclude` blocks against opts.TerraformCommand plus units
// already materialized by explicit stacks.
func implicitStackOutput(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	excludedPaths map[string]struct{},
) (map[string]cty.Value, error) {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to discover units in %s: %w", opts.WorkingDir, err)
	}

	units := components.Filter(component.UnitKind)

	if len(units) == 0 {
		l.Warnf("No units found in %s. Nothing to output.", opts.WorkingDir)
		return nil, nil
	}

	outputs := xsync.NewMap[string, cty.Value]()

	wp := worker.NewWorkerPool(opts.Parallelism)
	defer wp.Stop()

	for _, c := range units {
		unitDir := c.Path()

		if _, excluded := excludedPaths[filepath.Clean(unitDir)]; excluded {
			l.Debugf("Skipping output for excluded unit in %s", unitDir)
			continue
		}

		rel, relErr := filepath.Rel(opts.WorkingDir, unitDir)
		if relErr != nil {
			return nil, fmt.Errorf("failed to determine relative path of unit %s: %w", unitDir, relErr)
		}

		key := filepath.ToSlash(rel)

		ctx, pctx := configbridge.NewParsingContext(ctx, l, opts)
		unit := &config.Unit{Name: key}

		wp.Submit(func() error {
			out, err := readUnitOutput(ctx, l, pctx, unit, unitDir)
			if err != nil {
				return err
			}

			ctyVal, err := config.ConvertValuesMapToCtyVal(out)
			if err != nil {
				return fmt.Errorf("failed to convert unit output to cty value for %s: %w", key, err)
			}

			outputs.Store(key, ctyVal)

			return nil
		})
	}

	if err := wp.Wait(); err != nil {
		return nil, err
	}

	result := make(map[string]cty.Value)

	outputs.Range(func(k string, v cty.Value) bool {
		result[k] = v
		return true
	})

	return result, nil
}

// buildWorktreesIfNeeded creates worktrees if the filter-flag experiment is enabled and git filters exist.
// Returns nil worktrees if the experiment is not enabled or no git filters are present.
func buildWorktreesIfNeeded(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (*worktrees.Worktrees, error) {
	gitFilters := opts.Filters.UniqueGitFilters()
	if len(gitFilters) == 0 {
		return nil, nil
	}

	return worktrees.NewWorktrees(ctx, l, worktrees.WorktreeOpts{
		WorkingDir:     opts.WorkingDir,
		GitExpressions: gitFilters,
		Experiments:    opts.Experiments,
	})
}
