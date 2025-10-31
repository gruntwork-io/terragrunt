package tf

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

const (
	// TF commands.

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
	CommandNameGraph          = "graph"
	CommandNameTaint          = "taint"
	CommandNameUntaint        = "untaint"
	CommandNameConsole        = "console"
	CommandNameForceUnlock    = "force-unlock"
	CommandNameShow           = "show"
	CommandNameVersion        = "version"
	CommandNameFmt            = "fmt"
	CommandNameLogin          = "login"
	CommandNameLogout         = "logout"
	CommandNameMetadate       = "metadata"
	CommandNamePull           = "pull"
	CommandNameList           = "list"
	CommandNameMove           = "mv"
	CommandNamePush           = "push"
	CommandNameRefresh        = "refresh"
	CommandNameTest           = "test"
	CommandNameWorkspace      = "workspace"

	// Deprecated TF commands.

	CommandNameEnv = "env"

	// TF flags.

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

var (
	CommandNames = []string{
		CommandNameApply,
		CommandNameConsole,
		CommandNameDestroy,
		CommandNameEnv,
		CommandNameFmt,
		CommandNameGet,
		CommandNameGraph,
		CommandNameImport,
		CommandNameInit,
		CommandNameLogin,
		CommandNameLogout,
		CommandNameMetadate,
		CommandNameOutput,
		CommandNamePlan,
		CommandNameProviders,
		CommandNamePush,
		CommandNameRefresh,
		CommandNameShow,
		CommandNameTaint,
		CommandNameTest,
		CommandNameVersion,
		CommandNameValidate,
		CommandNameUntaint,
		CommandNameWorkspace,
		CommandNameForceUnlock,
		CommandNameState,
	}

	CommandUsages = map[string]string{
		CommandNameApply:       "Create or update infrastructure.",
		CommandNameConsole:     "Try OpenTofu/Terraform expressions at an interactive command prompt.",
		CommandNameDestroy:     "Destroy previously-created infrastructure.",
		CommandNameFmt:         "Reformat your configuration in the standard style.",
		CommandNameGet:         "Install or upgrade remote OpenTofu/Terraform modules.",
		CommandNameGraph:       "Generate a Graphviz graph of the steps in an operation.",
		CommandNameImport:      "Associate existing infrastructure with a OpenTofu/Terraform resource.",
		CommandNameInit:        "Prepare your working directory for other commands.",
		CommandNameLogin:       "Obtain and save credentials for a remote host.",
		CommandNameLogout:      "Remove locally-stored credentials for a remote host.",
		CommandNameMetadate:    "Metadata related commands.",
		CommandNameOutput:      "Show output values from your root module.",
		CommandNamePlan:        "Show changes required by the current configuration.",
		CommandNameProviders:   "Show the providers required for this configuration.",
		CommandNameRefresh:     "Update the state to match remote systems.",
		CommandNameShow:        "Show the current state or a saved plan.",
		CommandNameTaint:       "Mark a resource instance as not fully functional.",
		CommandNameTest:        "Execute integration tests for OpenTofu/Terraform modules.",
		CommandNameVersion:     "Show the current OpenTofu/Terraform version.",
		CommandNameValidate:    "Check whether the configuration is valid.",
		CommandNameUntaint:     "Remove the 'tainted' state from a resource instance.",
		CommandNameWorkspace:   "Workspace management.",
		CommandNameForceUnlock: "Release a stuck lock on the current workspace.",
		CommandNameState:       "Advanced state management.",
	}
)

// diagnosticDoesNotAffectModuleVariables tells you if a diagnostic can be ignored for the purpose of extracting
// variables defined in a module.
func diagnosticDoesNotAffectModuleVariables(d tfconfig.Diagnostic) bool {
	ignorableErrors := []tfconfig.Diagnostic{
		// These two occur when a module block uses a variable in the `version` or `source` fields.
		// Terraform doesn't support this, but OpenTofu does. Either way our ability to extract variables is unaffected.
		//
		// What we really need is an OpenTofu version of terraform-config-inspect. This may work for now but as / if
		// syntax continues to diverge we may run into other issues.
		{Summary: "Variables not allowed", Detail: "Variables may not be used here."},
		{Summary: "Unsuitable value type", Detail: "Unsuitable value: value must be known"},
	}
	if d.Severity != tfconfig.DiagError {
		return true
	}

	i := slices.IndexFunc(ignorableErrors, func(ie tfconfig.Diagnostic) bool {
		return ie.Summary == d.Summary && ie.Detail == d.Detail
	})

	return i != -1
}

// ModuleVariables will return all the variables defined in the downloaded terraform modules, taking into
// account all the generated sources. This function will return the required and optional variables separately.
func ModuleVariables(modulePath string) ([]string, []string, error) {
	module, diags := tfconfig.LoadModule(modulePath)

	diags = slices.DeleteFunc(diags, diagnosticDoesNotAffectModuleVariables)
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
