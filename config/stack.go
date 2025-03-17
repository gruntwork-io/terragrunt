package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	stackDir           = ".terragrunt-stack"
	valuesFile         = "terragrunt.values.hcl"
	manifestName       = ".terragrunt-stack-manifest"
	defaultStackFile   = "terragrunt.stack.hcl"
	unitDirPerm        = 0755
	valueFilePerm      = 0644
	generationMaxDepth = 100
)

// StackConfigFile represents the structure of terragrunt.stack.hcl stack file.
type StackConfigFile struct {
	Locals *terragruntLocal `hcl:"locals,block"`
	Stacks []*Stack         `hcl:"stack,block"`
	Units  []*Unit          `hcl:"unit,block"`
}

// Unit represent unit from stack file.
type Unit struct {
	Name    string     `hcl:",label"`
	Source  string     `hcl:"source,attr"`
	Path    string     `hcl:"path,attr"`
	NoStack *bool      `hcl:"no_dot_terragrunt_stack,attr"`
	Values  *cty.Value `hcl:"values,attr"`
}

// Stack represents the stack block in the configuration.
type Stack struct {
	Name    string     `hcl:",label"`
	Source  string     `hcl:"source,attr"`
	Path    string     `hcl:"path,attr"`
	NoStack *bool      `hcl:"no_dot_terragrunt_stack,attr"`
	Values  *cty.Value `hcl:"values,attr"`
}

// GenerateStacks generates the stack files.
func GenerateStacks(ctx context.Context, opts *options.TerragruntOptions) error {
	processedFiles := make(map[string]bool)
	wp := util.NewWorkerPool(opts.Parallelism)
	// stop worker pool on exit
	defer wp.Stop()
	// initial files setting as stack file
	foundFiles := []string{opts.TerragruntStackConfigPath}

	for {
		// check if we have already processed the files
		processedNewFiles := false

		for _, file := range foundFiles {
			if processedFiles[file] {
				continue
			}

			processedNewFiles = true
			processedFiles[file] = true

			if err := generateStackFile(ctx, opts, wp, file); err != nil {
				return errors.Errorf("Failed to process stack file %s %v", file, err)
			}
		}

		if err := wp.Wait(); err != nil {
			return err
		}

		if !processedNewFiles {
			break
		}

		newFiles, err := listStackFiles(opts, opts.WorkingDir)

		if err != nil {
			return errors.Errorf("Failed to list stack files %v", err)
		}

		foundFiles = newFiles
	}

	return nil
}

// StackOutput generates the output from the stack files.
func StackOutput(ctx context.Context, opts *options.TerragruntOptions) (map[string]map[string]cty.Value, error) {
	opts.Logger.Debugf("Generating output from %s", opts.TerragruntStackConfigPath)
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	stackTargetDir := filepath.Join(opts.WorkingDir, stackDir)
	stackFiles, err := listStackFiles(opts, stackTargetDir)

	if err != nil {
		return nil, errors.Errorf("Failed to list stack files in %s %v", stackTargetDir, err)
	}

	unitOutputs := make(map[string]map[string]cty.Value)

	if util.FileExists(opts.TerragruntStackConfigPath) {
		// add default stack file if exists
		stackFiles = append(stackFiles, opts.TerragruntStackConfigPath)
	}

	for _, path := range stackFiles {
		// read stack values file
		dir := filepath.Dir(path)
		values, err := ReadValues(ctx, opts, dir)

		if err != nil {
			return nil, errors.New(err)
		}

		stackFile, err := ReadStackConfigFile(ctx, opts, path, values)

		if err != nil {
			return nil, errors.New(err)
		}

		// process each unit and get outputs
		for _, unit := range stackFile.Units {
			opts.Logger.Debugf("Processing unit %s", unit.Name)

			dir := filepath.Dir(path)
			unitDir := filepath.Join(dir, stackDir, unit.Path)
			output, err := unit.ReadOutputs(ctx, opts, unitDir)

			if err != nil {
				return nil, errors.New(err)
			}

			unitOutputs[unit.Name] = output
		}
	}

	return unitOutputs, nil
}

// generateStackFile processes the Terragrunt stack configuration from the given stackFilePath,
// reads necessary values, and generates units and stacks in the target directory.
// It handles the creation of required directories and returns any errors encountered.
func generateStackFile(ctx context.Context, opts *options.TerragruntOptions, pool *util.WorkerPool, stackFilePath string) error {
	stackSourceDir := filepath.Dir(stackFilePath)

	values, err := ReadValues(ctx, opts, stackSourceDir)
	if err != nil {
		return errors.Errorf("failed to read values from directory %s: %v", stackSourceDir, err)
	}

	stackFile, err := ReadStackConfigFile(ctx, opts, stackFilePath, values)

	if err != nil {
		return errors.Errorf("Failed to read stack file %s in %s %v", stackFilePath, stackSourceDir, err)
	}

	stackTargetDir := filepath.Join(stackSourceDir, stackDir)

	if err := generateUnits(ctx, opts, pool, stackSourceDir, stackTargetDir, stackFile.Units); err != nil {
		return err
	}

	if err := generateStacks(ctx, opts, pool, stackSourceDir, stackTargetDir, stackFile.Stacks); err != nil {
		return err
	}

	return nil
}

