// Package output provides functionality for collecting and collating the
// unit outputs for all units in a stack hierarchy.
package output

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/zclconf/go-cty/cty"
)

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
	opts *options.TerragruntOptions,
) (cty.Value, error) {
	l.Debugf("Generating output from %s", opts.WorkingDir)

	// Create worktrees internally if filter-flag experiment is enabled and git filters are present
	wts, err := buildWorktreesIfNeeded(ctx, l, opts)
	if err != nil {
		return cty.NilVal, errors.Errorf("failed to create worktrees: %w", err)
	}

	if wts != nil {
		defer func() {
			if cleanupErr := wts.Cleanup(ctx, l); cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()
	}

	foundFiles, err := generate.ListStackFiles(ctx, l, opts, opts.WorkingDir, wts)
	if err != nil {
		return cty.NilVal, errors.Errorf("Failed to list stack files in %s: %w", opts.WorkingDir, err)
	}

	if len(foundFiles) == 0 {
		l.Warnf("No stack files found in %s Nothing to generate.", opts.WorkingDir)
		return cty.NilVal, nil
	}

	outputs := make(map[string]map[string]cty.Value)
	declaredStacks := make(map[string]string)
	declaredUnits := make(map[string]*config.Unit)

	// save parsed stacks
	parsedStackFiles := make(map[string]*config.StackConfig, len(foundFiles))

	for _, path := range foundFiles {
		dir := filepath.Dir(path)

		values, valuesErr := config.ReadValues(ctx, l, opts, dir)
		if valuesErr != nil {
			return cty.NilVal, errors.Errorf("Failed to read values from %s: %w", dir, valuesErr)
		}

		stackFile, stackErr := config.ReadStackConfigFile(ctx, l, opts, path, values)
		if stackErr != nil {
			return cty.NilVal, errors.Errorf("Failed to read stack file %s: %w", path, stackErr)
		}

		parsedStackFiles[path] = stackFile

		targetDir := filepath.Join(dir, config.StackDir)

		for _, stack := range stackFile.Stacks {
			declaredStacks[filepath.Join(targetDir, stack.Path)] = stack.Name
			l.Debugf("Registered stack %s at path %s", stack.Name, filepath.Join(targetDir, stack.Path))
		}

		for _, unit := range stackFile.Units {
			unitDir := config.GetUnitDir(dir, unit)

			var output map[string]cty.Value

			telemetryErr := telemetry.TelemeterFromContext(ctx).Collect(ctx, "unit_output", map[string]any{
				"unit_name":   unit.Name,
				"unit_source": unit.Source,
				"unit_path":   unit.Path,
			}, func(ctx context.Context) error {
				var outputErr error

				output, outputErr = unit.ReadOutputs(ctx, l, opts, unitDir)

				return outputErr
			})
			if telemetryErr != nil {
				return cty.NilVal, errors.New(telemetryErr)
			}

			key := filepath.Join(targetDir, unit.Path)
			declaredUnits[key] = unit
			outputs[key] = output

			l.Debugf("Added output for %s", key)
		}
	}

	unitOutputs := make(map[string]map[string]cty.Value)

	// Build stack list separated by stacks, find all nested stacks, and build a dotted path. If no stack is found, use the unit name.
	for path, unit := range declaredUnits {
		output, found := outputs[path]
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

		// Sort stackNames based on the length of stackPath to ensure correct order
		stackNamesSorted := make([]string, len(stackNames))
		copy(stackNamesSorted, stackNames)

		for i := range stackNamesSorted {
			for j := i + 1; j < len(stackNamesSorted); j++ {
				// Compare lengths of the actual paths from the nameToPath map, not the declaredStacks lookup
				if len(nameToPath[stackNamesSorted[i]]) < len(nameToPath[stackNamesSorted[j]]) {
					stackNamesSorted[i], stackNamesSorted[j] = stackNamesSorted[j], stackNamesSorted[i]
				}
			}
		}

		stackKey := unit.Name
		if len(stackNamesSorted) > 0 {
			stackKey = strings.Join(stackNamesSorted, ".") + "." + unit.Name
		}

		unitOutputs[stackKey] = output

		l.Debugf("Added output for stack key %s", stackKey)
	}

	// Convert finalMap into a cty.ObjectVal
	result := make(map[string]cty.Value)

	nestedOutputs, err := nestUnitOutputs(unitOutputs)
	if err != nil {
		return cty.NilVal, errors.Errorf("Failed to nest unit outputs: %w", err)
	}

	ctyResult, err := config.GoTypeToCty(nestedOutputs)
	if err != nil {
		return cty.NilVal, errors.Errorf("Failed to convert unit output to cty value: %s %w", result, err)
	}

	return ctyResult, nil
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
					return nil, errors.Errorf("Failed to convert unit output to cty value: %s %w", flatKey, err)
				}

				current[part] = ctyValue
			} else {
				if _, exists := current[part]; !exists { // Traverse or create next level
					current[part] = make(map[string]any)
				}

				var ok bool

				current, ok = current[part].(map[string]any)

				if !ok {
					return nil, errors.Errorf("Failed to traverse unit output: %v %s", flat, part)
				}
			}
		}
	}

	return nested, nil
}

// buildWorktreesIfNeeded creates worktrees if the filter-flag experiment is enabled and git filters exist.
// Returns nil worktrees if the experiment is not enabled or no git filters are present.
func buildWorktreesIfNeeded(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (*worktrees.Worktrees, error) {
	if !opts.Experiments.Evaluate(experiment.FilterFlag) {
		return nil, nil
	}

	filters, err := filter.ParseFilterQueries(opts.FilterQueries)
	if err != nil {
		return nil, errors.Errorf("failed to parse filters: %w", err)
	}

	gitFilters := filters.UniqueGitFilters()
	if len(gitFilters) == 0 {
		return nil, nil
	}

	return worktrees.NewWorktrees(ctx, l, opts.WorkingDir, gitFilters)
}
