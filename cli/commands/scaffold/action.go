package scaffold

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	boilerplate_options "github.com/gruntwork-io/boilerplate/options"
	"github.com/gruntwork-io/boilerplate/templates"
	"github.com/gruntwork-io/boilerplate/variables"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/go-getter"
)

func Run(opts *options.TerragruntOptions) error {
	// download remote repo to local
	moduleUrl := ""
	templateUrl := ""
	if len(opts.TerraformCliArgs) >= 2 {
		moduleUrl = opts.TerraformCliArgs[1]
	}

	if len(opts.TerraformCliArgs) >= 3 {
		templateUrl = opts.TerraformCliArgs[2]
	}

	tempDir, err := ioutil.TempDir("", "scaffold")
	if err != nil {
		return errors.WithStackTrace(err)
	}

	opts.Logger.Infof("Scaffolding a new Terragrunt module %s %s to %s", moduleUrl, templateUrl, opts.WorkingDir)

	err = getter.GetAny(tempDir, moduleUrl)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	err = files.CopyFolderContents(tempDir, opts.WorkingDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	listVariables(opts.WorkingDir)

	// run boilerplate
	opts.Logger.Infof("Running boilerplate in %s", opts.WorkingDir)

	bopts := &boilerplate_options.BoilerplateOptions{
		TemplateFolder:  fmt.Sprintf("%s/boilerplate", opts.WorkingDir),
		OutputFolder:    opts.WorkingDir,
		OnMissingKey:    boilerplate_options.DefaultMissingKeyAction,
		OnMissingConfig: boilerplate_options.DefaultMissingConfigAction,
	}

	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(bopts, bopts, emptyDep)
	if err != nil {
		return err
	}

	return nil
}

func listVariables(directoryPath string) ([]string, error) {
	tfFiles, err := listTerraformFiles(directoryPath)
	if err != nil {
		return nil, err
	}
	// Create an HCL parser
	parser := hclparse.NewParser()

	// Extract variables from all TF files
	var allVariables []string
	for _, tfFile := range tfFiles {
		// Read the content of the TF file
		content, err := ioutil.ReadFile(tfFile)
		if err != nil {
			log.Printf("Error reading file %s: %v", tfFile, err)
			continue
		}

		// Parse the HCL content
		file, diags := parser.ParseHCL(content, tfFile)
		if diags.HasErrors() {
			log.Printf("Failed to parse HCL in file %s: %v", tfFile, diags)
			continue
		}
		if body, ok := file.Body.(*hclsyntax.Body); ok {
			fmt.Sprintf("%v", body)
			for _, block := range body.Blocks {
				if block.Type == "variable" {
					if len(block.Labels[0]) > 0 {
						allVariables = append(allVariables, block.Labels[0])
					}
				}
			}
		}
	}

	fmt.Printf("%v", allVariables)

	return allVariables, nil
}

// listTerraformFiles returns a list of all TF files in the specified directory.
func listTerraformFiles(directoryPath string) ([]string, error) {
	var tfFiles []string

	err := filepath.Walk(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".tf" {
			tfFiles = append(tfFiles, path)
		}
		return nil
	})

	return tfFiles, err
}