// generateUnits iterates through a slice of Unit objects, processing each one by copying
// source files to their destination paths and writing unit-specific values.
// It logs the processing progress and returns any errors encountered during the operation.
func generateUnits(ctx context.Context, opts *options.TerragruntOptions, pool *util.WorkerPool, sourceDir, targetDir string, units []*Unit) error {
	for _, unit := range units {
		unitCopy := unit // Create a copy to avoid capturing the loop variable reference

		pool.Submit(func() error {
			item := componentToProcess{
				sourceDir: sourceDir,
				targetDir: targetDir,
				name:      unitCopy.Name,
				path:      unitCopy.Path,
				source:    unitCopy.Source,
				values:    unitCopy.Values,
				noStack:   unitCopy.NoStack != nil && *unitCopy.NoStack,
			}

			opts.Logger.Infof("Processing unit %s", unitCopy.Name)

			if err := processComponent(ctx, opts, &item); err != nil {
				return err
			}

			return nil
		})
	}

	return nil
}

// generateStacks processes each stack by resolving its destination path and copying files from the source.
// It logs each operation and returns early if any error is encountered.
func generateStacks(ctx context.Context, opts *options.TerragruntOptions, pool *util.WorkerPool, sourceDir, targetDir string, stacks []*Stack) error {
	for _, stack := range stacks {
		stackCopy := stack // Create a copy to avoid capturing the loop variable reference

		pool.Submit(func() error {
			item := componentToProcess{
				sourceDir: sourceDir,
				targetDir: targetDir,
				name:      stackCopy.Name,
				path:      stackCopy.Path,
				source:    stackCopy.Source,
				noStack:   stackCopy.NoStack != nil && *stackCopy.NoStack,
				values:    stackCopy.Values,
			}

			opts.Logger.Infof("Processing stack %s", stackCopy.Name)

			if err := processComponent(ctx, opts, &item); err != nil {
				return err
			}

			return nil
		})
	}

	return nil
}

// componentToProcess represents an item of work for processing a stack or unit.
// It contains information about the source and target directories, the name and path of the item, the source URL or path,
// and any associated values that need to be processed.
type componentToProcess struct {
	sourceDir string
	targetDir string
	name      string
	path      string
	source    string
	noStack   bool
	values    *cty.Value
}

// processComponent copies files from the source directory to the target destination and generates a corresponding values file.
func processComponent(ctx context.Context, opts *options.TerragruntOptions, cmp *componentToProcess) error {
	source := cmp.source
	// Adjust source path using the provided source mapping configuration if available
	source, err := adjustSourceWithMap(opts.SourceMap, source, opts.TerragruntStackConfigPath)

	if err != nil {
		return errors.Errorf("failed to adjust source %s: %v", cmp.source, err)
	}

	dest := cmp.path
	// if destination is not an absolute path, join with target directory
	if !filepath.IsAbs(cmp.path) {
		dest = filepath.Join(cmp.targetDir, cmp.path)
	}

	if cmp.noStack {
		// for noStack components, we copy the files to the base directory of the target directory
		dest = filepath.Join(filepath.Dir(cmp.targetDir), cmp.path)
	}

	opts.Logger.Debugf("Processing: %s (%s) to %s", cmp.name, source, dest)

	if err := copyFiles(ctx, opts, cmp.name, cmp.sourceDir, source, dest); err != nil {
		return err
	}

	// generate values file
	if err := writeValues(opts, cmp.values, dest); err != nil {
		return errors.Errorf("failed to write values %v %v", cmp.name, err)
	}

	return nil
}

// copyFiles copies files or directories from a source to a destination path.
//
// The function checks if the source is local or remote. If local, it copies the
// contents of the source directory to the destination. If remote, it fetches the
// source and stores it in the destination directory.
func copyFiles(ctx context.Context, opts *options.TerragruntOptions, identifier, sourceDir, src, dest string) error {
	if isLocal(opts, sourceDir, src) {
		// check if src is absolute path, if not, join with sourceDir
		var localSrc string

		if filepath.IsAbs(src) {
			localSrc = src
		} else {
			localSrc = filepath.Join(sourceDir, src)
		}

		localSrc, err := filepath.Abs(localSrc)

		if err != nil {
			opts.Logger.Warnf("failed to get absolute path for source '%s': %v", identifier, err)
			// fallback to original source
			localSrc = src
		}

		if err := util.CopyFolderContentsWithFilter(opts.Logger, localSrc, dest, manifestName, func(absolutePath string) bool {
			return true
		}); err != nil {
			return errors.Errorf("Failed to copy %s to %s %v", localSrc, dest, err)
		}
	} else {
		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			return errors.Errorf("Failed to create directory %s for %s %v", dest, identifier, err)
		}

		if _, err := getter.GetAny(ctx, dest, src); err != nil {
			return errors.Errorf("Failed to fetch %s %v", identifier, err)
		}
	}

	return nil
}

