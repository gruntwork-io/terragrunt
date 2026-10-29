package cli_test

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
)

// cliInventory is every command and flag the Terragrunt CLI exposes, with the
// names and environment variables each flag answers to. [TestCLIInventory]
// compares it with the commands the app registers, and
// [TestCLIHelpMatchesInventory] with the help each command prints. Adding,
// renaming, deprecating, or hiding a command or flag means updating it here.
var cliInventory = cliCommand{
	name:  "terragrunt",
	flags: globalFlags,
	subcommands: []cliCommand{
		{
			name:          "apply",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "aws-provider-patch",
			category: commands.ConfigurationCommandsCategoryName,
			hidden:   true,
			flags: slices.Concat(runFlags, []cliFlag{
				allFlag,
				{
					names:             []string{"override-attr"},
					envVars:           []string{"TG_OVERRIDE_ATTR"},
					deprecatedEnvVars: []string{"TERRAGRUNT_OVERRIDE_ATTR"},
				},
			}),
		},
		{
			name:     "backend",
			category: commands.MainCommandsCategoryName,
			subcommands: []cliCommand{
				{
					name: "bootstrap",
					flags: []cliFlag{
						allFlag,
						{
							names:             []string{"backend-bootstrap"},
							envVars:           []string{"TG_BACKEND_BOOTSTRAP"},
							deprecatedEnvVars: []string{"TG_TG_BACKEND_BOOTSTRAP"},
						},
						{
							names:             []string{"backend-require-bootstrap"},
							envVars:           []string{"TG_BACKEND_REQUIRE_BOOTSTRAP"},
							deprecatedEnvVars: []string{"TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION", "TG_TG_BACKEND_REQUIRE_BOOTSTRAP"},
						},
						{
							names:             []string{"config"},
							envVars:           []string{"TG_CONFIG"},
							deprecatedEnvVars: []string{"TERRAGRUNT_CONFIG", "TG_TG_CONFIG"},
						},
						{
							names:             []string{"disable-bucket-update"},
							envVars:           []string{"TG_DISABLE_BUCKET_UPDATE"},
							deprecatedEnvVars: []string{"TERRAGRUNT_DISABLE_BUCKET_UPDATE", "TG_TG_DISABLE_BUCKET_UPDATE"},
						},
						{
							names:             []string{"download-dir"},
							envVars:           []string{"TG_DOWNLOAD_DIR"},
							deprecatedEnvVars: []string{"TERRAGRUNT_DOWNLOAD", "TERRAGRUNT_DOWNLOAD_DIR", "TG_TG_DOWNLOAD_DIR"},
						},
						{names: []string{"fail-fast"}, envVars: []string{"TG_FAIL_FAST"}},
						{
							names:             []string{"feature"},
							envVars:           []string{"TG_FEATURE"},
							deprecatedEnvVars: []string{"TERRAGRUNT_FEATURE", "TG_TG_FEATURE"},
						},
					},
				},
				{
					name: "delete",
					flags: slices.Concat(backendFlags, []cliFlag{
						allFlag,
						featureFlag,
						{names: []string{"bucket"}, envVars: []string{"TG_BUCKET"}, hidden: true},
						{
							names:             []string{"config"},
							envVars:           []string{"TG_CONFIG"},
							deprecatedEnvVars: []string{"TERRAGRUNT_CONFIG"},
						},
						{
							names:             []string{"download-dir"},
							envVars:           []string{"TG_DOWNLOAD_DIR"},
							deprecatedEnvVars: []string{"TERRAGRUNT_DOWNLOAD", "TERRAGRUNT_DOWNLOAD_DIR"},
						},
						{names: []string{"fail-fast"}, envVars: []string{"TG_FAIL_FAST"}},
						{names: []string{"force"}, envVars: []string{"TG_FORCE"}},
					}),
				},
				{
					name: "migrate",
					flags: slices.Concat(backendFlags, []cliFlag{
						featureFlag,
						{
							names:             []string{"config"},
							envVars:           []string{"TG_CONFIG"},
							deprecatedEnvVars: []string{"TERRAGRUNT_CONFIG"},
						},
						{
							names:             []string{"download-dir"},
							envVars:           []string{"TG_DOWNLOAD_DIR"},
							deprecatedEnvVars: []string{"TERRAGRUNT_DOWNLOAD", "TERRAGRUNT_DOWNLOAD_DIR"},
						},
						{names: []string{"force"}, envVars: []string{"TG_FORCE"}},
					}),
				},
			},
		},
		{
			name:     "browse",
			category: commands.DiscoveryCommandsCategoryName,
			flags: []cliFlag{
				{
					names:             []string{"feature"},
					envVars:           []string{"TG_FEATURE"},
					deprecatedEnvVars: []string{"TERRAGRUNT_FEATURE", "TG_BROWSE_FEATURE"},
				},
				{
					names:             []string{"no-discovery-auth-provider-cmd"},
					envVars:           []string{"TG_NO_DISCOVERY_AUTH_PROVIDER_CMD"},
					deprecatedEnvVars: []string{"TG_BROWSE_NO_DISCOVERY_AUTH_PROVIDER_CMD"},
				},
			},
		},
		{
			name:     "catalog",
			category: commands.CatalogCommandsCategoryName,
			flags: []cliFlag{
				{names: []string{"cas-clone-depth"}, envVars: []string{"TG_CAS_CLONE_DEPTH"}},
				{names: []string{"cas-offline"}, envVars: []string{"TG_CAS_OFFLINE"}},
				{names: []string{"cas-probe-ttl"}, envVars: []string{"TG_CAS_PROBE_TTL"}},
				{names: []string{"cas-refresh"}, envVars: []string{"TG_CAS_REFRESH"}},
				{names: []string{"format"}, envVars: []string{"TG_FORMAT"}},
				{names: []string{"ignore-file"}, envVars: []string{"TG_IGNORE_FILE"}},
				{names: []string{"no-cas"}, envVars: []string{"TG_NO_CAS"}},
				{names: []string{"no-hooks"}, envVars: []string{"TG_NO_HOOKS"}},
				{names: []string{"no-include-root"}, envVars: []string{"TG_NO_INCLUDE_ROOT"}},
				{names: []string{"no-shell"}, envVars: []string{"TG_NO_SHELL"}},
				{names: []string{"root-file-name"}, envVars: []string{"TG_ROOT_FILE_NAME"}},
			},
		},
		{
			name:     "dag",
			category: commands.ConfigurationCommandsCategoryName,
			subcommands: []cliCommand{
				{
					name: "graph",
					flags: slices.Concat(backendFlags, filterFlags, queueFlags, []cliFlag{
						featureFlag,
					}),
				},
			},
		},
		{
			name:          "destroy",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "exec",
			category: commands.MainCommandsCategoryName,
			flags: []cliFlag{
				{
					names:             []string{"auth-provider-cmd"},
					envVars:           []string{"TG_AUTH_PROVIDER_CMD"},
					deprecatedEnvVars: []string{"TERRAGRUNT_AUTH_PROVIDER_CMD"},
				},
				{
					names:             []string{"config"},
					envVars:           []string{"TG_CONFIG"},
					deprecatedEnvVars: []string{"TERRAGRUNT_CONFIG"},
				},
				{
					names:             []string{"download-dir"},
					envVars:           []string{"TG_DOWNLOAD_DIR"},
					deprecatedEnvVars: []string{"TERRAGRUNT_DOWNLOAD", "TERRAGRUNT_DOWNLOAD_DIR"},
				},
				{
					names:             []string{"iam-assume-role"},
					envVars:           []string{"TG_IAM_ASSUME_ROLE"},
					deprecatedEnvVars: []string{"TERRAGRUNT_IAM_ROLE"},
				},
				{
					names:             []string{"iam-assume-role-duration"},
					envVars:           []string{"TG_IAM_ASSUME_ROLE_DURATION"},
					deprecatedEnvVars: []string{"TERRAGRUNT_IAM_ASSUME_ROLE_DURATION"},
				},
				{
					names:             []string{"iam-assume-role-session-name"},
					envVars:           []string{"TG_IAM_ASSUME_ROLE_SESSION_NAME"},
					deprecatedEnvVars: []string{"TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME"},
				},
				{
					names:             []string{"iam-assume-role-web-identity-token"},
					envVars:           []string{"TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"},
					deprecatedEnvVars: []string{"TERRAGRUNT_IAM_WEB_IDENTITY_TOKEN", "TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"},
				},
				{names: []string{"in-download-dir"}, envVars: []string{"TG_IN_DOWNLOAD_DIR"}},
				{
					names:             []string{"inputs-debug"},
					envVars:           []string{"TG_INPUTS_DEBUG"},
					deprecatedEnvVars: []string{"TERRAGRUNT_DEBUG"},
				},
				{
					names:             []string{"no-auto-init"},
					envVars:           []string{"TG_NO_AUTO_INIT"},
					deprecatedEnvVars: []string{"TERRAGRUNT_AUTO_INIT"},
				},
				{
					names:             []string{"source"},
					envVars:           []string{"TG_SOURCE"},
					deprecatedEnvVars: []string{"TERRAGRUNT_SOURCE"},
				},
				{
					names:             []string{"source-map"},
					envVars:           []string{"TG_SOURCE_MAP"},
					deprecatedEnvVars: []string{"TERRAGRUNT_SOURCE_MAP"},
				},
				{
					names:             []string{"tf-path"},
					envVars:           []string{"TG_TF_PATH"},
					deprecatedEnvVars: []string{"TERRAGRUNT_TFPATH"},
				},
			},
		},
		{
			name:     "find",
			aliases:  []string{"fd"},
			category: commands.DiscoveryCommandsCategoryName,
			flags: slices.Concat(backendFlags, filterFlags, []cliFlag{
				featureFlag,
				{names: []string{"dag"}, envVars: []string{"TG_DAG"}},
				{names: []string{"dependencies"}, envVars: []string{"TG_DEPENDENCIES"}},
				{names: []string{"exclude"}, envVars: []string{"TG_EXCLUDE"}},
				{names: []string{"external"}, envVars: []string{"TG_EXTERNAL"}, hidden: true},
				{names: []string{"format"}, envVars: []string{"TG_FORMAT"}},
				{names: []string{"hidden"}, envVars: []string{"TG_HIDDEN"}, hidden: true},
				{names: []string{"include"}, envVars: []string{"TG_INCLUDE"}},
				{names: []string{"json", "j"}, envVars: []string{"TG_JSON"}},
				{
					names:   []string{"no-discovery-auth-provider-cmd"},
					envVars: []string{"TG_NO_DISCOVERY_AUTH_PROVIDER_CMD"},
				},
				{names: []string{"no-hidden"}, envVars: []string{"TG_NO_HIDDEN"}},
				{names: []string{"queue-construct-as", "as"}, envVars: []string{"TG_QUEUE_CONSTRUCT_AS"}},
				{names: []string{"reading"}, envVars: []string{"TG_READING"}},
			}),
		},
		{
			name:          "force-unlock",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "hcl",
			category: commands.ConfigurationCommandsCategoryName,
			subcommands: []cliCommand{
				{
					name:    "format",
					aliases: []string{"fmt"},
					flags: slices.Concat(filterFlags, queueFlags, []cliFlag{
						{
							names:             []string{"check"},
							envVars:           []string{"TG_CHECK"},
							deprecatedEnvVars: []string{"TG_HCLFMT_CHECK", "TERRAGRUNT_CHECK"},
						},
						{
							names:             []string{"diff"},
							envVars:           []string{"TG_DIFF"},
							deprecatedEnvVars: []string{"TG_HCLFMT_DIFF", "TERRAGRUNT_DIFF"},
						},
						{
							names:             []string{"exclude-dir"},
							envVars:           []string{"TG_EXCLUDE_DIR"},
							deprecatedEnvVars: []string{"TG_HCLFMT_EXCLUDE_DIR", "TERRAGRUNT_HCLFMT_EXCLUDE_DIR"},
						},
						{
							names:             []string{"file"},
							envVars:           []string{"TG_FILE"},
							deprecatedEnvVars: []string{"TG_HCLFMT_FILE", "TERRAGRUNT_HCLFMT_FILE"},
						},
						{
							names:             []string{"parallelism"},
							envVars:           []string{"TG_PARALLELISM"},
							deprecatedEnvVars: []string{"TERRAGRUNT_PARALLELISM"},
						},
						{
							names:             []string{"stdin"},
							envVars:           []string{"TG_STDIN"},
							deprecatedEnvVars: []string{"TG_HCLFMT_STDIN", "TERRAGRUNT_HCLFMT_STDIN"},
						},
					}),
				},
				{
					name: "validate",
					flags: slices.Concat(filterFlags, queueFlags, []cliFlag{
						{names: []string{"inputs"}, envVars: []string{"TG_INPUTS"}},
						{
							names:             []string{"json"},
							envVars:           []string{"TG_JSON"},
							deprecatedEnvVars: []string{"TG_HCLVALIDATE_JSON", "TERRAGRUNT_HCLVALIDATE_JSON"},
						},
						{
							names:             []string{"show-config-path"},
							envVars:           []string{"TG_SHOW_CONFIG_PATH"},
							deprecatedEnvVars: []string{"TG_HCLVALIDATE_SHOW_CONFIG_PATH", "TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH"},
						},
						{
							names:             []string{"strict"},
							envVars:           []string{"TG_STRICT"},
							deprecatedEnvVars: []string{"TG_STRICT_VALIDATE", "TG_HCLVALIDATE_STRICT_VALIDATE", "TERRAGRUNT_STRICT_VALIDATE"},
						},
						{
							names:             []string{"tf-path"},
							envVars:           []string{"TG_TF_PATH"},
							deprecatedEnvVars: []string{"TERRAGRUNT_TFPATH"},
						},
					}),
				},
			},
		},
		{
			name:     "help",
			category: commands.ConfigurationCommandsCategoryName,
			hidden:   true,
		},
		{
			name:          "import",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "info",
			category: commands.ConfigurationCommandsCategoryName,
			subcommands: []cliCommand{
				{
					name: "print",
					flags: slices.Concat(runFlags, []cliFlag{
						allFlag,
					}),
				},
				{
					name: "strict",
					subcommands: []cliCommand{
						{
							name: "list",
							flags: []cliFlag{
								{names: []string{"all"}, envVars: []string{"TG_ALL"}},
							},
						},
					},
				},
			},
		},
		{
			name:          "init",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "list",
			aliases:  []string{"ls"},
			category: commands.DiscoveryCommandsCategoryName,
			flags: slices.Concat(filterFlags, []cliFlag{
				{
					names:             []string{"backend-bootstrap"},
					envVars:           []string{"TG_BACKEND_BOOTSTRAP"},
					deprecatedEnvVars: []string{"TG_LIST_BACKEND_BOOTSTRAP"},
				},
				{
					names:             []string{"backend-require-bootstrap"},
					envVars:           []string{"TG_BACKEND_REQUIRE_BOOTSTRAP"},
					deprecatedEnvVars: []string{"TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION", "TG_LIST_BACKEND_REQUIRE_BOOTSTRAP"},
				},
				{names: []string{"dag"}, envVars: []string{"TG_DAG"}, deprecatedEnvVars: []string{"TG_LIST_DAG"}},
				{
					names:             []string{"dependencies"},
					envVars:           []string{"TG_DEPENDENCIES"},
					deprecatedEnvVars: []string{"TG_LIST_DEPENDENCIES"},
				},
				{
					names:             []string{"disable-bucket-update"},
					envVars:           []string{"TG_DISABLE_BUCKET_UPDATE"},
					deprecatedEnvVars: []string{"TERRAGRUNT_DISABLE_BUCKET_UPDATE", "TG_LIST_DISABLE_BUCKET_UPDATE"},
				},
				{
					names:             []string{"external"},
					envVars:           []string{"TG_EXTERNAL"},
					deprecatedEnvVars: []string{"TG_LIST_EXTERNAL"},
					hidden:            true,
				},
				{
					names:             []string{"feature"},
					envVars:           []string{"TG_FEATURE"},
					deprecatedEnvVars: []string{"TERRAGRUNT_FEATURE", "TG_LIST_FEATURE"},
				},
				{
					names:             []string{"format"},
					envVars:           []string{"TG_FORMAT"},
					deprecatedEnvVars: []string{"TG_LIST_FORMAT"},
				},
				{
					names:             []string{"hidden"},
					envVars:           []string{"TG_HIDDEN"},
					deprecatedEnvVars: []string{"TG_LIST_HIDDEN"},
					hidden:            true,
				},
				{
					names:             []string{"long", "l"},
					envVars:           []string{"TG_LONG"},
					deprecatedEnvVars: []string{"TG_LIST_LONG"},
				},
				{
					names:             []string{"no-discovery-auth-provider-cmd"},
					envVars:           []string{"TG_NO_DISCOVERY_AUTH_PROVIDER_CMD"},
					deprecatedEnvVars: []string{"TG_LIST_NO_DISCOVERY_AUTH_PROVIDER_CMD"},
				},
				{
					names:             []string{"no-hidden"},
					envVars:           []string{"TG_NO_HIDDEN"},
					deprecatedEnvVars: []string{"TG_LIST_NO_HIDDEN"},
				},
				{
					names:             []string{"queue-construct-as", "as"},
					envVars:           []string{"TG_QUEUE_CONSTRUCT_AS"},
					deprecatedEnvVars: []string{"TG_LIST_QUEUE_CONSTRUCT_AS"},
				},
				{
					names:             []string{"tree", "T"},
					envVars:           []string{"TG_TREE"},
					deprecatedEnvVars: []string{"TG_LIST_TREE"},
				},
			}),
		},
		{
			name:     "login",
			category: commands.PortalCommandsCategoryName,
			flags: []cliFlag{
				{
					names:             []string{"force"},
					envVars:           []string{"TG_FORCE"},
					deprecatedEnvVars: []string{"TG_LOGIN_FORCE"},
				},
			},
		},
		{
			name:     "mcp",
			category: commands.ConfigurationCommandsCategoryName,
			flags: []cliFlag{
				{names: []string{"allow"}, envVars: []string{"TG_ALLOW"}},
				{names: []string{"allow-cmd"}, envVars: []string{"TG_ALLOW_CMD"}},
				{names: []string{"dangerously-allow-apply"}, envVars: []string{"TG_DANGEROUSLY_ALLOW_APPLY"}},
				{
					names:             []string{"tf-path"},
					envVars:           []string{"TG_TF_PATH"},
					deprecatedEnvVars: []string{"TERRAGRUNT_TFPATH"},
				},
			},
		},
		{
			name:          "output",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:          "plan",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:          "refresh",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "render",
			category: commands.ConfigurationCommandsCategoryName,
			flags: slices.Concat(runFlags, []cliFlag{
				{
					names:             []string{"all", "a"},
					envVars:           []string{"TG_ALL"},
					deprecatedEnvVars: []string{"TG_RENDER_ALL"},
				},
				{
					names:             []string{"disable-dependent-modules"},
					envVars:           []string{"TG_DISABLE_DEPENDENT_MODULES"},
					deprecatedEnvVars: []string{"TG_RENDER_JSON_DISABLE_DEPENDENT_MODULES", "TERRAGRUNT_JSON_DISABLE_DEPENDENT_MODULES", "TG_RENDER_DISABLE_DEPENDENT_MODULES"},
					hidden:            true,
				},
				{
					names:             []string{"format"},
					envVars:           []string{"TG_FORMAT"},
					deprecatedEnvVars: []string{"TG_RENDER_FORMAT"},
				},
				{names: []string{"json"}, envVars: []string{"TG_JSON"}, deprecatedEnvVars: []string{"TG_RENDER_JSON"}},
				{
					names:             []string{"out"},
					envVars:           []string{"TG_OUT"},
					deprecatedNames:   []string{"json-out"},
					deprecatedEnvVars: []string{"TG_RENDER_JSON_OUT", "TERRAGRUNT_JSON_OUT", "TG_RENDER_OUT"},
				},
				{
					names:             []string{"with-metadata"},
					envVars:           []string{"TG_WITH_METADATA"},
					deprecatedEnvVars: []string{"TG_RENDER_JSON_WITH_METADATA", "TERRAGRUNT_WITH_METADATA", "TG_RENDER_WITH_METADATA"},
				},
				{
					names:             []string{"write", "w"},
					envVars:           []string{"TG_WRITE"},
					deprecatedEnvVars: []string{"TG_RENDER_WRITE"},
				},
			}),
		},
		{
			name:     "run",
			category: commands.MainCommandsCategoryName,
			flags:    runCommandFlags,
			subcommands: []cliCommand{
				{name: "apply", showsTofuHelp: true},
				{name: "console", showsTofuHelp: true},
				{name: "destroy", showsTofuHelp: true},
				{name: "env", hidden: true, showsTofuHelp: true},
				{name: "fmt", showsTofuHelp: true},
				{name: "force-unlock", showsTofuHelp: true},
				{name: "get", showsTofuHelp: true},
				{name: "graph", showsTofuHelp: true},
				{name: "import", showsTofuHelp: true},
				{name: "init", showsTofuHelp: true},
				{name: "login", showsTofuHelp: true},
				{name: "logout", showsTofuHelp: true},
				{name: "metadata", showsTofuHelp: true},
				{name: "output", showsTofuHelp: true},
				{name: "plan", showsTofuHelp: true},
				{name: "providers", showsTofuHelp: true},
				{name: "push", hidden: true, showsTofuHelp: true},
				{name: "query", hidden: true, showsTofuHelp: true},
				{name: "refresh", showsTofuHelp: true},
				{name: "show", showsTofuHelp: true},
				{name: "state", showsTofuHelp: true},
				{name: "taint", showsTofuHelp: true},
				{name: "test", showsTofuHelp: true},
				{name: "untaint", showsTofuHelp: true},
				{name: "validate", showsTofuHelp: true},
				{name: "version", showsTofuHelp: true},
				{name: "workspace", showsTofuHelp: true},
			},
		},
		{
			name:     "scaffold",
			category: commands.CatalogCommandsCategoryName,
			flags: slices.Concat(backendFlags, []cliFlag{
				featureFlag,
				{names: []string{"no-dependency-prompt"}, envVars: []string{"TG_NO_DEPENDENCY_PROMPT"}},
				{names: []string{"no-hooks"}, envVars: []string{"TG_NO_HOOKS"}},
				{names: []string{"no-include-root"}, envVars: []string{"TG_NO_INCLUDE_ROOT"}},
				{names: []string{"no-shell"}, envVars: []string{"TG_NO_SHELL"}},
				{names: []string{"output-folder"}},
				{names: []string{"root-file-name"}, envVars: []string{"TG_ROOT_FILE_NAME"}},
				{names: []string{"var"}, envVars: []string{"TG_VAR"}},
				{names: []string{"var-file"}, envVars: []string{"TG_VAR_FILE"}},
			}),
		},
		{
			name:          "show",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "stack",
			category: commands.MainCommandsCategoryName,
			subcommands: []cliCommand{
				{name: "clean"},
				{
					name: "generate",
					flags: slices.Concat(runFlags, []cliFlag{
						{names: []string{"no-stack-validate"}, envVars: []string{"TG_NO_STACK_VALIDATE"}, hidden: true},
					}),
				},
				{
					name: "output",
					flags: slices.Concat(runFlags, []cliFlag{
						{names: []string{"format"}, envVars: []string{"TG_FORMAT"}},
						{names: []string{"json"}},
						{names: []string{"no-stack-validate"}, envVars: []string{"TG_NO_STACK_VALIDATE"}, hidden: true},
						{names: []string{"raw"}},
					}),
				},
				{
					name: "run",
					flags: slices.Concat(runFlags, []cliFlag{
						{names: []string{"no-stack-validate"}, envVars: []string{"TG_NO_STACK_VALIDATE"}, hidden: true},
					}),
				},
			},
		},
		{
			name:          "state",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:          "test",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:          "validate",
			category:      commands.ShortcutsCommandsCategoryName,
			showsTofuHelp: true,
			flags:         runCommandFlags,
		},
		{
			name:     "version",
			category: commands.ConfigurationCommandsCategoryName,
			hidden:   true,
		},
	},
}

