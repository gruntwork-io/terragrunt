package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/worker"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/hashicorp/go-getter/v2"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	StackDir          = ".terragrunt-stack"
	valuesFile        = "terragrunt.values.hcl"
	manifestName      = ".terragrunt-stack-manifest"
	defaultStackFile  = "terragrunt.stack.hcl"
	unitDirPerm       = 0755
	valueFilePerm     = 0644
	generationMaxPath = 1024
)

// StackConfigFile represents the structure of terragrunt.stack.hcl stack file.
type StackConfigFile struct {
	Locals *terragruntLocal `hcl:"locals,block"`
	Stacks []*Stack         `hcl:"stack,block"`
	Units  []*Unit          `hcl:"unit,block"`
}

// StackConfig represents the structure of terragrunt.stack.hcl stack file.
type StackConfig struct {
	Locals map[string]any
	Stacks []*Stack
	Units  []*Unit
}

// Unit represents unit from a stack file.
type Unit struct {
	NoStack      *bool      `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation *bool      `hcl:"no_validation,attr"`
	Values       *cty.Value `hcl:"values,attr"`
	Name         string     `hcl:",label"`
	Source       string     `hcl:"source,attr"`
	Path         string     `hcl:"path,attr"`
}

// Stack represents the stack block in the configuration.
type Stack struct {
	NoStack      *bool      `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation *bool      `hcl:"no_validation,attr"`
	Values       *cty.Value `hcl:"values,attr"`
	Name         string     `hcl:",label"`
	Source       string     `hcl:"source,attr"`
	Path         string     `hcl:"path,attr"`
}