// isLocal determines if a given source path is local or remote.
//
// It checks if the provided source file exists locally. If not, it checks if
// the path is relative to the working directory. If that also fails, the function
// attempts to detect the source's getter type and recognizes if it is a file URL.
func isLocal(opts *options.TerragruntOptions, workingDir, src string) bool {
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
			opts.Logger.Debugf("Error detecting getter for %s: %v", src, err)
			continue
		}

		if recognized {
			break
		}
	}

	return strings.HasPrefix(req.Src, "file://")
}

// ReadOutputs retrieves the Terraform output JSON for this unit, converts it into a map of cty.Values,
// and logs the operation for debugging. It returns early in case of any errors during retrieval or conversion.
func (u *Unit) ReadOutputs(ctx context.Context, opts *options.TerragruntOptions, unitDir string) (map[string]cty.Value, error) {
	configPath := filepath.Join(unitDir, DefaultTerragruntConfigPath)
	opts.Logger.Debugf("Getting output from unit %s in %s", u.Name, unitDir)

	parserCtx := NewParsingContext(ctx, opts)

	jsonBytes, err := getOutputJSONWithCaching(parserCtx, configPath) //nolint: contextcheck

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
// It creates a parsing context, processes locals, and decodes the file into a StackConfigFile struct.
// Validation is performed on the resulting config, and any encountered errors cause an early return.
func ReadStackConfigFile(ctx context.Context, opts *options.TerragruntOptions, filePath string, values *cty.Value) (*StackConfigFile, error) {
	opts.Logger.Debugf("Reading Terragrunt stack config file at %s", filePath)

	parser := NewParsingContext(ctx, opts)

	if values != nil {
		parser = parser.WithValues(values)
	}

	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(filePath)
	if err != nil {
		return nil, errors.New(err)
	}

	//nolint:contextcheck
	if err := processLocals(parser, opts, file); err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	evalParsingContext, err := createTerragruntEvalContext(parser, file.ConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}

	config := &StackConfigFile{}
	if err := file.Decode(config, evalParsingContext); err != nil {
		return nil, errors.New(err)
	}

	if err := ValidateStackConfig(config); err != nil {
		return nil, errors.New(err)
	}

	return config, nil
}

// writeValues generates and writes values to a terragrunt.values.hcl file in the specified directory.
func writeValues(opts *options.TerragruntOptions, values *cty.Value, directory string) error {
	if values == nil {
		opts.Logger.Debugf("No values to write in %s", directory)
		return nil
	}

	if directory == "" {
		return errors.New("writeValues: unit directory path cannot be empty")
	}

	if err := os.MkdirAll(directory, unitDirPerm); err != nil {
		return errors.Errorf("failed to create directory %s: %w", directory, err)
	}

	opts.Logger.Debugf("Writing values file in %s", directory)
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
func ReadValues(ctx context.Context, opts *options.TerragruntOptions, directory string) (*cty.Value, error) {
	if directory == "" {
		return nil, errors.New("ReadValues: directory path cannot be empty")
	}

	filePath := filepath.Join(directory, valuesFile)

	if util.FileNotExists(filePath) {
		return nil, nil
	}

	opts.Logger.Debugf("Reading Terragrunt stack values file at %s", filePath)
	parser := NewParsingContext(ctx, opts)
	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(filePath)

	if err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	evalParsingContext, err := createTerragruntEvalContext(parser, file.ConfigPath)

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
func processLocals(parser *ParsingContext, opts *options.TerragruntOptions, file *hclparse.File) error {
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
			file,
			attrs,
			evaluatedLocals,
		)

		if err != nil {
			opts.Logger.Debugf("Encountered error while evaluating locals in file %s", opts.TerragruntStackConfigPath)

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
func listStackFiles(opts *options.TerragruntOptions, dir string) ([]string, error) {
	walkWithSymlinks := opts.Experiments.Evaluate(experiment.Symlinks)
	walkFunc := filepath.Walk

	if walkWithSymlinks {
		walkFunc = util.WalkWithSymlinks
	}

	opts.Logger.Debugf("Searching for stack files in %s", dir)

	var stackFiles []string

	// find all defaultStackFile files
	if err := walkFunc(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			opts.Logger.Warnf("Error accessing path %s: %v", path, err)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(dir, path)
		depth := len(strings.Split(relPath, string(os.PathSeparator)))
		if depth > generationMaxDepth {
			return errors.Errorf("Cycle detected: max depth of %d exceeded at %s", generationMaxDepth, path)
		}

		if strings.HasSuffix(path, defaultStackFile) {
			opts.Logger.Debugf("Found stack file %s", path)
			stackFiles = append(stackFiles, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return stackFiles, nil
}