// globalFlags are the flags of the terragrunt app itself, which help lists
// under Global Options.
var globalFlags = []cliFlag{
	{envVars: []string{"TG_TELEMETRY_LOGS_EXPORTER"}},
	{envVars: []string{"TG_TELEMETRY_LOGS_EXPORTER_INSECURE_ENDPOINT"}},
	{
		envVars:           []string{"TG_TELEMETRY_METRIC_EXPORTER"},
		deprecatedEnvVars: []string{"TERRAGRUNT_TELEMETRY_METRIC_EXPORTER", "TERRAGRUNT_TELEMERTY_METRIC_EXPORTER"},
	},
	{
		envVars:           []string{"TG_TELEMETRY_METRIC_EXPORTER_INSECURE_ENDPOINT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_TELEMETRY_METRIC_EXPORTER_INSECURE_ENDPOINT", "TERRAGRUNT_TELEMERTY_METRIC_EXPORTER_INSECURE_ENDPOINT"},
	},
	{
		envVars:           []string{"TG_TELEMETRY_TRACE_EXPORTER"},
		deprecatedEnvVars: []string{"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER", "TERRAGRUNT_TELEMERTY_TRACE_EXPORTER"},
	},
	{
		envVars:           []string{"TG_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_HTTP_ENDPOINT", "TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_HTTP_ENDPOINT"},
	},
	{
		envVars:           []string{"TG_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_TELEMETRY_TRACE_EXPORTER_INSECURE_ENDPOINT", "TERRAGRUNT_TELEMERTY_TRACE_EXPORTER_INSECURE_ENDPOINT"},
	},
	{envVars: []string{"TRACEPARENT"}},
	{
		names:             []string{"experiment"},
		envVars:           []string{"TG_EXPERIMENT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_EXPERIMENT"},
	},
	{
		names:             []string{"experiment-mode"},
		envVars:           []string{"TG_EXPERIMENT_MODE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_EXPERIMENT_MODE"},
	},
	{names: []string{"help", "h"}},
	{
		names:             []string{"log-custom-format"},
		envVars:           []string{"TG_LOG_CUSTOM_FORMAT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_LOG_CUSTOM_FORMAT"},
	},
	{
		names:             []string{"log-disable"},
		envVars:           []string{"TG_LOG_DISABLE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_LOG_DISABLE"},
	},
	{
		names:             []string{"log-format"},
		envVars:           []string{"TG_LOG_FORMAT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_LOG_FORMAT", "TERRAGRUNT_DISABLE_LOG_FORMATTING", "TERRAGRUNT_JSON_LOG", "TERRAGRUNT_TF_LOGS_TO_JSON"},
	},
	{
		names:             []string{"log-level"},
		envVars:           []string{"TG_LOG_LEVEL"},
		deprecatedEnvVars: []string{"TERRAGRUNT_LOG_LEVEL"},
	},
	{
		names:             []string{"log-show-abs-paths"},
		envVars:           []string{"TG_LOG_SHOW_ABS_PATHS"},
		deprecatedEnvVars: []string{"TERRAGRUNT_LOG_SHOW_ABS_PATHS"},
	},
	{names: []string{"no-color"}, envVars: []string{"TG_NO_COLOR"}, deprecatedEnvVars: []string{"TERRAGRUNT_NO_COLOR"}},
	{names: []string{"no-tip"}, envVars: []string{"TG_NO_TIP"}},
	{names: []string{"no-tips"}, envVars: []string{"TG_NO_TIPS"}},
	{
		names:             []string{"non-interactive"},
		envVars:           []string{"TG_NON_INTERACTIVE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_NON_INTERACTIVE", "TF_INPUT"},
	},
	{names: []string{"profile-cpu"}, envVars: []string{"TG_PROFILE_CPU"}},
	{names: []string{"profile-dir"}, envVars: []string{"TG_PROFILE_DIR"}},
	{names: []string{"profile-goroutine"}, envVars: []string{"TG_PROFILE_GOROUTINE"}},
	{names: []string{"profile-mem"}, envVars: []string{"TG_PROFILE_MEM"}},
	{
		names:             []string{"strict-control"},
		envVars:           []string{"TG_STRICT_CONTROL"},
		deprecatedEnvVars: []string{"TERRAGRUNT_STRICT_CONTROL"},
	},
	{
		names:             []string{"strict-mode"},
		envVars:           []string{"TG_STRICT_MODE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_STRICT_MODE"},
	},
	{names: []string{"version", "v"}},
	{
		names:             []string{"working-dir"},
		envVars:           []string{"TG_WORKING_DIR"},
		deprecatedEnvVars: []string{"TERRAGRUNT_WORKING_DIR"},
	},
}

