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
	unitValuesFile     = "terragrunt.values.hcl"
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
	Name   string     `hcl:",label"`
	Source string     `hcl:"source,attr"`
	Path   string     `hcl:"path,attr"`
	Values *cty.Value `hcl:"values,attr"`
}

// Stack represents the stack block in the configuration.
type Stack struct {
	Name   string `hcl:",label"`
	Source string `hcl:"source,attr"`
	Path   string `hcl:"path,attr"`
}

// GenerateStacks generates the stack files.
func GenerateStacks(ctx context.Context, opts *options.TerragruntOptions) error {
	processedFiles := make(map[string]bool)
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

			if err := generateStackFile(ctx, opts, file); err != nil {
				return errors.Errorf("Failed to process stack file %s %v", file, err)
			}
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
		stackFile, err := ReadStackConfigFile(ctx, opts, path)

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

// generateStackFile process single stack file.
func generateStackFile(ctx context.Context, opts *options.TerragruntOptions, stackFilePath string) error {
	stackSourceDir := filepath.Dir(stackFilePath)
	stackFile, err := ReadStackConfigFile(ctx, opts, stackFilePath)

	if err != nil {
		return errors.Errorf("Failed to read stack file %s in %s %v", stackFilePath, stackSourceDir, err)
	}

	stackTargetDir := filepath.Join(stackSourceDir, stackDir)

	if err := os.MkdirAll(stackTargetDir, os.ModePerm); err != nil {
		return errors.Errorf("failed to create base directory: %s %v", stackTargetDir, err)
	}

	if err := generateUnits(ctx, opts, stackSourceDir, stackTargetDir, stackFile.Units); err != nil {
		return err
	}

	if err := generateStacks(ctx, opts, stackSourceDir, stackTargetDir, stackFile.Stacks); err != nil {
		return err
	}

	return nil
}

// generateUnits processes each unit by resolving its destination path and copying files from the source.
// It then writes the unit's values file and logs any errors encountered.
// In case of an error, the function exits early.
func generateUnits(ctx context.Context, opts *options.TerragruntOptions, stackSourceDir, stackTargetDir string, units []*Unit) error {
	for _, unit := range units {
		opts.Logger.Infof("Processing unit %s", unit.Name)

		destPath := filepath.Join(stackTargetDir, unit.Path)
		dest, err := filepath.Abs(destPath)

		if err != nil {
			return errors.Errorf("failed to get absolute path for destination '%s': %v", dest, err)
		}

		src := unit.Source
		opts.Logger.Debugf("Processing unit: %s (%s) to %s", unit.Name, src, dest)

		if err := copyFiles(ctx, opts, unit.Name, stackSourceDir, src, dest); err != nil {
			return err
		}

		// generate unit values file
		if err := writeUnitValues(opts, unit, dest); err != nil {
			return errors.Errorf("Failed to write unit values %v %v", unit.Name, err)
		}
	}

	return nil
}

// generateStacks processes each stack by resolving its destination path and copying files from the source.
// It logs each operation and returns early if any error is encountered.
func generateStacks(ctx context.Context, opts *options.TerragruntOptions, stackSourceDir, stackTargetDir string, stacks []*Stack) error {
	for _, stack := range stacks {
		opts.Logger.Infof("Processing stack %s", stack.Name)

		destPath := filepath.Join(stackTargetDir, stack.Path)
		dest, err := filepath.Abs(destPath)

		if err != nil {
			return errors.Errorf("Failed to get absolute path for destination '%s': %v", dest, err)
		}

		src := stack.Source
		opts.Logger.Debugf("Processing stack: %s (%s) to %s", stack.Name, src, dest)

		if err := copyFiles(ctx, opts, stack.Name, stackSourceDir, src, dest); err != nil {
			return err
		}
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
func ReadStackConfigFile(ctx context.Context, opts *options.TerragruntOptions, filePath string) (*StackConfigFile, error) {
	opts.Logger.Debugf("Reading Terragrunt stack config file at %s", filePath)

	parser := NewParsingContext(ctx, opts)

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

// writeUnitValues generates and writes unit values to a terragrunt.values.hcl file in the specified unit directory.
// If the unit has no values (Values is nil), the function logs a debug message and returns.
// Parameters:
//   - opts: TerragruntOptions containing logger and other configuration
//   - unit: Unit containing the values to write
//   - unitDirectory: Target directory where the values file will be created
//
// Returns an error if the directory creation or file writing fails.
func writeUnitValues(opts *options.TerragruntOptions, unit *Unit, unitDirectory string) error {
	if unitDirectory == "" {
		return errors.New("writeUnitValues: unit directory path cannot be empty")
	}

	if err := os.MkdirAll(unitDirectory, unitDirPerm); err != nil {
		return errors.Errorf("failed to create directory %s: %w", unitDirectory, err)
	}

	filePath := filepath.Join(unitDirectory, unitValuesFile)
	if unit.Values == nil {
		opts.Logger.Debugf("No values to write for unit %s in %s", unit.Name, filePath)
		return nil
	}

	file := hclwrite.NewEmptyFile()
	body := file.Body()
	body.AppendUnstructuredTokens([]*hclwrite.Token{
		{
			Type:  hclsyntax.TokenComment,
			Bytes: []byte("# Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually\n"),
		},
	})

	for key, val := range unit.Values.AsValueMap() {
		body.SetAttributeValue(key, val)
	}

	if err := os.WriteFile(filePath, file.Bytes(), valueFilePerm); err != nil {
		return errors.Errorf("failed to write values file %s: %w", filePath, err)
	}

	return nil
}

// ReadUnitValues reads the unit values from the terragrunt.values.hcl file.
func ReadUnitValues(ctx context.Context, opts *options.TerragruntOptions, unitDirectory string) (*cty.Value, error) {
	if unitDirectory == "" {
		return nil, errors.New("ReadUnitValues: unit directory path cannot be empty")
	}

	filePath := filepath.Join(unitDirectory, unitValuesFile)

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
			if info.IsDir() {
				opts.Logger.Warnf("Skipping directory %s: max depth of %d exceeded", path, generationMaxDepth)
			} else {
				opts.Logger.Warnf("Skipping file %s: max depth of %d exceeded", path, generationMaxDepth)
			}
			return nil
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
