package preprocess

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/cli/tfsource"
	"github.com/gruntwork-io/terragrunt/graph"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func processFiles(parsedTerraformFiles TerraformFiles, modulePath string, currentModuleName string, otherModuleNames []string, envName *string, dependencyGraph *graph.TerraformGraph, parsedTerraformVariableFiles TerraformFiles, terragruntOptions *options.TerragruntOptions) error {
	allBlocks := getAllBlocks(parsedTerraformFiles)
	blocksByType := groupBlocksByType(allBlocks)

	// The order of these steps matters! For example, if we remove output variables not relevant to the current
	// module first, then when we go to remove locals, the "does any output variable depend on this local?" check
	// will apply only to the outputs that are relevant, rather than all outputs.

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

	if err := updateModuleSourceUrls(blocksByType["module"], modulePath, terragruntOptions); err != nil {
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

	if err := removeUnneededVariables(blocksByType["variable"], currentModuleName, dependencyGraph, parsedTerraformVariableFiles, terragruntOptions); err != nil {
		return err
	}

	if err := addModuleOutput(allBlocks, blocksByType["output"], currentModuleName, terragruntOptions); err != nil {
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
			if IsBackendBlock(nestedBlock) {
				backend, err := NewTerraformBackend(nestedBlock)
				if err != nil {
					return nil, err
				}

				if err := backend.UpdateConfig(currentModuleName, envName, terragruntOptions); err != nil {
					return nil, err
				}

				return backend, nil
			}
		}
	}

	return nil, nil
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
				if err := replaceBlockWithDataSource(moduleBlock.block, currentModuleName, otherModuleName, backend, envName, terragruntOptions); err != nil {
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

func replaceBlockWithDataSource(block *hclwrite.Block, currentModuleName string, otherModuleName string, backend *TerraformBackend, envName *string, terragruntOptions *options.TerragruntOptions) error {
	block.SetType("data")
	block.SetLabels([]string{"terraform_remote_state", otherModuleName})

	block.Body().Clear()
	block.Body().AppendNewline()

	return backend.ConfigureDataSource(block.Body(), currentModuleName, otherModuleName, envName, terragruntOptions)
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
			attr.Expr().RenameVariablePrefix([]string{"module", otherModuleName}, []string{"data", fmt.Sprintf("terraform_remote_state.%s.outputs.%s", otherModuleName, moduleOutputName)})
		}
	}

	return nil
}

func updateModuleSourceUrls(moduleBlocks []BlockAndFile, modulePath string, terragruntOptions *options.TerragruntOptions) error {
	canonicalModulePath, err := util.CanonicalPath(modulePath, "")
	if err != nil {
		return err
	}

	for _, moduleBlock := range moduleBlocks {
		sourceAttr := moduleBlock.block.Body().GetAttribute("source")
		sourceAsStr := attrValueAsString(sourceAttr)

		if sourceAsStr != nil {
			canonicalWorkingDir, err := util.CanonicalPath(terragruntOptions.WorkingDir, "")
			if err != nil {
				return err
			}

			url, err := tfsource.ToSourceUrl(*sourceAsStr, canonicalWorkingDir)
			if err != nil {
				return err
			}

			if tfsource.IsLocalSource(url) {
				relPath, err := util.GetPathRelativeTo(url.Path, canonicalModulePath)
				if err != nil {
					return err
				}
				terragruntOptions.Logger.Debugf("Updating 'source' parameter, which was a local file path, from '%s' to '%s', to account for the new output directory '%s'.", *sourceAsStr, relPath, canonicalModulePath)
				moduleBlock.block.Body().SetAttributeValue("source", cty.StringVal(relPath))
			}
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

		if !dependsOn {
			terragruntOptions.Logger.Debugf("Removing output variable %s", outputName)

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
			terragruntOptions.Logger.Debugf("Removing data source %s.%s", dataSourceType, dataSourceName)

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
	// TODO: the 'terraform graph' command doesn't seem to produce the full dependency graph for providers: not all
	// the modules that use a provider block seem to have a clear dependency on it, so if we remove it here, then we
	// end up removing too much. Therefore, for now, disabling this logic.

	//for _, providerBlock := range providerBlocks {
	//	if len(providerBlock.block.Labels()) != 1 {
	//		return WrongNumberOfLabels{blockType: providerBlock.block.Type(), expectedLabelCount: 1, actualLabels: providerBlock.block.Labels()}
	//	}
	//
	//	providerName := providerBlock.block.Labels()[0]
	//
	//	dependsOn, err := dependencyGraph.DoesAnythingDependOnProvider(providerName)
	//	if err != nil {
	//		return err
	//	}
	//
	//	if !dependsOn {
	//		terragruntOptions.Logger.Debugf("Removing provider %s", providerName)
	//
	//		providerBlock.file.Body().RemoveBlock(providerBlock.block)
	//
	//		if err := dependencyGraph.RemoveProvider(providerName); err != nil {
	//			return err
	//		}
	//	}
	//}

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
				terragruntOptions.Logger.Debugf("Removing local %s", localName)

				localsBlock.block.Body().RemoveAttribute(localName)

				if err := dependencyGraph.RemoveLocal(localName); err != nil {
					return err
				}
			}
		}

		if len(localsBlock.block.Body().Attributes()) == 0 {
			terragruntOptions.Logger.Debugf("Removing empty locals block")
			localsBlock.file.Body().RemoveBlock(localsBlock.block)
		}
	}

	return nil
}

func removeUnneededVariables(variableBlocks []BlockAndFile, currentModuleName string, dependencyGraph *graph.TerraformGraph, parsedTerraformVariableFiles TerraformFiles, terragruntOptions *options.TerragruntOptions) error {
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
			terragruntOptions.Logger.Debugf("Removing input variable %s", variableName)

			variableBlock.file.Body().RemoveBlock(variableBlock.block)

			if err := dependencyGraph.RemoveVariable(variableName); err != nil {
				return err
			}

			if err := removeVariableFromVarFiles(variableName, parsedTerraformVariableFiles); err != nil {
				return err
			}
		}
	}

	return nil
}

func addModuleOutput(allBlocks []BlockAndFile, outputBlocks []BlockAndFile, currentModuleName string, terragruntOptions *options.TerragruntOptions) error {
	var fileToWriteTo *hclwrite.File

	if len(outputBlocks) > 0 {
		// If there are output variables already defined somewhere, we'll add our outputs to the first file where we find
		// outputs
		fileToWriteTo = outputBlocks[0].file
	} else {
		// Otherwise, pick the first file in the list
		fileToWriteTo = allBlocks[0].file
	}

	block := fileToWriteTo.Body().AppendNewBlock("output", []string{moduleOutputName})
	block.Body().AppendNewline()
	block.Body().SetAttributeValue("description", cty.StringVal("This output is added by Terragrunt so that modules that depend on each other can read all the info they need from each other's state files using this output variable and the terraform_remote_state data source."))
	block.Body().AppendNewline()
	block.Body().SetAttributeTraversal("value", hcl.Traversal{
		hcl.TraverseRoot{Name: "module"},
		hcl.TraverseAttr{Name: currentModuleName},
	})

	return nil
}

func removeVariableFromVarFiles(variableName string, parsedTerraformVariableFiles TerraformFiles) error {
	for _, parsedFile := range parsedTerraformVariableFiles {
		parsedFile.Body().RemoveAttribute(variableName)
	}

	return nil
}

const moduleOutputName = "__module__"