// GenerateStacks generates the stack files.
func GenerateStacks(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	processedFiles := make(map[string]bool)
	wp := worker.NewWorkerPool(opts.Parallelism)
	// stop worker pool on exit
	defer wp.Stop()
	// initial files setting as stack file

	foundFiles, err := listStackFiles(l, opts, opts.WorkingDir)
	if err != nil {
		return errors.Errorf("Failed to list stack files in %s %w", opts.WorkingDir, err)
	}

	for {
		// check if we have already processed the files
		processedNewFiles := false

		for _, file := range foundFiles {
			if processedFiles[file] {
				continue
			}

			processedNewFiles = true
			processedFiles[file] = true

			l.Infof("Generating stack from %s", file)

			if err := generateStackFile(ctx, l, opts, wp, file); err != nil {
				return errors.Errorf("Failed to process stack file %s %w", file, err)
			}
		}

		if wpError := wp.Wait(); wpError != nil {
			return wpError
		}

		if !processedNewFiles {
			break
		}

		newFiles, err := listStackFiles(l, opts, opts.WorkingDir)

		if err != nil {
			return errors.Errorf("Failed to list stack files %w", err)
		}

		foundFiles = newFiles
	}

	return nil
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
func StackOutput(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (cty.Value, error) {
	l.Debugf("Generating output from %s", opts.WorkingDir)

	foundFiles, err := listStackFiles(l, opts, opts.WorkingDir)
	if err != nil {
		return cty.NilVal, errors.Errorf("Failed to list stack files in %s: %w", opts.WorkingDir, err)
	}

	outputs := make(map[string]map[string]cty.Value)
	declaredStacks := make(map[string]string)
	declaredUnits := make(map[string]*Unit)

	// save parsed stacks
	parsedStackFiles := make(map[string]*StackConfig, len(foundFiles))

	for _, path := range foundFiles {
		dir := filepath.Dir(path)

		values, err := ReadValues(ctx, l, opts, dir)
		if err != nil {
			return cty.NilVal, errors.Errorf("Failed to read values from %s: %w", dir, err)
		}

		stackFile, err := ReadStackConfigFile(ctx, l, opts, path, values)
		if err != nil {
			return cty.NilVal, errors.Errorf("Failed to read stack file %s: %w", path, err)
		}

		parsedStackFiles[path] = stackFile

		targetDir := filepath.Join(dir, StackDir)

		for _, stack := range stackFile.Stacks {
			declaredStacks[filepath.Join(targetDir, stack.Path)] = stack.Name
			l.Debugf("Registered stack %s at path %s", stack.Name, filepath.Join(targetDir, stack.Path))
		}

		for _, unit := range stackFile.Units {
			unitDir := filepath.Join(dir, StackDir, unit.Path)

			var output map[string]cty.Value

			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "unit_output", map[string]any{
				"unit_name":   unit.Name,
				"unit_source": unit.Source,
				"unit_path":   unit.Path,
			}, func(ctx context.Context) error {
				output, err = unit.ReadOutputs(ctx, l, opts, unitDir)
				return err
			})

			if err != nil {
				return cty.NilVal, errors.New(err)
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

		for i := 0; i < len(stackNamesSorted); i++ {
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

	ctyResult, err := goTypeToCty(nestedOutputs)

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
				ctyValue, err := convertValuesMapToCtyVal(value)
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

// generateStackFile processes the Terragrunt stack configuration from the given stackFilePath,
// reads necessary values, and generates units and stacks in the target directory.
// It handles the creation of required directories and returns any errors encountered.
func generateStackFile(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, pool *worker.Pool, stackFilePath string) error {
	stackSourceDir := filepath.Dir(stackFilePath)

	values, err := ReadValues(ctx, l, opts, stackSourceDir)
	if err != nil {
		return errors.Errorf("failed to read values from directory %s: %w", stackSourceDir, err)
	}

	stackFile, err := ReadStackConfigFile(ctx, l, opts, stackFilePath, values)

	if err != nil {
		return errors.Errorf("Failed to read stack file %s in %s %w", stackFilePath, stackSourceDir, err)
	}

	stackTargetDir := filepath.Join(stackSourceDir, StackDir)

	if err := generateUnits(ctx, l, opts, pool, stackFilePath, stackSourceDir, stackTargetDir, stackFile.Units); err != nil {
		return err
	}

	if err := generateStacks(ctx, l, opts, pool, stackFilePath, stackSourceDir, stackTargetDir, stackFile.Stacks); err != nil {
		return err
	}

	return nil
}

// generateUnits iterates through a slice of Unit objects, processing each one by copying
// source files to their destination paths and writing unit-specific values.
// It logs the processing progress and returns any errors encountered during the operation.
func generateUnits(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, pool *worker.Pool, sourceFile, sourceDir, targetDir string, units []*Unit) error {
	for _, unit := range units {
		unitCopy := unit // Create a copy to avoid capturing the loop variable reference

		pool.Submit(func() error {
			item := componentToProcess{
				sourceDir:    sourceDir,
				targetDir:    targetDir,
				name:         unitCopy.Name,
				path:         unitCopy.Path,
				source:       unitCopy.Source,
				values:       unitCopy.Values,
				noStack:      unitCopy.NoStack != nil && *unitCopy.NoStack,
				noValidation: unitCopy.NoValidation != nil && *unitCopy.NoValidation,
				kind:         unitKind,
			}

			l.Infof("Processing unit %s from %s", unitCopy.Name, sourceFile)

			return telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate_unit", map[string]any{
				"stack_file":  sourceFile,
				"unit_name":   unitCopy.Name,
				"unit_source": unitCopy.Source,
				"unit_path":   unitCopy.Path,
			}, func(ctx context.Context) error {
				return processComponent(ctx, l, opts, &item)
			})
		})
	}

	return nil
}

// generateStacks processes each stack by resolving its destination path and copying files from the source.
// It logs each operation and returns early if any error is encountered.
func generateStacks(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, pool *worker.Pool, sourceFile, sourceDir, targetDir string, stacks []*Stack) error {
	for _, stack := range stacks {
		stackCopy := stack // Create a copy to avoid capturing the loop variable reference

		pool.Submit(func() error {
			item := componentToProcess{
				sourceDir:    sourceDir,
				targetDir:    targetDir,
				name:         stackCopy.Name,
				path:         stackCopy.Path,
				source:       stackCopy.Source,
				noStack:      stackCopy.NoStack != nil && *stackCopy.NoStack,
				noValidation: stackCopy.NoValidation != nil && *stackCopy.NoValidation,
				values:       stackCopy.Values,
				kind:         stackKind,
			}

			l.Infof("Processing stack %s from %s", stackCopy.Name, sourceFile)

			return telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate_stack", map[string]any{
				"stack_file":   sourceFile,
				"stack_name":   stackCopy.Name,
				"stack_source": stackCopy.Source,
				"stack_path":   stackCopy.Path,
			}, func(ctx context.Context) error {
				return processComponent(ctx, l, opts, &item)
			})
		})
	}

	return nil
}

type componentKind int

const (
	unitKind componentKind = iota
	stackKind
)

// componentToProcess represents an item of work for processing a stack or unit.
// It contains information about the source and target directories, the name and path of the item, the source URL or path,
// and any associated values that need to be processed.
type componentToProcess struct {
	values       *cty.Value
	sourceDir    string
	targetDir    string
	name         string
	path         string
	source       string
	noStack      bool
	noValidation bool
	kind         componentKind
}

// processComponent copies files from the source directory to the target destination and generates a corresponding values file.
func processComponent(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cmp *componentToProcess) error {
	source := cmp.source
	// Adjust source path using the provided source mapping configuration if available
	source, err := adjustSourceWithMap(opts.SourceMap, source, opts.TerragruntStackConfigPath)

	if err != nil {
		return errors.Errorf("failed to adjust source %s: %w", cmp.source, err)
	}

	if filepath.IsAbs(cmp.path) {
		return errors.Errorf("path %s must be relative", cmp.path)
	}

	kindStr := "unit"
	if cmp.kind == stackKind {
		kindStr = "stack"
	}

	// building destination path based on target directory
	dest := filepath.Join(cmp.targetDir, cmp.path)

	// validate destination path is within the stack directory
	// get the absolute path of the destination directory
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return errors.Errorf("failed to get absolute path for destination '%s': %w", cmp.name, err)
	}

	// get the absolute path of the stack directory
	absStackDir, err := filepath.Abs(cmp.targetDir)
	if err != nil {
		return errors.Errorf("failed to get absolute path for stack directory '%s': %w", cmp.name, err)
	}

	// validate that the destination path is within the stack directory
	if !strings.HasPrefix(absDest, absStackDir) {
		return errors.Errorf("%s destination path '%s' is outside of the stack directory '%s'", cmp.name, absDest, absStackDir)
	}

	if cmp.noStack {
		// for noStack components, we copy the files to the base directory of the target directory
		dest = filepath.Join(filepath.Dir(cmp.targetDir), cmp.path)
	}

	l.Debugf("Processing: %s (%s) to %s", cmp.name, source, dest)

	if err := copyFiles(ctx, l, cmp.name, cmp.sourceDir, source, dest); err != nil {
		return errors.Errorf(
			"Failed to fetch %s %s\n"+
				"  Source:      %s\n"+
				"  Destination: %s\n\n"+
				"Troubleshooting:\n"+
				"  1. Check if your source path is correct relative to the stack file location\n"+
				"  2. Verify the units or stacks directory exists at the expected location\n"+
				"  3. Ensure you have proper permissions to read from source and write to destination\n\n"+
				"Original error: %w",
			kindStr,
			cmp.name,
			source,
			dest,
			err,
		)
	}

	skipValidation := false

	if cmp.noStack {
		l.Debugf("Skipping validation for %s %s due to no_stack flag", kindStr, cmp.name)

		skipValidation = true
	}

	if cmp.noValidation {
		l.Debugf("Skipping validation for %s %s due to no_validation flag", kindStr, cmp.name)

		skipValidation = true
	}

	if !skipValidation {
		// validate what was copied to the destination, don't do validation for special noStack components
		expectedFile := DefaultTerragruntConfigPath

		if cmp.kind == stackKind {
			expectedFile = defaultStackFile
		}

		if err := validateTargetDir(kindStr, cmp.name, dest, expectedFile); err != nil {
			if opts.NoStackValidate {
				// print warning if validation is skipped
				l.Warnf("Suppressing validation error for %s %s at path %s: expected %s to generate with %s file at root of generated directory.", kindStr, cmp.name, cmp.targetDir, kindStr, expectedFile)
			} else {
				return errors.Errorf("Validation failed for %s %s at path %s: expected %s to generate with %s file at root of generated directory.", kindStr, cmp.name, cmp.targetDir, kindStr, expectedFile)
			}
		}
	}

	// generate values file
	if err := writeValues(l, cmp.values, dest); err != nil {
		return errors.Errorf("failed to write values %v %w", cmp.name, err)
	}

	return nil
}

// copyFiles copies files or directories from a source to a destination path.
//
// The function checks if the source is local or remote. If local, it copies the
// contents of the source directory to the destination. If remote, it fetches the
// source and stores it in the destination directory.
func copyFiles(ctx context.Context, l log.Logger, identifier, sourceDir, src, dest string) error {
	if isLocal(l, sourceDir, src) {
		// check if src is absolute path, if not, join with sourceDir
		var localSrc string

		if filepath.IsAbs(src) {
			localSrc = src
		} else {
			localSrc = filepath.Join(sourceDir, src)
		}

		localSrc, err := filepath.Abs(localSrc)

		if err != nil {
			l.Warnf("failed to get absolute path for source '%s': %w", identifier, err)
			// fallback to original source
			localSrc = src
		}

		if err := util.CopyFolderContentsWithFilter(l, localSrc, dest, manifestName, func(absolutePath string) bool {
			return true
		}); err != nil {
			return errors.Errorf("Failed to copy %s to %s %w", localSrc, dest, err)
		}
	} else {
		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			return errors.Errorf("Failed to create directory %s for %s %w", dest, identifier, err)
		}

		if _, err := getter.GetAny(ctx, dest, src); err != nil {
			return errors.Errorf("Failed to fetch %s %s for %s %w", src, dest, identifier, err)
		}
	}

	return nil
}

