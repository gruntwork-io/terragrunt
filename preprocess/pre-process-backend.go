package preprocess

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"strings"
)

type TerraformBackend struct {
	backendType   string
	backendConfig *hclwrite.Body
}

func IsBackendBlock(block *hclwrite.Block) bool {
	return block.Type() == "backend" || block.Type() == "cloud"
}

func NewTerraformBackend(block *hclwrite.Block) (*TerraformBackend, error) {
	if block.Type() == "backend" {
		if len(block.Labels()) != 1 {
			return nil, errors.WithStackTrace(WrongNumberOfLabels{blockType: block.Type(), expectedLabelCount: 1, actualLabels: block.Labels()})
		}

		return &TerraformBackend{backendType: block.Labels()[0], backendConfig: block.Body()}, nil
	} else if block.Type() == "cloud" {
		// Special handling for the new cloud block, which is an alternative to the standard backend block:
		// https://developer.hashicorp.com/terraform/cli/cloud/settings

		return &TerraformBackend{backendType: "cloud", backendConfig: block.Body()}, nil
	} else {
		return nil, errors.WithStackTrace(fmt.Errorf("Unrecognized backend block: %s", block.Type()))
	}
}

func (backend *TerraformBackend) UpdateConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	switch backend.backendType {
	case "local":
		return backend.updateLocalConfig(currentModuleName, envName, terragruntOptions)
	case "remote":
		return backend.updateRemoteConfig(currentModuleName, envName, terragruntOptions)
	case "azurerm":
		return backend.updateAzureRmConfig(currentModuleName, envName, terragruntOptions)
	case "consul":
		return backend.updateConsulConfig(currentModuleName, envName, terragruntOptions)
	case "cos":
		return backend.notSupportedBackend(terragruntOptions)
	case "gcs":
		return backend.updateGcsConfig(currentModuleName, envName, terragruntOptions)
	case "http":
		return backend.notSupportedBackend(terragruntOptions)
	case "kubernetes":
		return backend.notSupportedBackend(terragruntOptions)
	case "oss":
		return backend.notSupportedBackend(terragruntOptions)
	case "pg":
		return backend.notSupportedBackend(terragruntOptions)
	case "s3":
		return backend.updateS3Config(currentModuleName, envName, terragruntOptions)
	case "cloud":
		return backend.updateCloudConfig(currentModuleName, envName, terragruntOptions)
	default:
		return backend.notSupportedBackend(terragruntOptions)
	}
}

// https://developer.hashicorp.com/terraform/language/settings/backends/local
// Updates the path param
func (backend *TerraformBackend) updateLocalConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateBackendConfigAttr("path", currentModuleName, envName, terragruntOptions)
}

// https://developer.hashicorp.com/terraform/language/settings/backends/remote
// TODO: this only supports named workspaces; not those using prefix
func (backend *TerraformBackend) updateRemoteConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateWorkspacesConfigAttr("workspaces", "name", currentModuleName, envName, terragruntOptions)
}

// https://developer.hashicorp.com/terraform/cli/cloud/settings
// TODO: this only supports named workspaces; not those using tags
func (backend *TerraformBackend) updateCloudConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateWorkspacesConfigAttr("workspaces", "name", currentModuleName, envName, terragruntOptions)
}

// https://developer.hashicorp.com/terraform/language/settings/backends/azurerm
// Updates the key param
func (backend *TerraformBackend) updateAzureRmConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateBackendConfigAttr("key", currentModuleName, envName, terragruntOptions)
}

// https://developer.hashicorp.com/terraform/language/settings/backends/consul
// Updates the path param
func (backend *TerraformBackend) updateConsulConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateBackendConfigAttr("path", currentModuleName, envName, terragruntOptions)
}

// https://developer.hashicorp.com/terraform/language/settings/backends/gcs
// Updates the prefix param
func (backend *TerraformBackend) updateGcsConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateBackendConfigAttr("prefix", currentModuleName, envName, terragruntOptions)
}

// https://developer.hashicorp.com/terraform/language/settings/backends/s3
// Updates the key param
func (backend *TerraformBackend) updateS3Config(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	return backend.updateBackendConfigAttr("key", currentModuleName, envName, terragruntOptions)
}

func (backend *TerraformBackend) updateBackendConfigAttr(attrName string, currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	originalAttr := backend.backendConfig.GetAttribute(attrName)
	newValue := formatStatePath(currentModuleName, envName, attrValueAsString(originalAttr))

	terragruntOptions.Logger.Debugf("Updating '%s' backend: setting '%s' to '%s'", backend.backendType, attrName, newValue)
	backend.backendConfig.SetAttributeValue(attrName, cty.StringVal(newValue))

	return nil
}

