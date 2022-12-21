package cli

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/graph"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"os"
	"path/filepath"
	"strings"
)
import "github.com/hashicorp/hcl/v2/hclwrite"

const processHelp = `
   Usage: terragrunt process <OUTPUT_DIR> [OPTIONS]

   Description:
     Pre-process the Terraform code in the current working directory and write the results to OUTPUT_DIR.
   
   Arguments:
     OUTPUT_DIR: The directory where to write the pre-processed results.

   Options:
     TODO
`

const envsDirName = "envs"

const manifestName = ".tgmanifest"

func runProcess(terragruntOptions *options.TerragruntOptions) error {
	// First arg should be "process"; second should be output dir
	if len(terragruntOptions.TerraformCliArgs) != 2 {
		return fmt.Errorf("Unexpected number of arguments. Usage: %s", processHelp)
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

// TerraformFiles is a map from file path to the parsed HCL
type TerraformFiles map[string]*hclwrite.File

func parseAllTerraformFilesInDir(dir string) (TerraformFiles, error) {
	out := map[string]*hclwrite.File{}

	terraformFiles, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return out, errors.WithStackTrace(err)
	}

	for _, terraformFile := range terraformFiles {
		bytes, err := os.ReadFile(terraformFile)
		if err != nil {
			return out, errors.WithStackTrace(err)
		}

		parsedFile, diags := hclwrite.ParseConfig(bytes, terraformFile, hcl.InitialPos)
		if diags.HasErrors() {
			return out, errors.WithStackTrace(diags)
		}

		out[terraformFile] = parsedFile
	}

	return out, nil
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

func createModule(moduleName string, otherModuleNames []string, outPath string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	modulePath := filepath.Join(outPath, moduleName)
	terragruntOptions.Logger.Debugf("Creating module: %s", modulePath)

	if err := copyOriginalModuleFromWorkingDir(modulePath, terragruntOptions); err != nil {
		return err
	}

	if envName != nil {
		if err := copyEnvContents(modulePath, *envName, terragruntOptions); err != nil {
			return err
		}
	}

	// Since we copied all the contents over, we parse them again, and will modify the copies
	parsedTerraformFiles, err := parseAllTerraformFilesInDir(modulePath)
	if err != nil {
		return err
	}

	if err := replaceOtherModulesWithDataSources(parsedTerraformFiles, moduleName, envName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := replaceReferencesToOtherModules(parsedTerraformFiles, moduleName, otherModuleNames, envName, terragruntOptions); err != nil {
		return err
	}

	// TODO: update backend config in each module

	// TODO: write the files back out to disk
	// TODO: format the code
	for _, parsedFile := range parsedTerraformFiles {
		terragruntOptions.Logger.Infof("FILE CONTENTS:\n\n%s\n\n", string(parsedFile.BuildTokens(nil).Bytes()))
	}

	return nil
}

func buildDependencyGraph(terragruntOptions *options.TerragruntOptions) (*graph.TerraformGraph, error) {
	return graph.GetParsedTerraformGraph(terragruntOptions.WorkingDir)
}

func buildDependencyGraph2(parsedTerraformFiles TerraformFiles, moduleName string, otherModuleNames []string, terragruntOptions *options.TerragruntOptions) error {
	for _, parsedFile := range parsedTerraformFiles {
		if err := buildDependencyGraphForBlocks(parsedFile.Body().Blocks(), moduleName, otherModuleNames, terragruntOptions, 0); err != nil {
			return err
		}

		if err := buildDependencyGraphForAttrs(parsedFile.Body().Attributes(), moduleName, otherModuleNames, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

func buildDependencyGraphForBlocks(blocks []*hclwrite.Block, moduleName string, otherModuleNames []string, terragruntOptions *options.TerragruntOptions, depthSearchedSoFar int) error {
	// A simple mechanism to avoid infinite loops where we call replaceReferencesToOtherModulesInBlocks recursively
	// over and over indefinitely. There shouldn't be any way to create loops in real HCL code (blocks that contain
	// blocks that somehow loop around to contain their parents), but it's useful to have this as an extra sanity check
	// in case the HCL parser has a bug, or someone creates an artificial AST with loops in it.
	if depthSearchedSoFar > 100 {
		return fmt.Errorf("Hit more than %d nested levels of blocks. Is there any infinite loop somewhere?", depthSearchedSoFar)
	}

	/*
	  We care about:

	  One module named "foo"

	  Other modules:
	    - If "foo" depends on them, replace them with terraform_remote_state
	    - Otherwise, remove

	  Input and local variables:
	    - If "foo" depends on them, or output vars depend on them and those output vars depend on "foo", keep them
	    - Otherwise, remove

	  Output variables:
	    - If they depend on "foo", keep them
	    - Otherwise, remove

	  Data sources:
	    - If "foo" depends on them, keep them
	    - Otherwise, remove

	  Resources:
	    - For now, just error out

	  Provider and terraform blocks:
	    - Keep all
	*/

	/*
			To build a dependency graph:

		    Identify the top-level items:
		      - module.xxx, var.xxx, output.xxx, data.xxx, <RESOURCE>.xxx, provider.xxx, terraform, locals
		      - Go into locals blocks and use attrs to idenfity top-level local vars
		    Go into every attr everywhere within the top-level items and cross-link at the top level:
		      - E.g., module.xxx depends on module.yyy and local.zzz
		    Create a method to check if x depends on y: depends_on(x, y):
		      - Check for a direct cross-link from x to y;
		      - If not found, follow everything x depends on with depth-first or breadth-first search to see if you end up at y


		    Alternative:

		    Run 'terraform graph
	*/

	for _, block := range blocks {
		if err := buildDependencyGraphForBlocks(block.Body().Blocks(), moduleName, otherModuleNames, terragruntOptions, depthSearchedSoFar+1); err != nil {
			return err
		}

		if err := buildDependencyGraphForAttrs(block.Body().Attributes(), moduleName, otherModuleNames, terragruntOptions); err != nil {
			return err
		}
	}

	return nil
}

func buildDependencyGraphForAttrs(attrs map[string]*hclwrite.Attribute, moduleName string, otherModuleNames []string, terragruntOptions *options.TerragruntOptions) error {
	for attrName, attr := range attrs {
		terragruntOptions.Logger.Infof("For attr %s:", attrName)
		// attr.Expr().Variables() tells us EVERYTHING this expression depends on: every input variable, local variable,
		// resource, data source, etc.
		for _, variable := range attr.Expr().Variables() {
			terragruntOptions.Logger.Infof("Variable: %v", string(variable.BuildTokens(nil).Bytes()))
		}
	}

	return nil
}

// Replace all modules other than the current module with terraform_remote_state data sources if the current module
// depends on them, or remove the other module entirely otherwise.
func replaceOtherModulesWithDataSources(parsedTerraformFiles TerraformFiles, moduleName string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, parsedFile := range parsedTerraformFiles {
		for _, block := range parsedFile.Body().Blocks() {
			if block.Type() != "module" || len(block.Labels()) != 1 {
				continue
			}

			otherModuleName := block.Labels()[0]
			if moduleName == otherModuleName {
				continue
			}

			dependsOn, err := dependencyGraph.DoesModuleDependOnModule(moduleName, otherModuleName)
			if err != nil {
				return err
			}

			if dependsOn {
				if err := replaceBlockWithDataSource(block, otherModuleName); err != nil {
					return err
				}
			} else {
				parsedFile.Body().RemoveBlock(block)
			}
		}
	}

	return nil
}

func replaceBlockWithDataSource(block *hclwrite.Block, blockName string) error {
	block.SetType("data")
	block.SetLabels([]string{"terraform_remote_state", blockName})

	// TODO: read backend settings of original module and set them accordingly here
	block.Body().Clear()
	block.Body().AppendNewline()
	block.Body().SetAttributeValue("backend", cty.StringVal("local"))

	return nil
}

// Replace all references to modules other than the current module with references to terraform_remote_state data sources
func replaceReferencesToOtherModules(parsedTerraformFiles TerraformFiles, moduleName string, otherModuleNames []string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	for _, parsedFile := range parsedTerraformFiles {
		if err := replaceReferencesToOtherModulesInBlocks(parsedFile.Body().Blocks(), moduleName, otherModuleNames, 0); err != nil {
			return err
		}

		if err := replaceReferencesToOtherModulesInAttributes(parsedFile.Body().Attributes(), otherModuleNames); err != nil {
			return err
		}
	}

	return nil
}

func replaceReferencesToOtherModulesInBlocks(blocks []*hclwrite.Block, moduleName string, otherModuleNames []string, depthSearchedSoFar int) error {
	// A simple mechanism to avoid infinite loops where we call replaceReferencesToOtherModulesInBlocks recursively
	// over and over indefinitely. There shouldn't be any way to create loops in real HCL code (blocks that contain
	// blocks that somehow loop around to contain their parents), but it's useful to have this as an extra sanity check
	// in case the HCL parser has a bug, or someone creates an artificial AST with loops in it.
	if depthSearchedSoFar > 100 {
		return fmt.Errorf("Hit more than %d nested levels of blocks. Is there any infinite loop somewhere?", depthSearchedSoFar)
	}

	for _, block := range blocks {
		if err := replaceReferencesToOtherModulesInBlocks(block.Body().Blocks(), moduleName, otherModuleNames, depthSearchedSoFar+1); err != nil {
			return err
		}

		if err := replaceReferencesToOtherModulesInAttributes(block.Body().Attributes(), otherModuleNames); err != nil {
			return err
		}
	}

	return nil
}

func replaceReferencesToOtherModulesInAttributes(attributes map[string]*hclwrite.Attribute, otherModuleNames []string) error {
	for _, attr := range attributes {
		for _, otherModuleName := range otherModuleNames {
			attr.Expr().RenameVariablePrefix([]string{"module", otherModuleName}, []string{"data", fmt.Sprintf("terraform_remote_state.%s.outputs.__module__", otherModuleName)})
		}
	}

	return nil
}

func copyOriginalModuleFromWorkingDir(modulePath string, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Copying contents from %s to %s", terragruntOptions.WorkingDir, modulePath)
	return util.CopyFolderContentsWithFilterImpl(terragruntOptions.WorkingDir, modulePath, nil, false, preprocessorFileCopyFilter)
}

func copyEnvContents(modulePath string, envName string, terragruntOptions *options.TerragruntOptions) error {
	envPath := filepath.Join(terragruntOptions.WorkingDir, envsDirName, envName)
	terragruntOptions.Logger.Debugf("Copying contents from %s to %s", envPath, modulePath)
	return util.CopyFolderContentsWithFilterImpl(envPath, modulePath, nil, false, preprocessorFileCopyFilter)
}

// preprocessorFileCopyFilter is a filter that can be used with util.CopyFolderContentsWithFilter to exclude hidden
// files & folders and state files
func preprocessorFileCopyFilter(absolutePath string) bool {
	return !util.TerragruntExcludes(absolutePath) && !strings.HasSuffix(absolutePath, ".tfstate") && !strings.HasSuffix(absolutePath, ".tfstate.backup")
}

func doStuff(terragruntOptions *options.TerragruntOptions) error {
	filename := "main.tf"
	bytes, err := os.ReadFile(filepath.Join("/Users/brikis98/src/terragrunt/test/fixture-preprocessor/before", filename))
	if err != nil {
		return errors.WithStackTrace(err)
	}

	parsedFile, diags := hclwrite.ParseConfig(bytes, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return errors.WithStackTrace(err)
	}

	for _, block := range parsedFile.Body().Blocks() {
		terragruntOptions.Logger.Infof("Block: type = %s, labels = %v", block.Type(), block.Labels())
		for _, attr := range block.Body().Attributes() {
			terragruntOptions.Logger.Infof("Attribute before: %v", string(attr.BuildTokens(nil).Bytes()))
			terragruntOptions.Logger.Infof("Expr before: %v", string(attr.Expr().BuildTokens(nil).Bytes()))

			attr.Expr().RenameVariablePrefix([]string{"module", "vpc"}, []string{"data", "terraform_remote_state.vpc.outputs.__module__."})

			terragruntOptions.Logger.Infof("Attribute after: %v", string(attr.BuildTokens(nil).Bytes()))
			terragruntOptions.Logger.Infof("Expr after: %v", string(attr.Expr().BuildTokens(nil).Bytes()))
		}
	}

	return nil
}