var allFlag = cliFlag{names: []string{"all", "a"}, envVars: []string{"TG_ALL"}}

var graphFlag = cliFlag{names: []string{"graph"}, envVars: []string{"TG_GRAPH"}}

var featureFlag = cliFlag{names: []string{"feature"}, envVars: []string{"TG_FEATURE"}, deprecatedEnvVars: []string{"TERRAGRUNT_FEATURE"}}

// backendFlags control how commands that read units bootstrap and update
// their remote state backend.
var backendFlags = []cliFlag{
	{names: []string{"backend-bootstrap"}, envVars: []string{"TG_BACKEND_BOOTSTRAP"}},
	{
		names:             []string{"backend-require-bootstrap"},
		envVars:           []string{"TG_BACKEND_REQUIRE_BOOTSTRAP"},
		deprecatedEnvVars: []string{"TERRAGRUNT_FAIL_ON_STATE_BUCKET_CREATION"},
	},
	{
		names:             []string{"disable-bucket-update"},
		envVars:           []string{"TG_DISABLE_BUCKET_UPDATE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_DISABLE_BUCKET_UPDATE"},
	},
}

// filterFlags select the components a command operates on.
var filterFlags = []cliFlag{
	{names: []string{"discovery-boundary"}, envVars: []string{"TG_DISCOVERY_BOUNDARY"}},
	{names: []string{"filter"}, envVars: []string{"TG_FILTER"}},
	{names: []string{"filter-affected"}, envVars: []string{"TG_FILTER_AFFECTED"}},
	{names: []string{"filter-allow-destroy"}, envVars: []string{"TG_FILTER_ALLOW_DESTROY"}},
	{names: []string{"filters-file"}, envVars: []string{"TG_FILTERS_FILE"}},
	{names: []string{"no-filters-file"}, envVars: []string{"TG_NO_FILTERS_FILE"}},
}