// isLocal determines if a given source path is local or remote.
//
// It checks if the provided source file exists locally. If not, it checks if
// the path is relative to the working directory. If that also fails, the function
// attempts to detect the source's getter type and recognizes if it is a file URL.
func isLocal(l log.Logger, workingDir, src string) bool {
	// check initially if the source is a local file
	if util.FileExists(src) {
		return true
	}

	src = filepath.Join(workingDir, src)
	if util.FileExists(src) {
		return true
	}
	// check path through getters
	req := &getter.Request{
		Src: src,
	}
	for _, g := range getter.Getters {
		recognized, err := getter.Detect(req, g)
		if err != nil {
			l.Debugf("Error detecting getter for %s: %w", src, err)
			continue
		}

		if recognized {
			break
		}
	}

	return strings.HasPrefix(req.Src, "file://")
}

// ReadOutputs retrieves the OpenTofu/Terraform output JSON for this unit, converts it into a map of cty.Values,
// and logs the operation for debugging. It returns early in case of any errors during retrieval or conversion.
func (u *Unit) ReadOutputs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, unitDir string) (map[string]cty.Value, error) {
	configPath := filepath.Join(unitDir, DefaultTerragruntConfigPath)
	l.Debugf("Getting output from unit %s in %s", u.Name, unitDir)

	parserCtx := NewParsingContext(ctx, l, opts)

	jsonBytes, err := getOutputJSONWithCaching(parserCtx, l, configPath) //nolint: contextcheck

	if err != nil {
		return nil, errors.New(err)
	}

	outputMap, err := TerraformOutputJSONToCtyValueMap(configPath, jsonBytes)

	if err != nil {
		return nil, errors.New(err)
	}

	return outputMap, nil
}

