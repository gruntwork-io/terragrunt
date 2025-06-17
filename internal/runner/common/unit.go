package common

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

// Unit represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type Unit struct {
	Stack                Stack
	TerragruntOptions    *options.TerragruntOptions
	Logger               log.Logger
	Path                 string
	Dependencies         Units
	Config               config.TerragruntConfig
	AssumeAlreadyApplied bool
	FlagExcluded         bool
}

type Units []*Unit

type UnitsMap map[string]*Unit

// String renders this module as a human-readable string
func (module *Unit) String() string {
	dependencies := []string{}
	for _, dependency := range module.Dependencies {
		dependencies = append(dependencies, dependency.Path)
	}

	return fmt.Sprintf(
		"Module %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		module.Path, module.FlagExcluded, module.AssumeAlreadyApplied, strings.Join(dependencies, ", "),
	)
}

// FlushOutput flushes buffer data to the output writer.
func (module *Unit) FlushOutput() error {
	if writer, ok := module.TerragruntOptions.Writer.(*ModuleWriter); ok {
		module.Stack.Lock()
		defer module.Stack.Unlock()

		return writer.Flush()
	}

	return nil
}

// planFile - return plan file location, if output folder is set
func (module *Unit) planFile(l log.Logger, opts *options.TerragruntOptions) string {
	var planFile string

	// set plan file location if output folder is set
	planFile = module.outputFile(l, opts)

	planCommand := module.TerragruntOptions.TerraformCommand == tf.CommandNamePlan || module.TerragruntOptions.TerraformCommand == tf.CommandNameShow

	// in case if JSON output is enabled, and not specified planFile, save plan in working dir
	if planCommand && planFile == "" && module.TerragruntOptions.JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// outputFile - return plan file location, if output folder is set
func (module *Unit) outputFile(l log.Logger, opts *options.TerragruntOptions) string {
	return module.getPlanFilePath(l, opts, opts.OutputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile - return plan JSON file location, if JSON output folder is set
func (module *Unit) OutputJSONFile(l log.Logger, opts *options.TerragruntOptions) string {
	return module.getPlanFilePath(l, opts, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

func (module *Unit) getPlanFilePath(l log.Logger, opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	path, _ := filepath.Rel(opts.WorkingDir, module.Path)
	dir := filepath.Join(outputFolder, path)

	if !filepath.IsAbs(dir) {
		dir = filepath.Join(opts.WorkingDir, dir)
		if absDir, err := filepath.Abs(dir); err == nil {
			dir = absDir
		} else {
			l.Warnf("Failed to get absolute path for %s: %v", dir, err)
		}
	}

	return filepath.Join(dir, fileName)
}

// findModuleInPath returns true if a module is located under one of the target directories
func (module *Unit) findModuleInPath(targetDirs []string) bool {
	return slices.Contains(targetDirs, module.Path)
}
