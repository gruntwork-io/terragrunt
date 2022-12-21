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

func createModule(currentModuleName string, otherModuleNames []string, outPath string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	modulePath := filepath.Join(outPath, currentModuleName)
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

	// We are going to modify the graph for each module, so clone it so we aren't modifying the original
	dependencyGraphClone := dependencyGraph.Clone()

	if err := processFiles(parsedTerraformFiles, currentModuleName, otherModuleNames, envName, dependencyGraphClone, terragruntOptions); err != nil {
		return err
	}

	for path, parsedFile := range parsedTerraformFiles {
		fileContents := parsedFile.Bytes()
		formattedFileContents := hclwrite.Format(fileContents)

		terragruntOptions.Logger.Debugf("Writing updated contents to %s", path)
		if err := util.WriteFileWithSamePermissions(path, path, formattedFileContents); err != nil {
			return err
		}
	}

	return nil
}

func buildDependencyGraph(terragruntOptions *options.TerragruntOptions) (*graph.TerraformGraph, error) {
	return graph.GetParsedTerraformGraph(terragruntOptions.WorkingDir)
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

func processFiles(parsedTerraformFiles TerraformFiles, currentModuleName string, otherModuleNames []string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	allBlocks := getAllBlocks(parsedTerraformFiles)
	blocksByType := groupBlocksByType(allBlocks)

	// The order of these steps matters! For example, if we remove output variables not relevant to the current
	// module first, then when we go to remove locals, the "does any output variable depend on this local?" check
	// will apply only to the outputs that are relevant, rather than all outputs.

	// TODO: update dependency graph when removing vars, resources, data sources, etc

	backend, err := updateTerraformConfig(blocksByType["terraform"], currentModuleName, otherModuleNames, envName, dependencyGraph, terragruntOptions)
	if err != nil {
		return err
	}

	if err := removeOrReplaceModules(blocksByType["module"], currentModuleName, otherModuleNames, envName, backend, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := replaceReferencesToOtherModulesInBlocks(allBlocks, currentModuleName, otherModuleNames, 0); err != nil {
		return err
	}

	if err := removeUnneededOutputVariables(blocksByType["output"], currentModuleName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := removeUnneededDataSources(blocksByType["data"], currentModuleName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := removeUnneededResources(blocksByType["resource"], currentModuleName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := removeUnneededProviders(blocksByType["provider"], currentModuleName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := removeUnneededLocals(blocksByType["locals"], currentModuleName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	if err := removeUnneededVariables(blocksByType["variable"], currentModuleName, dependencyGraph, terragruntOptions); err != nil {
		return err
	}

	return nil
}

type BlockAndFile struct {
	file  *hclwrite.File
	block *hclwrite.Block
}

func getAllBlocks(parsedTerraformFiles TerraformFiles) []BlockAndFile {
	out := []BlockAndFile{}

	for _, parsedFile := range parsedTerraformFiles {
		out = append(out, getAllBlocksFromBody(parsedFile.Body(), parsedFile)...)
	}

	return out
}

func getAllBlocksFromBody(body *hclwrite.Body, file *hclwrite.File) []BlockAndFile {
	out := []BlockAndFile{}

	for _, block := range body.Blocks() {
		out = append(out, BlockAndFile{file: file, block: block})
	}

	return out
}

func groupBlocksByType(blocks []BlockAndFile) map[string][]BlockAndFile {
	out := map[string][]BlockAndFile{}

	for _, block := range blocks {
		blocksOfType, ok := out[block.block.Type()]
		if !ok {
			blocksOfType = []BlockAndFile{}
		}
		blocksOfType = append(blocksOfType, block)
		out[block.block.Type()] = blocksOfType
	}

	return out
}

func updateTerraformConfig(terraformBlocks []BlockAndFile, currentModuleName string, otherModuleNames []string, envName *string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) (*TerraformBackend, error) {
	for _, terraformBlock := range terraformBlocks {
		for _, nestedBlock := range terraformBlock.block.Body().Blocks() {
			if nestedBlock.Type() == "backend" {
				backend, err := NewTerraformBackend(nestedBlock)
				if err != nil {
					return nil, err
				}

				if err := backend.UpdateConfig(currentModuleName, envName); err != nil {
					return nil, err
				}

				return backend, nil
			}
		}
	}

	return nil, nil
}

type TerraformBackend struct {
	backendType   string
	backendConfig *hclwrite.Body
}

func NewTerraformBackend(block *hclwrite.Block) (*TerraformBackend, error) {
	if len(block.Labels()) != 1 {
		return nil, WrongNumberOfLabels{blockType: block.Type(), expectedLabelCount: 1, actualLabels: block.Labels()}
	}

	return &TerraformBackend{backendType: block.Labels()[0], backendConfig: block.Body()}, nil
}

func (backend *TerraformBackend) UpdateConfig(currentModuleName string, envName *string) error {
	// TODO! Implement this method to update the path/key values in the config accordingly.
	switch backend.backendType {
	case "local":
	case "remote":
	case "azurerm":
	case "consul":
	case "cos":
	case "gcs":
	case "http":
	case "kubernetes":
	case "oss":
	case "pg":
	case "s3":
	}

	return nil
}

func (backend *TerraformBackend) ConfigureDataSource(dataSourceBody *hclwrite.Body) error {
	dataSourceBody.SetAttributeValue("backend", cty.StringVal(backend.backendType))
	dataSourceBody.AppendNewline()

	// TODO: this needs to have the key/path/etc value updated accordingly!
	dataSourceBody.SetAttributeRaw("config", backend.backendConfig.BuildTokens(nil))
	dataSourceBody.AppendNewline()

	return nil
}

func removeOrReplaceModules(moduleBlocks []BlockAndFile, currentModuleName string, otherModuleNames []string, envName *string, backend *TerraformBackend, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, moduleBlock := range moduleBlocks {
		if len(moduleBlock.block.Labels()) != 1 {
			return WrongNumberOfLabels{blockType: moduleBlock.block.Type(), expectedLabelCount: 1, actualLabels: moduleBlock.block.Labels()}
		}

		otherModuleName := moduleBlock.block.Labels()[0]

		// If this isn't the current module, either turn it into a terraform_remote_state data source (if the current
		// module depends on it) or remove it.
		if otherModuleName != currentModuleName {
			dependsOn, err := dependencyGraph.DoesModuleDependOnModule(currentModuleName, otherModuleName)
			if err != nil {
				return err
			}

			if dependsOn {
				terragruntOptions.Logger.Debugf("Replacing module %s with a terraform_remote_state data source", otherModuleName)
				if err := replaceBlockWithDataSource(moduleBlock.block, otherModuleName, backend); err != nil {
					return err
				}
			} else {
				terragruntOptions.Logger.Debugf("Removing module %s", otherModuleName)
				moduleBlock.file.Body().RemoveBlock(moduleBlock.block)
			}

			// Update the graph to indicate the module is gone too
			if err := dependencyGraph.RemoveModule(otherModuleName); err != nil {
				return err
			}
		}
	}

	return nil
}

func replaceBlockWithDataSource(block *hclwrite.Block, blockName string, backend *TerraformBackend) error {
	block.SetType("data")
	block.SetLabels([]string{"terraform_remote_state", blockName})

	block.Body().Clear()
	block.Body().AppendNewline()

	return backend.ConfigureDataSource(block.Body())
}

func replaceReferencesToOtherModulesInBlocks(blocks []BlockAndFile, currentModuleName string, otherModuleNames []string, depthSearchedSoFar int) error {
	// A simple mechanism to avoid infinite loops where we call replaceReferencesToOtherModulesInBlocks recursively
	// over and over indefinitely. There shouldn't be any way to create loops in real HCL code (blocks that contain
	// blocks that somehow loop around to contain their parents), but it's useful to have this as an extra sanity check
	// in case the HCL parser has a bug, or someone creates an artificial AST with loops in it.
	if depthSearchedSoFar > 100 {
		return fmt.Errorf("Hit more than %d nested levels of blocks. Is there any infinite loop somewhere?", depthSearchedSoFar)
	}

	for _, block := range blocks {
		if err := replaceReferencesToOtherModulesInBlocks(getAllBlocksFromBody(block.block.Body(), block.file), currentModuleName, otherModuleNames, depthSearchedSoFar+1); err != nil {
			return err
		}

		if err := replaceReferencesToOtherModulesInAttributes(block.block.Body().Attributes(), otherModuleNames); err != nil {
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

func removeUnneededOutputVariables(outputBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, outputBlock := range outputBlocks {
		if len(outputBlock.block.Labels()) != 1 {
			return WrongNumberOfLabels{blockType: outputBlock.block.Type(), expectedLabelCount: 1, actualLabels: outputBlock.block.Labels()}
		}

		outputName := outputBlock.block.Labels()[0]

		dependsOn, err := dependencyGraph.DoesOutputDependOnModule(outputName, currentModuleName)
		if err != nil {
			return err
		}

		terragruntOptions.Logger.Debugf("Looking up if output %s depends on module %s and got %v", outputName, currentModuleName, dependsOn)

		if !dependsOn {
			outputBlock.file.Body().RemoveBlock(outputBlock.block)

			if err := dependencyGraph.RemoveOutput(outputName); err != nil {
				return err
			}
		}
	}

	return nil
}

func removeUnneededDataSources(dataSourceBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, dataSourceBlock := range dataSourceBlocks {
		if len(dataSourceBlock.block.Labels()) != 2 {
			return WrongNumberOfLabels{blockType: dataSourceBlock.block.Type(), expectedLabelCount: 2, actualLabels: dataSourceBlock.block.Labels()}
		}

		dataSourceType := dataSourceBlock.block.Labels()[0]
		dataSourceName := dataSourceBlock.block.Labels()[1]

		dependsOn, err := dependencyGraph.DoesModuleDependOnDataSource(currentModuleName, dataSourceType, dataSourceName)
		if err != nil {
			return err
		}

		if !dependsOn {
			dataSourceBlock.file.Body().RemoveBlock(dataSourceBlock.block)

			if err := dependencyGraph.RemoveDataSource(dataSourceType, dataSourceName); err != nil {
				return err
			}
		}
	}

	return nil
}

func removeUnneededResources(resourceBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	resourceAddresses := []string{}

	for _, resourceBlock := range resourceBlocks {
		if len(resourceBlock.block.Labels()) != 2 {
			return WrongNumberOfLabels{blockType: resourceBlock.block.Type(), expectedLabelCount: 2, actualLabels: resourceBlock.block.Labels()}
		}

		resourceType := resourceBlock.block.Labels()[0]
		resourceName := resourceBlock.block.Labels()[1]

		resourceAddresses = append(resourceAddresses, fmt.Sprintf("%s.%s", resourceType, resourceName))
	}

	if len(resourceAddresses) > 0 {
		return fmt.Errorf("Top-level resources are not currently supported. That's because when splitting across multiple environments/modules, it's not clear in which one(s) the resource should go. Found %d resources: %v. Please move these into the relevant modules.", len(resourceAddresses), resourceAddresses)
	}

	return nil
}

func removeUnneededProviders(providerBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, providerBlock := range providerBlocks {
		if len(providerBlock.block.Labels()) != 1 {
			return WrongNumberOfLabels{blockType: providerBlock.block.Type(), expectedLabelCount: 1, actualLabels: providerBlock.block.Labels()}
		}

		providerName := providerBlock.block.Labels()[0]

		dependsOn, err := dependencyGraph.DoesModuleDependOnProvider(currentModuleName, providerName)
		if err != nil {
			return err
		}

		if !dependsOn {
			providerBlock.file.Body().RemoveBlock(providerBlock.block)

			if err := dependencyGraph.RemoveProvider(providerName); err != nil {
				return err
			}
		}
	}

	return nil
}

func removeUnneededLocals(localsBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, localsBlock := range localsBlocks {
		for localName, _ := range localsBlock.block.Body().Attributes() {
			dependsOn, err := dependencyGraph.DoesAnythingDependOnLocal(localName)
			if err != nil {
				return err
			}

			if !dependsOn {
				localsBlock.block.Body().RemoveAttribute(localName)

				if err := dependencyGraph.RemoveLocal(localName); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func removeUnneededVariables(variableBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, terragruntOptions *options.TerragruntOptions) error {
	for _, variableBlock := range variableBlocks {
		if len(variableBlock.block.Labels()) != 1 {
			return WrongNumberOfLabels{blockType: variableBlock.block.Type(), expectedLabelCount: 1, actualLabels: variableBlock.block.Labels()}
		}

		variableName := variableBlock.block.Labels()[0]

		dependsOn, err := dependencyGraph.DoesAnythingDependOnVariable(variableName)
		if err != nil {
			return err
		}

		if !dependsOn {
			variableBlock.file.Body().RemoveBlock(variableBlock.block)

			if err := dependencyGraph.RemoveVariable(variableName); err != nil {
				return err
			}
		}
	}

	return nil
}

// Custom error types

type WrongNumberOfLabels struct {
	blockType          string
	expectedLabelCount int
	actualLabels       []string
}

func (err WrongNumberOfLabels) Error() string {
	return fmt.Sprintf("Expected block of type '%s' to have %d labels, but got %d: %v", err.blockType, err.expectedLabelCount, len(err.actualLabels), err.actualLabels)
}
