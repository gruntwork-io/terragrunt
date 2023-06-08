package cli

type Config struct {
	WorkingDir       string
	DownloadDir      string
	Parallelism      int
	NonInteractive   bool
	Debug            bool
	LogLevel         string
	LogDisableColors bool

	ConfigPath                     string
	HclFilePath                    string
	SourceUpdate                   bool
	IgnoreDependencyErrors         bool
	IgnoreDependencyOrder          bool
	IgnoreExternalDependencies     bool
	IncludeExternalDependencies    bool
	StrictInclude                  bool
	ExcludeDirs                    []string
	IncludeDirs                    []string
	ModulesThatInclude             []string
	ValidateStrictMode             bool
	IncludeModulePrefix            bool
	AutoApprove                    bool
	AutoInit                       bool
	AutoRetry                      bool
	Check                          bool
	Diff                           bool
	FetchDependencyOutputFromState bool
	UsePartialParseConfigCache     bool
	OutputWithMetadata             bool
	JSONOut                        string
	IAMRole                        string
	IAMAssumeRoleDuration          int
	IAMAssumeRoleSessionName       string

	TerraformPath      string
	TerraformSource    string
	TerraformSourceMap string

	AWSProviderPatchOverrides string
}
