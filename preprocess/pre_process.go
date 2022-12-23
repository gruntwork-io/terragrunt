package preprocess

import (
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/graph"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"os"
	"path/filepath"
)

const ProcessHelp = `
   Usage: terragrunt process <OUTPUT_DIR> [OPTIONS]

   Description:
     Pre-process the Terraform code in the current working directory and write the results to OUTPUT_DIR.
   
   Arguments:
     OUTPUT_DIR: The directory where to write the pre-processed results.

   Options:
     TODO
`

const envsDirName = "envs"

func RunProcess(terragruntOptions *options.TerragruntOptions) error {
	// First arg should be "process"; second should be output dir
	if len(terragruntOptions.TerraformCliArgs) != 2 {
		return errors.WithStackTrace(WrongNumberOfArguments{command: "process", expected: 1, actual: len(terragruntOptions.TerraformCliArgs) - 1, usage: ProcessHelp})
	}

	outputDir := terragruntOptions.TerraformCliArgs[1]

	envNames, err := getEnvNames(terragruntOptions)
	if err != nil {
		return err
	}

	dependencyGraph, err := buildDependencyGraph(terragruntOptions)
	if err != nil {
		return err
	}

	if len(envNames) > 0 {
		for _, envName := range envNames {
			if err := createEnv(outputDir, &envName, dependencyGraph, terragruntOptions); err != nil {
				return err
			}
		}

		return nil
	} else {
		return createEnv(outputDir, nil, dependencyGraph, terragruntOptions)
	}
}

func getEnvNames(terragruntOptions *options.TerragruntOptions) ([]string, error) {
	envNames := []string{}

	envsDir := filepath.Join(terragruntOptions.WorkingDir, envsDirName)
	if !util.IsDir(envsDir) {
		return envNames, nil
	}

	envDirEntries, err := os.ReadDir(envsDir)
	if err != nil {
		return envNames, errors.WithStackTrace(err)
	}

	for _, envDirEntry := range envDirEntries {
		if envDirEntry.IsDir() {
			envNames = append(envNames, envDirEntry.Name())
		}
	}

	return envNames, nil
}

func buildDependencyGraph(terragruntOptions *options.TerragruntOptions) (*graph.TerraformGraph, error) {
	return graph.GetParsedTerraformGraph(terragruntOptions.WorkingDir)
}
