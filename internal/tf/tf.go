package tf

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
	CommandNamePush           = "push"
	CommandNameRefresh        = "refresh"
	CommandNameTest           = "test"
	CommandNameWorkspace      = "workspace"
	CommandNameQuery          = "query"

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

	// `lockfile` is a flag used with the `init` command to control how the
	// dependency lock file is handled. The `readonly` mode tells OpenTofu/Terraform
	// to fail rather than create or update `.terraform.lock.hcl`.
	FlagNameLockfile     = "-lockfile"
	LockfileModeReadonly = "readonly"

	EnvNameTFCLIConfigFile  = "TF_CLI_CONFIG_FILE"
	EnvNameTFPluginCacheDir = "TF_PLUGIN_CACHE_DIR"
	EnvNameTFTokenFmt       = "TF_TOKEN_%s"
	EnvNameTFVarFmt         = "TF_VAR_%s"
	// `TF_CLI_ARGS` and `TF_CLI_ARGS_init` are read by OpenTofu/Terraform to inject
	// extra arguments. The suffixed form applies only to the `init` command.
	EnvNameTFCLIArgs     = "TF_CLI_ARGS"
	EnvNameTFCLIArgsInit = "TF_CLI_ARGS_init"

	EnvNameTofuCPUProfile = "TOFU_CPU_PROFILE"

	DefaultTFDataDir  = ".terraform"
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
		CommandNameQuery,
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