func (backend *TerraformBackend) updateWorkspacesConfigAttr(workspacesBlockName string, workspacesAttrName string, currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	workspacesBlock := backend.backendConfig.FirstMatchingBlock(workspacesBlockName, []string{})
	if workspacesBlock == nil {
		workspacesBlock = backend.backendConfig.AppendNewBlock(workspacesBlockName, []string{})
	}

	originalWorkspaceNameAttr := workspacesBlock.Body().GetAttribute(workspacesAttrName)
	newWorkspaceName := formatWorkspace(currentModuleName, envName, attrValueAsString(originalWorkspaceNameAttr))

	terragruntOptions.Logger.Debugf("Updating '%s' backend: setting '%s' in '%s' block to '%s'", backend.backendType, workspacesAttrName, workspacesBlockName, newWorkspaceName)
	workspacesBlock.Body().SetAttributeValue(workspacesAttrName, cty.StringVal(newWorkspaceName))

	return nil
}

func (backend *TerraformBackend) notSupportedBackend(terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Warnf("Backend '%s' is not yet supported! Cannot update the config automatically, so please ensure you do it manually!", backend.backendType)
	return nil
}

func formatStatePath(currentModuleName string, envName *string, originalStatePath *string) string {
	out := currentModuleName

	if envName != nil {
		out = fmt.Sprintf("%s/%s", *envName, out)
	}

	if originalStatePath == nil {
		out = fmt.Sprintf("%s/%s", out, "terraform.tfstate")
	} else {
		out = fmt.Sprintf("%s/%s", out, *originalStatePath)
	}

	return out
}

func formatWorkspace(currentModuleName string, envName *string, originalWorkspace *string) string {
	out := currentModuleName

	if envName != nil {
		out = fmt.Sprintf("%s-%s", *envName, out)
	}

	if originalWorkspace != nil {
		out = fmt.Sprintf("%s-%s", out, *originalWorkspace)
	}

	return out
}

var openBraceToken = &hclwrite.Token{
	Type:  hclsyntax.TokenOBrace,
	Bytes: []byte("{"),
}

var closeBraceToken = &hclwrite.Token{
	Type:  hclsyntax.TokenCBrace,
	Bytes: []byte("}"),
}

func (backend *TerraformBackend) ConfigureDataSource(dataSourceBody *hclwrite.Body, currentModuleName string, otherModuleName string, envName *string) error {
	dataSourceBody.SetAttributeValue("backend", cty.StringVal(backend.backendType))
	dataSourceBody.AppendNewline()

	configBody := backend.backendConfig.BuildTokens(nil)

	// For local backends, we need to set the config path to a relative path, and that need to be relative to the module
	// we're reading state from.
	if backend.backendType == "local" {
		parsed, err := hclwrite.ParseConfig(configBody.Bytes(), "__internal__", hcl.InitialPos)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		// Take the path of the current module, which we should've already configured properly with the UpdateConfig
		// method, and replace the current module's name with the module for which we're creating a
		// terraform_remote_state data source. Then, make it a relative path: the path from the current module to that
		// other module.
		originalPath := attrValueAsString(backend.backendConfig.GetAttribute("path"))
		if originalPath == nil {
			return errors.WithStackTrace(fmt.Errorf("Could not find path param in config. This is most likely a bug with Terragrunt. Please report it at https://github.com/gruntwork-io/terragrunt/issues/"))
		}
		basePath := strings.Replace(*originalPath, currentModuleName, otherModuleName, 1)
		newPath := fmt.Sprintf("${path.module}/../%s/%s", otherModuleName, basePath)

		// It's not clear how to set attributes that contain string interpolation with hclwrite. If you try it with
		// the SetAttributeValue method, the interpolation parts (${ ... }) get escaped. So here, we use an ugly hack
		// where we define what we want as a literal (string) HCL expression, parse it with hclwrite, and then read out
		// the parsed value as hclwrite types we can use later.
		newPathExpr := fmt.Sprintf(`path = "%s"`, newPath)
		parsedPath, err := hclwrite.ParseConfig([]byte(newPathExpr), "__internal__", hcl.InitialPos)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		parsed.Body().SetAttributeRaw("path", parsedPath.Body().GetAttribute("path").Expr().BuildTokens(nil))

		configBody = parsed.BuildTokens(nil)
	}

	configTokens := []*hclwrite.Token{}
	configTokens = append(configTokens, openBraceToken)
	configTokens = append(configTokens, configBody...)
	configTokens = append(configTokens, closeBraceToken)

	dataSourceBody.SetAttributeRaw("config", configTokens)
	dataSourceBody.AppendNewline()

	return nil
}