// ReadStackConfigFile reads and parses a Terragrunt stack configuration file from the given path.
// It creates a parsing context, processes locals, and decodes the file into a StackConfig struct.
// Validation is performed on the resulting config, and any encountered errors cause an early return.
func ReadStackConfigFile(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, filePath string, values *cty.Value) (*StackConfig, error) {
	l.Debugf("Reading Terragrunt stack config file at %s", filePath)

	parser := NewParsingContext(ctx, l, opts)

	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(filePath)
	if err != nil {
		return nil, errors.New(err)
	}

	//nolint:contextcheck
	return ParseStackConfig(l, parser, opts, file, values)
}

// ReadStackConfigString reads and parses a Terragrunt stack configuration from a string.
func ReadStackConfigString(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	configPath string,
	configString string,
	values *cty.Value,
) (*StackConfig, error) {
	parser := NewParsingContext(ctx, l, opts)

	if values != nil {
		parser = parser.WithValues(values)
	}

	hclFile, err := hclparse.NewParser(parser.ParserOptions...).ParseFromString(configString, configPath)
	if err != nil {
		return nil, errors.New(err)
	}

	//nolint:contextcheck
	return ParseStackConfig(l, parser, opts, hclFile, values)
}

// ParseStackConfig parses the stack configuration from the given file and values.
func ParseStackConfig(l log.Logger, parser *ParsingContext, opts *options.TerragruntOptions, file *hclparse.File, values *cty.Value) (*StackConfig, error) {
	if values != nil {
		parser = parser.WithValues(values)
	}

	//nolint:contextcheck
	if err := processLocals(l, parser, opts, file); err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	evalParsingContext, err := createTerragruntEvalContext(parser, l, file.ConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}

	config := &StackConfigFile{}
	if err := file.Decode(config, evalParsingContext); err != nil {
		return nil, errors.New(err)
	}

	localsParsed := map[string]any{}
	if parser.Locals != nil {
		localsParsed, err = ctyhelper.ParseCtyValueToMap(*parser.Locals)
		if err != nil {
			return nil, errors.New(err)
		}
	}

	stackConfig := &StackConfig{
		Locals: localsParsed,
		Stacks: config.Stacks,
		Units:  config.Units,
	}

	if err := ValidateStackConfig(config); err != nil {
		return nil, errors.New(err)
	}

	return stackConfig, nil
}

