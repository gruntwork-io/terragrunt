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
		return nil, errors.WithStackTrace(UnrecognizedBackendType(block.Type()))
	}
}

// BackendHandler represents a way to automatically update various types of Terraform backends: e.g., s3, azurerm, gcs,
// remote, cloud, etc.
type BackendHandler interface {
	// UpdateBackendConfig updates the backend configuration for the current module to have the proper settings in the
	// generated code (after preprocessing). This method should make the changes directly in the given backend object.
	UpdateBackendConfig(backend *TerraformBackend, currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error

	// UpdateTerraformRemoteStateConfig updates the configuration of a terraform_remote_state data source to allow the
	// current module to read the state file of another module. This method should make the changes directly in the
	// given backendConfigBody object, which represents the body of the config = { ... } section of the
	// terraform_remote_state data source.
	UpdateTerraformRemoteStateConfig(backend *TerraformBackend, backendConfigBody *hclwrite.Body, currentModuleName string, otherModuleName string, envName *string) error
}

// PathBasedBackendHandler represents Terraform backends that track where to store Terraform state using a "path" that
// looks like a file system path: e.g., /foo/bar/terraform.tfstate.
type PathBasedBackendHandler struct {
	// The name of the attribute in this backend's config that stores the "path" of the Terraform state file
	PathAttributeName string
}

func (handler PathBasedBackendHandler) UpdateBackendConfig(backend *TerraformBackend, currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	originalAttr := backend.backendConfig.GetAttribute(handler.PathAttributeName)
	newValue := formatStatePath(currentModuleName, envName, attrValueAsString(originalAttr))

	terragruntOptions.Logger.Debugf("Updating '%s' backend: setting '%s' to '%s'", backend.backendType, handler.PathAttributeName, newValue)
	backend.backendConfig.SetAttributeValue(handler.PathAttributeName, cty.StringVal(newValue))

	return nil
}

func (handler PathBasedBackendHandler) UpdateTerraformRemoteStateConfig(backend *TerraformBackend, backendConfigBody *hclwrite.Body, currentModuleName string, otherModuleName string, envName *string) error {
	// Take the path of the current module, which we should've already configured properly with the UpdateConfig
	// method, and replace the current module's name with the module for which we're creating a
	// terraform_remote_state data source.
	originalPath := attrValueAsString(backendConfigBody.GetAttribute(handler.PathAttributeName))
	if originalPath == nil {
		return errors.WithStackTrace(MissingExpectedParam{param: handler.PathAttributeName, block: "backend"})
	}
	newPath := strings.Replace(*originalPath, currentModuleName, otherModuleName, 1)

	// For the local backend, we have to explicitly make it a relative path on the file system from the current module
	// to that other module
	if backend.backendType == "local" {
		newPath = fmt.Sprintf("${path.module}/../%s/%s", otherModuleName, newPath)
	}

	// The new value we want to set may contain string interpolation ("${path.module}"), and I haven't figured out how
	// to set an attribute value with string interplation with hclwrite without hclwrite escaping the interpolation
	// (adding a second dollar sign, so you get "$${path.module}"). So here, we use an ugly hack where we define what
	// we want as a literal string that contains a "foo = bar" HCL expression, parse the string with hclwrite, and then
	// read out the foo value, which will now have all the proper hclwrite types, without additional escaping.
	placeholderAttr := "__attr__"
	newPathExpr := fmt.Sprintf(`%s = "%s"`, placeholderAttr, newPath)
	parsedPath, err := hclwrite.ParseConfig([]byte(newPathExpr), "__internal__", hcl.InitialPos)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	backendConfigBody.SetAttributeRaw(handler.PathAttributeName, parsedPath.Body().GetAttribute(placeholderAttr).Expr().BuildTokens(nil))

	return nil
}

// WorkspaceBasedBackendHandler represents Terraform backends that track where to store Terraform state using
// workspaces.
// TODO: this handler only supports named workspaces; it does not support workspaces that use prefixes or tags.
type WorkspaceBasedBackendHandler struct {
	// The name of the block in this backend's config that stores the workspace configuration
	WorkspaceBlockName string
	// The name of the attribute in the WorkspaceBlockName block that stores the name of the workspace
	WorkspaceNameAttrName string
}

func (handler WorkspaceBasedBackendHandler) UpdateBackendConfig(backend *TerraformBackend, currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	workspacesBlock := backend.backendConfig.FirstMatchingBlock(handler.WorkspaceBlockName, []string{})
	if workspacesBlock == nil {
		workspacesBlock = backend.backendConfig.AppendNewBlock(handler.WorkspaceBlockName, []string{})
	}

	originalWorkspaceNameAttr := workspacesBlock.Body().GetAttribute(handler.WorkspaceNameAttrName)
	newWorkspaceName := formatWorkspace(currentModuleName, envName, attrValueAsString(originalWorkspaceNameAttr))

	terragruntOptions.Logger.Debugf("Updating '%s' backend: setting '%s' in '%s' block to '%s'", backend.backendType, handler.WorkspaceNameAttrName, handler.WorkspaceBlockName, newWorkspaceName)
	workspacesBlock.Body().SetAttributeValue(handler.WorkspaceNameAttrName, cty.StringVal(newWorkspaceName))

	return nil
}

func (handler WorkspaceBasedBackendHandler) UpdateTerraformRemoteStateConfig(backend *TerraformBackend, backendConfigBody *hclwrite.Body, currentModuleName string, otherModuleName string, envName *string) error {
	// Take the workspace name of the current module, which we should've already configured properly with the
	// UpdateConfig method, and replace the current module's name with the module for which we're creating a
	// terraform_remote_state data source.
	originalWorkspaceBlock := backendConfigBody.FirstMatchingBlock(handler.WorkspaceBlockName, []string{})
	if originalWorkspaceBlock == nil {
		return errors.WithStackTrace(MissingExpectedParam{block: handler.WorkspaceBlockName})
	}
	originalWorkspaceName := attrValueAsString(originalWorkspaceBlock.Body().GetAttribute(handler.WorkspaceNameAttrName))
	if originalWorkspaceName == nil {
		return errors.WithStackTrace(MissingExpectedParam{param: handler.WorkspaceNameAttrName, block: handler.WorkspaceBlockName})
	}
	newWorkspaceName := strings.Replace(*originalWorkspaceName, currentModuleName, otherModuleName, 1)

	originalWorkspaceBlock.Body().SetAttributeValue(handler.WorkspaceNameAttrName, cty.StringVal(newWorkspaceName))

	return nil
}

// The backends we currently support: https://developer.hashicorp.com/terraform/language/settings/backends/configuration
var supportedBackendHandlers = map[string]BackendHandler{
	"local":   PathBasedBackendHandler{PathAttributeName: "path"},
	"azurerm": PathBasedBackendHandler{PathAttributeName: "key"},
	"consul":  PathBasedBackendHandler{PathAttributeName: "path"},
	"gcs":     PathBasedBackendHandler{PathAttributeName: "prefix"},
	"s3":      PathBasedBackendHandler{PathAttributeName: "key"},
	"remote":  WorkspaceBasedBackendHandler{WorkspaceBlockName: "workspaces", WorkspaceNameAttrName: "name"},
	"cloud":   WorkspaceBasedBackendHandler{WorkspaceBlockName: "workspaces", WorkspaceNameAttrName: "name"},
}

func (backend *TerraformBackend) UpdateConfig(currentModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	handler, supported := supportedBackendHandlers[backend.backendType]

	if !supported {
		terragruntOptions.Logger.Warnf("Backend '%s' is not yet supported! Cannot update the backend config automatically, so please ensure you do it manually!", backend.backendType)
		return nil
	}

	return handler.UpdateBackendConfig(backend, currentModuleName, envName, terragruntOptions)
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

func (backend *TerraformBackend) ConfigureDataSource(dataSourceBody *hclwrite.Body, currentModuleName string, otherModuleName string, envName *string, terragruntOptions *options.TerragruntOptions) error {
	dataSourceBody.SetAttributeValue("backend", cty.StringVal(backend.backendType))
	dataSourceBody.AppendNewline()

	configBody := backend.backendConfig.BuildTokens(nil)

	parsedConfigBody, err := hclwrite.ParseConfig(configBody.Bytes(), "__internal__", hcl.InitialPos)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	handler, supported := supportedBackendHandlers[backend.backendType]

	if supported {
		if err := handler.UpdateTerraformRemoteStateConfig(backend, parsedConfigBody.Body(), currentModuleName, otherModuleName, envName); err != nil {
			return err
		}
		configBody = parsedConfigBody.BuildTokens(nil)
	} else {
		terragruntOptions.Logger.Warnf("Backend '%s' is not yet supported! Cannot update the terraform_remote_state data source config automatically, so please ensure you do it manually!", backend.backendType)
	}

	configTokens := []*hclwrite.Token{}
	configTokens = append(configTokens, openBraceToken)
	configTokens = append(configTokens, configBody...)
	configTokens = append(configTokens, closeBraceToken)

	dataSourceBody.SetAttributeRaw("config", configTokens)

	return nil
}
