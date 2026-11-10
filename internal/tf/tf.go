package tf

const (
	// TF commands.

	// CommandNameInit is the `init` command.
	CommandNameInit = "init"
	// CommandNameInitFromModule is the command name Terragrunt reports while it downloads a unit's
	// source, so hooks listing `init-from-module` run around the download. No tofu command has this name.
	CommandNameInitFromModule = "init-from-module"
	// CommandNameImport is the `import` command.
	CommandNameImport = "import"
	// CommandNamePlan is the `plan` command.
	CommandNamePlan = "plan"
	// CommandNameApply is the `apply` command.
	CommandNameApply = "apply"
	// CommandNameDestroy is the `destroy` command.
	CommandNameDestroy = "destroy"
	// CommandNameValidate is the `validate` command.
	CommandNameValidate = "validate"
	// CommandNameOutput is the `output` command.
	CommandNameOutput = "output"
	// CommandNameProviders is the `providers` command.
	CommandNameProviders = "providers"
	// CommandNameState is the `state` command.
	CommandNameState = "state"
	// CommandNameLock is the `lock` subcommand of `providers`.
	CommandNameLock = "lock"
	// CommandNameGet is the `get` command.
	CommandNameGet = "get"
	// CommandNameGraph is the `graph` command.
	CommandNameGraph = "graph"
	// CommandNameTaint is the `taint` command.
	CommandNameTaint = "taint"
	// CommandNameUntaint is the `untaint` command.
	CommandNameUntaint = "untaint"
	// CommandNameConsole is the `console` command.
	CommandNameConsole = "console"
	// CommandNameForceUnlock is the `force-unlock` command.
	CommandNameForceUnlock = "force-unlock"
	// CommandNameShow is the `show` command.
	CommandNameShow = "show"
	// CommandNameVersion is the `version` command.
	CommandNameVersion = "version"
	// CommandNameFmt is the `fmt` command.
	CommandNameFmt = "fmt"
	// CommandNameLogin is the `login` command.
	CommandNameLogin = "login"
	// CommandNameLogout is the `logout` command.
	CommandNameLogout = "logout"
	// CommandNameMetadate is the `metadata` command.
	CommandNameMetadate = "metadata"
	// CommandNamePull is the `pull` subcommand of `state`.
	CommandNamePull = "pull"
	// CommandNamePush is the `push` subcommand of `state`.
	CommandNamePush = "push"
	// CommandNameRefresh is the `refresh` command.
	CommandNameRefresh = "refresh"
	// CommandNameTest is the `test` command.
	CommandNameTest = "test"
	// CommandNameWorkspace is the `workspace` command.
	CommandNameWorkspace = "workspace"
	// CommandNameQuery is the `query` command.
	CommandNameQuery = "query"

	// Deprecated TF commands.

	// CommandNameEnv is the `env` command, the deprecated alias of `workspace`.
	CommandNameEnv = "env"

	// TF flags.

	// FlagNameDetailedExitCode is the `-detailed-exitcode` flag.
	FlagNameDetailedExitCode = "-detailed-exitcode"
	// FlagNameHelpLong is the `-help` flag.
	FlagNameHelpLong = "-help"
	// FlagNameHelpShort is the `-h` flag, the short form of [FlagNameHelpLong].
	FlagNameHelpShort = "-h"
	// FlagNameVersion is the `-version` flag.
	FlagNameVersion = "-version"
	// FlagNameJSON is the `-json` flag.
	FlagNameJSON = "-json"
	// FlagNameNoColor is the `-no-color` flag.
	FlagNameNoColor = "-no-color"
	// FlagNameDestroy is the `-destroy` flag. `apply -destroy` is an alias for `destroy`.
	FlagNameDestroy = "-destroy"

	// FlagNamePlatform is the `-platform` flag used with the `providers lock` command.
	FlagNamePlatform = "-platform"

	// FlagNameLockfile is the `-lockfile` flag used with the `init` command to control how the
	// dependency lock file is handled. The `readonly` mode tells OpenTofu/Terraform
	// to fail rather than create or update `.terraform.lock.hcl`.
	FlagNameLockfile = "-lockfile"
	// LockfileModeReadonly is the `readonly` value of [FlagNameLockfile].
	LockfileModeReadonly = "readonly"

	// EnvNameTFCLIConfigFile names the env var that points OpenTofu/Terraform at a CLI configuration file.
	EnvNameTFCLIConfigFile = "TF_CLI_CONFIG_FILE"
	// EnvNameTFPluginCacheDir names the env var that sets the provider plugin cache directory.
	EnvNameTFPluginCacheDir = "TF_PLUGIN_CACHE_DIR"
	// EnvNameTFTokenFmt is the format of the env var that holds the API token for a host. The %s
	// takes the hostname with each dot replaced by an underscore.
	EnvNameTFTokenFmt = "TF_TOKEN_%s"
	// EnvNameTFVarFmt is the format of the env var that sets an input variable. The %s takes the
	// variable name.
	EnvNameTFVarFmt = "TF_VAR_%s"
	// EnvNameTFCLIArgs and EnvNameTFCLIArgsInit name `TF_CLI_ARGS` and `TF_CLI_ARGS_init`, which
	// OpenTofu/Terraform read to inject extra arguments. The suffixed form applies only to the `init` command.
	EnvNameTFCLIArgs = "TF_CLI_ARGS"
	// EnvNameTFCLIArgsInit names `TF_CLI_ARGS_init`, the form of [EnvNameTFCLIArgs] that applies
	// only to `init`.
	EnvNameTFCLIArgsInit = "TF_CLI_ARGS_init"

	// EnvNameTofuCPUProfile names the env var that makes OpenTofu write a CPU profile to the file it names.
	EnvNameTofuCPUProfile = "TOFU_CPU_PROFILE"

	// DefaultTFDataDir is the data directory OpenTofu/Terraform uses when `TF_DATA_DIR` is unset.
	DefaultTFDataDir = ".terraform"
	// TerraformLockFile is the file name of the dependency lock file.
	TerraformLockFile = ".terraform.lock.hcl"

	// TerraformPlanFile is the file name of the plan Terragrunt saves for each unit when an output
	// folder is set.
	TerraformPlanFile = "tfplan.tfplan"
	// TerraformPlanJSONFile is the file name of the JSON plan Terragrunt saves for each unit when a
	// JSON output folder is set.
	TerraformPlanJSONFile = "tfplan.json"
)

var (
	// CommandNames lists the commands `terragrunt run` registers as subcommands.
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

	// CommandUsages maps each command in [CommandNames] to its help text. A command without an
	// entry is hidden from help.
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