// writeValues generates and writes values to a terragrunt.values.hcl file in the specified directory.
func writeValues(l log.Logger, values *cty.Value, directory string) error {
	if values == nil {
		l.Debugf("No values to write in %s", directory)
		return nil
	}

	if directory == "" {
		return errors.New("writeValues: unit directory path cannot be empty")
	}

	if err := os.MkdirAll(directory, unitDirPerm); err != nil {
		return errors.Errorf("failed to create directory %s: %w", directory, err)
	}

	l.Debugf("Writing values file in %s", directory)
	filePath := filepath.Join(directory, valuesFile)

	file := hclwrite.NewEmptyFile()
	body := file.Body()
	body.AppendUnstructuredTokens([]*hclwrite.Token{
		{
			Type:  hclsyntax.TokenComment,
			Bytes: []byte("# Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually\n"),
		},
	})

	for key, val := range values.AsValueMap() {
		body.SetAttributeValue(key, val)
	}

	if err := os.WriteFile(filePath, file.Bytes(), valueFilePerm); err != nil {
		return errors.Errorf("failed to write values file %s: %w", filePath, err)
	}

	return nil
}

// ReadValues reads values from the terragrunt.values.hcl file in the specified directory.
func ReadValues(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, directory string) (*cty.Value, error) {
	if directory == "" {
		return nil, errors.New("ReadValues: directory path cannot be empty")
	}

	filePath := filepath.Join(directory, valuesFile)

	if util.FileNotExists(filePath) {
		return nil, nil
	}

	l.Debugf("Reading Terragrunt stack values file at %s", filePath)
	parser := NewParsingContext(ctx, l, opts)
	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(filePath)

	if err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	evalParsingContext, err := createTerragruntEvalContext(parser, l, file.ConfigPath)

	if err != nil {
		return nil, errors.New(err)
	}

	values := map[string]cty.Value{}

	if err := file.Decode(&values, evalParsingContext); err != nil {
		return nil, errors.New(err)
	}

	result := cty.ObjectVal(values)

	return &result, nil
}

