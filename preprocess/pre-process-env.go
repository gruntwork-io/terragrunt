package preprocess

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/graph"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"path/filepath"
)

func createEnv(outputDir string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	outPath := outputDir
	if envName != nil {
		outPath = filepath.Join(outputDir, *envName)
	}

	terragruntOptions.Logger.Debugf("Creating env folder: %s", outPath)
	if err := util.EnsureDirectory(outPath); err != nil {
		return err
	}

	parsedTerraformFiles, err := parseAllTerraformFilesInDir(terragruntOptions.WorkingDir)
	if err != nil {
		return err
	}

	moduleNames, err := extractModuleNames(parsedTerraformFiles)
	if err != nil {
		return err
	}

	terragruntOptions.Logger.Debugf("Found the following modules in %s: %v", terragruntOptions.WorkingDir, moduleNames)
	for _, moduleName := range moduleNames {
		if err := createModule(moduleName, util.RemoveElementFromList(moduleNames, moduleName), outPath, envName, dependencyGraph, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

func extractModuleNames(parsedTerraformFiles TerraformFiles) ([]string, error) {
	moduleNames := []string{}

	for path, parsedFile := range parsedTerraformFiles {
		for _, block := range parsedFile.Body().Blocks() {
			if block.Type() == "module" {
				if len(block.Labels()) != 1 {
					return moduleNames, fmt.Errorf("Found an invalid module block in %s with more than 1 label: %v", path, block.Labels())
				}

				moduleNames = append(moduleNames, block.Labels()[0])
			}
		}
	}

	return moduleNames, nil
}
