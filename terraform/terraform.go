package terraform

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

const (
	CommandNameInit           = "init"
	CommandNameInitFromModule = "init-from-module"
	CommandNameImport         = "import"
	CommandNamePlan           = "plan"
	CommandNameApply          = "apply"
	CommandNameDestroy        = "destroy"
	CommandNameValidate       = "validate"
	CommandNameOutput         = "output"
	CommandNameProviders      = "providers"
	CommandNameState          = "state"
	CommandNameLock           = "lock"
	CommandNameGet            = "get"
	CommandNameTaint          = "taint"
	CommandNameUntaint        = "untaint"
	CommandNameConsole        = "console"
	CommandNameForceUnlock    = "force-unlock"
	CommandNameShow           = "show"
	CommandNameVersion        = "version"

	FlagNameDetailedExitCode = "-detailed-exitcode"
	FlagNameHelpLong         = "-help"
	FlagNameHelpShort        = "-h"
	FlagNameVersion          = "-version"
	FlagNameJSON             = "-json"
	FlagNameNoColor          = "-no-color"
	// `apply -destroy` is alias for `destroy`
	FlagNameDestroy = "-destroy"

	// `platform` is a flag used with the `providers lock` command.
	FlagNamePlatform = "-platform"

	EnvNameTFCLIConfigFile                         = "TF_CLI_CONFIG_FILE"
	EnvNameTFPluginCacheDir                        = "TF_PLUGIN_CACHE_DIR"
	EnvNameTFPluginCacheMayBreakDependencyLockFile = "TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE"
	EnvNameTFTokenFmt                              = "TF_TOKEN_%s"
	EnvNameTFVarFmt                                = "TF_VAR_%s"

	TerraformLockFile = ".terraform.lock.hcl"

	TerraformPlanFile     = "tfplan.tfplan"
	TerraformPlanJSONFile = "tfplan.json"
)

// ModuleVariables will return all the variables defined in the downloaded terraform modules, taking into
// account all the generated sources. This function will return the required and optional variables separately.
func ModuleVariables(modulePath string) ([]string, []string, error) {
	module, diags := tfconfig.LoadModule(modulePath)
	if diags.HasErrors() {
		return nil, nil, errors.New(diags)
	}

	required := []string{}
	optional := []string{}

	for _, variable := range module.Variables {
		if variable.Required {
			required = append(required, variable.Name)
		} else {
			optional = append(optional, variable.Name)
		}
	}

	return required, optional, nil
}