// queueFlags shape the queue of units a command runs.
var queueFlags = []cliFlag{
	{
		names:             []string{"queue-exclude-dir"},
		envVars:           []string{"TG_QUEUE_EXCLUDE_DIR"},
		deprecatedEnvVars: []string{"TERRAGRUNT_EXCLUDE_DIR"},
		hidden:            true,
	},
	{
		names:             []string{"queue-exclude-external"},
		envVars:           []string{"TG_QUEUE_EXCLUDE_EXTERNAL"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IGNORE_EXTERNAL_DEPENDENCIES"},
		hidden:            true,
	},
	{
		names:             []string{"queue-excludes-file"},
		envVars:           []string{"TG_QUEUE_EXCLUDES_FILE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_EXCLUDES_FILE"},
		hidden:            true,
	},
	{
		names:             []string{"queue-ignore-dag-order"},
		envVars:           []string{"TG_QUEUE_IGNORE_DAG_ORDER"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IGNORE_DEPENDENCY_ORDER"},
	},
	{
		names:             []string{"queue-ignore-errors"},
		envVars:           []string{"TG_QUEUE_IGNORE_ERRORS"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IGNORE_DEPENDENCY_ERRORS"},
	},
	{
		names:             []string{"queue-include-dir"},
		envVars:           []string{"TG_QUEUE_INCLUDE_DIR"},
		deprecatedEnvVars: []string{"TERRAGRUNT_INCLUDE_DIR"},
		hidden:            true,
	},
	{
		names:             []string{"queue-include-external"},
		envVars:           []string{"TG_QUEUE_INCLUDE_EXTERNAL"},
		deprecatedEnvVars: []string{"TERRAGRUNT_INCLUDE_EXTERNAL_DEPENDENCIES"},
		hidden:            true,
	},
	{
		names:             []string{"queue-include-units-reading"},
		envVars:           []string{"TG_QUEUE_INCLUDE_UNITS_READING"},
		deprecatedEnvVars: []string{"TERRAGRUNT_QUEUE_INCLUDE_UNITS_READING"},
		hidden:            true,
	},
	{
		names:             []string{"queue-strict-include"},
		envVars:           []string{"TG_QUEUE_STRICT_INCLUDE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_STRICT_INCLUDE"},
		hidden:            true,
	},
}

// runFlags are the flags of run, except --all and --graph, that the commands
// built on run share.
var runFlags = slices.Concat(backendFlags, filterFlags, queueFlags, []cliFlag{
	featureFlag,
	{
		names:             []string{"auth-provider-cmd"},
		envVars:           []string{"TG_AUTH_PROVIDER_CMD"},
		deprecatedEnvVars: []string{"TERRAGRUNT_AUTH_PROVIDER_CMD"},
	},
	{names: []string{"cas-clone-depth"}, envVars: []string{"TG_CAS_CLONE_DEPTH"}},
	{names: []string{"cas-offline"}, envVars: []string{"TG_CAS_OFFLINE"}},
	{names: []string{"cas-probe-ttl"}, envVars: []string{"TG_CAS_PROBE_TTL"}},
	{names: []string{"cas-refresh"}, envVars: []string{"TG_CAS_REFRESH"}},
	{names: []string{"config"}, envVars: []string{"TG_CONFIG"}, deprecatedEnvVars: []string{"TERRAGRUNT_CONFIG"}},
	{
		names:             []string{"dependency-fetch-output-from-state"},
		envVars:           []string{"TG_DEPENDENCY_FETCH_OUTPUT_FROM_STATE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_FETCH_DEPENDENCY_OUTPUT_FROM_STATE"},
	},
	{names: []string{"destroy-dependencies-check"}, envVars: []string{"TG_DESTROY_DEPENDENCIES_CHECK"}},
	{
		names:             []string{"disable-command-validation"},
		envVars:           []string{"TG_DISABLE_COMMAND_VALIDATION"},
		deprecatedEnvVars: []string{"TERRAGRUNT_DISABLE_COMMAND_VALIDATION"},
		hidden:            true,
	},
	{
		names:             []string{"download-dir"},
		envVars:           []string{"TG_DOWNLOAD_DIR"},
		deprecatedEnvVars: []string{"TERRAGRUNT_DOWNLOAD", "TERRAGRUNT_DOWNLOAD_DIR"},
	},
	{
		names:             []string{"engine-cache-path"},
		envVars:           []string{"TG_ENGINE_CACHE_PATH"},
		deprecatedEnvVars: []string{"TERRAGRUNT_ENGINE_CACHE_PATH"},
		hidden:            true,
	},
	{
		names:             []string{"engine-log-level"},
		envVars:           []string{"TG_ENGINE_LOG_LEVEL"},
		deprecatedEnvVars: []string{"TERRAGRUNT_ENGINE_LOG_LEVEL"},
		hidden:            true,
	},
	{
		names:             []string{"engine-skip-check"},
		envVars:           []string{"TG_ENGINE_SKIP_CHECK"},
		deprecatedEnvVars: []string{"TERRAGRUNT_ENGINE_SKIP_CHECK"},
		hidden:            true,
	},
	{
		names:             []string{"experimental-engine"},
		envVars:           []string{"TG_EXPERIMENTAL_ENGINE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_EXPERIMENTAL_ENGINE"},
		hidden:            true,
	},
	{names: []string{"fail-fast"}, envVars: []string{"TG_FAIL_FAST"}},
	{
		names:             []string{"graph-root"},
		envVars:           []string{"TG_GRAPH_ROOT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_GRAPH_ROOT"},
	},
	{
		names:             []string{"iam-assume-role"},
		envVars:           []string{"TG_IAM_ASSUME_ROLE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IAM_ROLE"},
	},
	{
		names:             []string{"iam-assume-role-duration"},
		envVars:           []string{"TG_IAM_ASSUME_ROLE_DURATION"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IAM_ASSUME_ROLE_DURATION"},
	},
	{
		names:             []string{"iam-assume-role-session-name"},
		envVars:           []string{"TG_IAM_ASSUME_ROLE_SESSION_NAME"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME"},
	},
	{
		names:             []string{"iam-assume-role-web-identity-token"},
		envVars:           []string{"TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"},
		deprecatedEnvVars: []string{"TERRAGRUNT_IAM_WEB_IDENTITY_TOKEN", "TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"},
	},
	{
		names:             []string{"inputs-debug"},
		envVars:           []string{"TG_INPUTS_DEBUG"},
		deprecatedEnvVars: []string{"TERRAGRUNT_DEBUG"},
	},
	{
		names:             []string{"json-out-dir"},
		envVars:           []string{"TG_JSON_OUT_DIR"},
		deprecatedEnvVars: []string{"TERRAGRUNT_JSON_OUT_DIR"},
	},
	{
		names:             []string{"no-auto-approve"},
		envVars:           []string{"TG_NO_AUTO_APPROVE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_AUTO_APPROVE"},
	},
	{
		names:             []string{"no-auto-init"},
		envVars:           []string{"TG_NO_AUTO_INIT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_AUTO_INIT"},
	},
	{names: []string{"no-auto-provider-cache-dir"}, envVars: []string{"TG_NO_AUTO_PROVIDER_CACHE_DIR"}},
	{
		names:             []string{"no-auto-retry"},
		envVars:           []string{"TG_NO_AUTO_RETRY"},
		deprecatedEnvVars: []string{"TERRAGRUNT_AUTO_RETRY"},
	},
	{names: []string{"no-cas"}, envVars: []string{"TG_NO_CAS"}},
	{
		names:   []string{"no-dependency-fetch-output-from-state"},
		envVars: []string{"TG_NO_DEPENDENCY_FETCH_OUTPUT_FROM_STATE"},
	},
	{names: []string{"no-dependency-outputs"}, envVars: []string{"TG_NO_DEPENDENCY_OUTPUTS"}},
	{
		names:   []string{"no-destroy-dependencies-check"},
		envVars: []string{"TG_NO_DESTROY_DEPENDENCIES_CHECK"},
		hidden:  true,
	},
	{names: []string{"no-discovery-auth-provider-cmd"}, envVars: []string{"TG_NO_DISCOVERY_AUTH_PROVIDER_CMD"}},
	{names: []string{"no-engine"}, envVars: []string{"TG_NO_ENGINE"}, hidden: true},
	{names: []string{"no-hooks"}, envVars: []string{"TG_NO_HOOKS"}},
	{names: []string{"no-stack-generate"}, envVars: []string{"TG_NO_STACK_GENERATE"}},
	{names: []string{"out-dir"}, envVars: []string{"TG_OUT_DIR"}, deprecatedEnvVars: []string{"TERRAGRUNT_OUT_DIR"}},
	{
		names:             []string{"parallelism"},
		envVars:           []string{"TG_PARALLELISM"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PARALLELISM"},
	},
	{
		names:             []string{"provider-cache"},
		envVars:           []string{"TG_PROVIDER_CACHE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PROVIDER_CACHE"},
	},
	{
		names:             []string{"provider-cache-dir"},
		envVars:           []string{"TG_PROVIDER_CACHE_DIR"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PROVIDER_CACHE_DIR"},
	},
	{
		names:             []string{"provider-cache-hostname"},
		envVars:           []string{"TG_PROVIDER_CACHE_HOSTNAME"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PROVIDER_CACHE_HOSTNAME"},
	},
	{
		names:             []string{"provider-cache-port"},
		envVars:           []string{"TG_PROVIDER_CACHE_PORT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PROVIDER_CACHE_PORT"},
	},
	{
		names:             []string{"provider-cache-registry-names"},
		envVars:           []string{"TG_PROVIDER_CACHE_REGISTRY_NAMES"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PROVIDER_CACHE_REGISTRY_NAMES"},
	},
	{
		names:             []string{"provider-cache-token"},
		envVars:           []string{"TG_PROVIDER_CACHE_TOKEN"},
		deprecatedEnvVars: []string{"TERRAGRUNT_PROVIDER_CACHE_TOKEN"},
	},
	{names: []string{"report-file"}, envVars: []string{"TG_REPORT_FILE"}},
	{names: []string{"report-format"}, envVars: []string{"TG_REPORT_FORMAT"}},
	{names: []string{"report-schema-file"}, envVars: []string{"TG_REPORT_SCHEMA_FILE"}},
	{names: []string{"source"}, envVars: []string{"TG_SOURCE"}, deprecatedEnvVars: []string{"TERRAGRUNT_SOURCE"}},
	{
		names:             []string{"source-map"},
		envVars:           []string{"TG_SOURCE_MAP"},
		deprecatedEnvVars: []string{"TERRAGRUNT_SOURCE_MAP"},
	},
	{
		names:             []string{"source-update"},
		envVars:           []string{"TG_SOURCE_UPDATE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_SOURCE_UPDATE"},
	},
	{names: []string{"summary-disable"}, envVars: []string{"TG_SUMMARY_DISABLE"}},
	{names: []string{"summary-per-unit"}, envVars: []string{"TG_SUMMARY_PER_UNIT"}},
	{
		names:             []string{"tf-forward-stdout"},
		envVars:           []string{"TG_TF_FORWARD_STDOUT"},
		deprecatedEnvVars: []string{"TERRAGRUNT_FORWARD_TF_STDOUT", "TERRAGRUNT_INCLUDE_MODULE_PREFIX"},
	},
	{names: []string{"tf-path"}, envVars: []string{"TG_TF_PATH"}, deprecatedEnvVars: []string{"TERRAGRUNT_TFPATH"}},
	{
		names:             []string{"units-that-include"},
		envVars:           []string{"TG_UNITS_THAT_INCLUDE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_MODULES_THAT_INCLUDE"},
		hidden:            true,
	},
	{
		names:             []string{"use-partial-parse-config-cache"},
		envVars:           []string{"TG_USE_PARTIAL_PARSE_CONFIG_CACHE"},
		deprecatedEnvVars: []string{"TERRAGRUNT_USE_PARTIAL_PARSE_CONFIG_CACHE"},
	},
	{names: []string{"version-manager-file-name"}, envVars: []string{"TG_VERSION_MANAGER_FILE_NAME"}},
})

// runCommandFlags are the flags of run and of the OpenTofu shortcuts.
var runCommandFlags = slices.Concat(runFlags, []cliFlag{allFlag, graphFlag})
