// Package output provides functionality for collecting and collating the
// unit outputs for all units in a stack hierarchy.
package output

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
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

// UnitAddressCollisionError is returned when two units resolve to the same output
// address. Expansion makes this reachable: a nested stack and an expanded unit can claim
// the same name.key pair.
type UnitAddressCollisionError struct {
	Address string
}

func (e UnitAddressCollisionError) Error() string {
	return fmt.Sprintf("more than one unit resolves to the output address %q", e.Address)
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
	v *venv.Venv,
	opts *options.TerragruntOptions,
) (cty.Value, error) {
	l.Debugf("Generating output from %s", opts.WorkingDir)

	// Create worktrees internally if filter-flag experiment is enabled and git filters are present
	wts, err := buildWorktreesIfNeeded(ctx, l, v, opts)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to create worktrees: %w", err)
	}

	if wts != nil {
		defer func() {
			if cleanupErr := wts.Cleanup(ctx, l, v.FS); cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()
	}

	// Single discovery walk returns both stack files and excluded unit paths.
	foundFiles, excludedPaths, err := generate.ListStackFilesWithExcludes(ctx, l, v, opts, wts)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to list stack files in %s: %w", opts.WorkingDir, err)
	}

	if len(foundFiles) == 0 {
		l.Warnf("No stack files found in %s Nothing to generate.", opts.WorkingDir)
		return cty.NilVal, nil
	}

	outputs := xsync.NewMap[string, map[string]cty.Value]()
	declaredStacks := make(map[string][]string)
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

		ctx, pctx := configbridge.NewParsingContext(ctx, l, v, opts)

		values, valuesErr := config.ReadValues(ctx, pctx, l, dir)
		if valuesErr != nil {
			return cty.NilVal, waitWorkerErrors(
				fmt.Errorf("failed to read values from %s: %w", dir, valuesErr),
			)
		}

		stackFile, stackErr := config.ReadStackConfigFile(ctx, l, pctx, path, values)
		if stackErr != nil {
			return cty.NilVal, waitWorkerErrors(
				fmt.Errorf("failed to read stack file %s: %w", path, stackErr),
			)
		}

		parsedStackFiles[path] = stackFile

		targetDir := filepath.Join(dir, config.StackDir)

		for _, stack := range stackFile.Stacks {
			if !stack.IsEnabled() {
				continue
			}

			stackPath := filepath.Join(targetDir, stack.Path)
			key, expanded := stack.InstanceKey()
			declaredStacks[stackPath] = componentAddress(stack.Name, key, expanded)

			l.Debugf("Registered stack %s at path %s", stack.Name, stackPath)
		}

		for _, unit := range stackFile.Units {
			if !unit.IsEnabled() {
				continue
			}

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

	collected := make([]UnitOutput, 0, len(declaredUnits))

	for path, unit := range declaredUnits {
		output, found := outputs.Load(path)
		if !found {
			l.Debugf("No output found for %s", path)
			continue
		}

		collected = append(collected, UnitOutput{Unit: unit, Outputs: output, Path: path})
	}

	nestedOutputs, err := NestOutputs(l, declaredStacks, collected)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to nest unit outputs: %w", err)
	}

	ctyResult, err := config.GoTypeToCty(nestedOutputs)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to convert unit outputs to a cty value: %w", err)
	}

	return ctyResult, nil
}

// UnitOutput is one unit's outputs together with the on-disk path it generated to.
type UnitOutput struct {
	Unit    *config.Unit
	Outputs map[string]cty.Value
	Path    string
}

