package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tf"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/worker"

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

// GenerateStackFile generates the Terragrunt stack configuration from the given stackFilePath,
// reads necessary values, and generates units and stacks in the target directory.
// It handles the creation of required directories and returns any errors encountered.
func GenerateStackFile(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, pool *worker.Pool, stackFilePath string) error {
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

// generateUnits iterates through a slice of Unit objects, generating each one by copying
// source files to their destination paths and writing unit-specific values.
// It logs the generating progress and returns any errors encountered during the operation.
func generateUnits(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, pool *worker.Pool, sourceFile, sourceDir, targetDir string, units []*Unit) error {
	for _, unit := range units {
		pool.Submit(func() error {
			item := componentToGenerate{
				sourceDir:    sourceDir,
				targetDir:    targetDir,
				name:         unit.Name,
				path:         unit.Path,
				source:       unit.Source,
				values:       unit.Values,
				noStack:      unit.NoStack != nil && *unit.NoStack,
				noValidation: unit.NoValidation != nil && *unit.NoValidation,
				kind:         unitKind,
			}

			l.Infof("Generating unit %s from %s", unit.Name, sourceFile)

			return telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate_unit", map[string]any{
				"stack_file":  sourceFile,
				"unit_name":   unit.Name,
				"unit_source": unit.Source,
				"unit_path":   unit.Path,
			}, func(ctx context.Context) error {
				return generateComponent(ctx, l, opts, &item)
			})
		})
	}

	return nil
}

// generateStacks generates each stack by resolving its destination path and copying files from the source.
// It logs each operation and returns early if any error is encountered.
func generateStacks(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, pool *worker.Pool, sourceFile, sourceDir, targetDir string, stacks []*Stack) error {
	for _, stack := range stacks {
		pool.Submit(func() error {
			item := componentToGenerate{
				sourceDir:    sourceDir,
				targetDir:    targetDir,
				name:         stack.Name,
				path:         stack.Path,
				source:       stack.Source,
				noStack:      stack.NoStack != nil && *stack.NoStack,
				noValidation: stack.NoValidation != nil && *stack.NoValidation,
				values:       stack.Values,
				kind:         stackKind,
			}

			l.Infof("Generating stack %s from %s", stack.Name, sourceFile)

			return telemetry.TelemeterFromContext(ctx).Collect(ctx, "stack_generate_stack", map[string]any{
				"stack_file":   sourceFile,
				"stack_name":   stack.Name,
				"stack_source": stack.Source,
				"stack_path":   stack.Path,
			}, func(ctx context.Context) error {
				return generateComponent(ctx, l, opts, &item)
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

// componentToGenerate represents an item of work for generating a stack or unit.
// It contains information about the source and target directories, the name and path of the item, the source URL or path,
// and any associated values that need to be generated.
type componentToGenerate struct {
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

// generateComponent copies files from the source directory to the target destination and generates a corresponding values file.
func generateComponent(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cmp *componentToGenerate) error {
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

	l.Debugf("Generating: %s (%s) to %s", cmp.name, source, dest)

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
			expectedFile = DefaultStackFile
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

	stackOpts := opts.Clone()
	stackOpts.TerragruntConfigPath = filePath

	parser := NewParsingContext(ctx, l, stackOpts)

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
	if decodeErr := file.Decode(config, evalParsingContext); decodeErr != nil {
		return nil, errors.New(decodeErr)
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
	// Avoid panics if the provided values are in unsupported format
	if values.IsNull() {
		l.Debugf("Skipping writing values in %s: values is null", directory)
		return nil
	}

	if !values.IsWhollyKnown() {
		l.Debugf("Skipping writing values in %s: values are not fully known", directory)
		return nil
	}

	valType := values.Type()

	if !valType.IsObjectType() && !valType.IsMapType() {
		return errors.Errorf("writeValues: expected object or map, got %s", valType.FriendlyName())
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

	// Sort keys for deterministic output
	valueMap := values.AsValueMap()

	keys := make([]string, 0, len(valueMap))
	for key := range valueMap {
		keys = append(keys, key)
	}

	// Sort keys alphabetically
	sort.Strings(keys)

	for _, key := range keys {
		body.SetAttributeValue(key, valueMap[key])
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

		var evalErr error

		attrs, evaluatedLocals, evaluated, evalErr = attemptEvaluateLocals(
			parser,
			l,
			file,
			attrs,
			evaluatedLocals,
		)
		if evalErr != nil {
			l.Debugf("Encountered error while evaluating locals in file %s", opts.TerragruntStackConfigPath)

			return errors.New(evalErr)
		}
	}

	localsAsCtyVal, err := ConvertValuesMapToCtyVal(evaluatedLocals)
	if err != nil {
		return errors.New(err)
	}

	parser.Locals = &localsAsCtyVal

	return nil
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

// CleanStacks removes stack directories within the specified working directory, unless the command is "destroy".
// It returns an error if any issues occur during the deletion process, or nil if successful.
func CleanStacks(_ context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if opts.TerraformCommand == tf.CommandNameDestroy {
		l.Debugf("Skipping stack clean for %s, as part of delete command", opts.WorkingDir)
		return nil
	}

	errs := &errors.MultiError{}

	walkFn := func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			l.Warnf("Error accessing path %s: %v", path, walkErr)

			errs = errs.Append(walkErr)

			return nil
		}

		if d.IsDir() && d.Name() == StackDir {
			relPath, relErr := filepath.Rel(opts.WorkingDir, path)
			if relErr != nil {
				relPath = path // fallback to absolute if error
			}

			l.Infof("Deleting stack directory: %s", relPath)

			if rmErr := os.RemoveAll(path); rmErr != nil {
				l.Errorf("Failed to delete stack directory %s: %v", relPath, rmErr)

				errs = errs.Append(rmErr)
			}

			return filepath.SkipDir
		}

		return nil
	}
	if walkErr := filepath.WalkDir(opts.WorkingDir, walkFn); walkErr != nil {
		errs = errs.Append(walkErr)
	}

	return errs.ErrorOrNil()
}

// GetUnitDir returns the directory path for a unit based on its no_dot_terragrunt_stack setting.
func GetUnitDir(dir string, unit *Unit) string {
	if unit.NoStack != nil && *unit.NoStack {
		return filepath.Join(dir, unit.Path)
	}

	return filepath.Join(dir, StackDir, unit.Path)
}