// processLocals processes the locals block in the stack file.
func processLocals(l log.Logger, parser *ParsingContext, opts *options.TerragruntOptions, file *hclparse.File) error {
	localsBlock, err := file.Blocks(MetadataLocals, false)

	if err != nil {
		return errors.New(err)
	}

	if len(localsBlock) == 0 {
		return nil
	}

	if len(localsBlock) > 1 {
		return errors.New(fmt.Sprintf("up to one locals block is allowed per stack file, but found %d in %s", len(localsBlock), file.ConfigPath))
	}

	attrs, err := localsBlock[0].JustAttributes()

	if err != nil {
		return errors.New(err)
	}

	evaluatedLocals := map[string]cty.Value{}
	evaluated := true

	for iterations := 0; len(attrs) > 0 && evaluated; iterations++ {
		if iterations > MaxIter {
			// Reached maximum supported iterations, which is most likely an infinite loop bug so cut the iteration
			// short and return an error.
			return errors.New(MaxIterError{})
		}

		var err error
		attrs, evaluatedLocals, evaluated, err = attemptEvaluateLocals(
			parser,
			l,
			file,
			attrs,
			evaluatedLocals,
		)

		if err != nil {
			l.Debugf("Encountered error while evaluating locals in file %s", opts.TerragruntStackConfigPath)

			return errors.New(err)
		}
	}

	localsAsCtyVal, err := convertValuesMapToCtyVal(evaluatedLocals)

	if err != nil {
		return errors.New(err)
	}

	parser.Locals = &localsAsCtyVal

	return nil
}

// listStackFiles searches for stack files in the specified directory.
//
// The function walks through the given directory to find files that match the
// default stack file name. It optionally follows symbolic links based on the
// provided Terragrunt options.
func listStackFiles(l log.Logger, opts *options.TerragruntOptions, dir string) ([]string, error) {
	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)
	walkFunc := filepath.Walk

	if walkWithSymlinks {
		walkFunc = util.WalkWithSymlinks
	}

	l.Debugf("Searching for stack files in %s", dir)

	var stackFiles []string

	// find all defaultStackFile files
	if err := walkFunc(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			l.Warnf("Error accessing path %s: %w", path, err)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// skip files in Terragrunt cache directory
		if strings.Contains(path, string(os.PathSeparator)+util.TerragruntCacheDir+string(os.PathSeparator)) ||
			filepath.Base(path) == util.TerragruntCacheDir {
			return filepath.SkipDir
		}

		if len(path) >= generationMaxPath {
			return errors.Errorf("Cycle detected: maximum path length (%d) exceeded at %s", generationMaxPath, path)
		}

		if strings.HasSuffix(path, defaultStackFile) {
			l.Debugf("Found stack file %s", path)
			stackFiles = append(stackFiles, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return stackFiles, nil
}

// validateTargetDir target destination directory.
func validateTargetDir(kind, name, destDir, expectedFile string) error {
	expectedPath := filepath.Join(destDir, expectedFile)

	info, err := os.Stat(expectedPath)
	if err != nil {
		return fmt.Errorf("%s '%s': expected file '%s' not found in target directory '%s': %w", kind, name, expectedFile, destDir, err)
	}

	if info.IsDir() {
		return fmt.Errorf("%s '%s': expected file '%s' is a directory, not a file", kind, name, expectedFile)
	}

	return nil
}