// NestOutputs arranges each unit's outputs under the address it is reachable at: a unit
// inside a stack at stack.unit, and one element of an expanded unit at unit.key.
func NestOutputs(
	l log.Logger,
	declaredStacks map[string][]string,
	units []UnitOutput,
) (map[string]any, error) {
	// A stack file that declares no stacks still yields an empty map, so nil means the
	// caller never built one rather than that there is nothing to place units under.
	if declaredStacks == nil {
		panic("NestOutputs requires the non-nil declaredStacks map that StackOutput builds")
	}

	addressed := make([]unitOutput, 0, len(units))

	for _, unit := range units {
		key, expanded := unit.Unit.InstanceKey()
		address := slices.Concat(
			enclosingStackAddress(declaredStacks, unit.Path),
			componentAddress(unit.Unit.Name, key, expanded),
		)

		addressed = append(addressed, unitOutput{address: address, outputs: unit.Outputs})

		l.Debugf("Added output for %s", strings.Join(address, "."))
	}

	return nestUnitOutputs(addressed)
}

// unitOutput pairs one unit's outputs with the address it is reachable at. The address
// stays split into segments rather than joined with dots, so a stack name, unit name, or
// iteration key that itself contains a dot cannot split into two levels of the result.
type unitOutput struct {
	outputs map[string]cty.Value
	address []string
}

// componentAddress returns the address segments a unit or stack is reachable at. An
// expanded component gets its iteration key as a segment of its own, so that
// `foo["web"]` addresses one element the way `dependency.foo["web"]` already does.
func componentAddress(name, key string, expanded bool) []string {
	if !expanded {
		return []string{name}
	}

	return []string{name, key}
}

// enclosingStackAddress returns the address segments of every stack the unit at path sits
// inside, outermost first, so a unit in a nested stack addresses as parent.child.unit.
func enclosingStackAddress(declaredStacks map[string][]string, path string) []string {
	enclosing := make([]string, 0, len(declaredStacks))

	for stackPath := range declaredStacks {
		if containsPath(stackPath, path) {
			enclosing = append(enclosing, stackPath)
		}
	}

	// Every match is an ancestor of the same unit, so they form a chain in which each is
	// a prefix of the next. Ordering them lexicographically therefore runs the nesting
	// from the outermost stack inward.
	slices.Sort(enclosing)

	address := make([]string, 0, len(enclosing))
	for _, stackPath := range enclosing {
		address = append(address, declaredStacks[stackPath]...)
	}

	return address
}

// containsPath reports whether path sits inside dir. Comparing the cleaned relative path
// rather than testing for a string prefix is what stops a stack generated to team/1 from
// claiming the units under team/10.
func containsPath(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func nestUnitOutputs(unitOutputs []unitOutput) (map[string]any, error) {
	nested := make(map[string]any)

	for _, unit := range unitOutputs {
		current := nested

		for i, segment := range unit.address {
			// Both branches reject an address already spoken for. Overwriting it would
			// drop a unit from the output with nothing to signal the loss.
			if i == len(unit.address)-1 {
				if _, taken := current[segment]; taken {
					return nil, UnitAddressCollisionError{Address: strings.Join(unit.address, ".")}
				}

				ctyValue, err := config.ConvertValuesMapToCtyVal(unit.outputs)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to convert unit output to cty value: %s %w",
						strings.Join(unit.address, "."),
						err,
					)
				}

				current[segment] = ctyValue

				break
			}

			if _, exists := current[segment]; !exists {
				current[segment] = make(map[string]any)
			}

			next, ok := current[segment].(map[string]any)
			if !ok {
				return nil, UnitAddressCollisionError{
					Address: strings.Join(unit.address[:i+1], "."),
				}
			}

			current = next
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

// buildWorktreesIfNeeded creates worktrees if the filter-flag experiment is enabled and git filters exist.
// Returns nil worktrees if the experiment is not enabled or no git filters are present.
func buildWorktreesIfNeeded(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) (*worktrees.Worktrees, error) {
	gitFilters := opts.Filters.UniqueGitFilters()
	if len(gitFilters) == 0 {
		return nil, nil
	}

	return worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
		WorkingDir:     opts.WorkingDir,
		GitExpressions: gitFilters,
		Experiments:    opts.Experiments,
	})
}
