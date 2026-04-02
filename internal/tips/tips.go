package tips

const (
	// DebuggingDocs is the tip that points users to the debugging documentation.
	DebuggingDocs = "debugging-docs"

	// WindowsSymlinkWarning is the tip that warns Windows users about symlinks.
	WindowsSymlinkWarning = "windows-symlink-warning"

	// WindowsSymlinkWarningMessage is the default message for the Windows symlink warning tip.
	WindowsSymlinkWarningMessage = "Windows users may encounter silent fallback behavior to provider copying " +
		"instead of symlinking in OpenTofu/Terraform. " +
		"See https://github.com/gruntwork-io/terragrunt/issues/5061 for more information."

	// WindowsSymlinkWarningOpenTofuMessage is the OpenTofu-specific message for the Windows symlink warning tip,
	// shown when the user is running OpenTofu >= 1.12.0.
	WindowsSymlinkWarningOpenTofuMessage = "Windows users may encounter silent fallback from symlinking to " +
		"copying for provider plugins. " +
		"Set TF_LOG=warn to check if OpenTofu is falling back to copying. " +
		"See https://github.com/gruntwork-io/terragrunt/issues/5061 for more information."
)

// NewTips returns a new Tips collection with all available tips.
//
// Never remove any of these tips, as removing them will cause a breaking change for users
// using an invocation of `--no-tip` pointing to a non-existent tip.
//
// e.g. `terragrunt run --no-tip=debugging-docs`
//
// If you want to programmatically document that a tip should no longer be
// used after removing it from the codebase, just set `disabled` to `1` here for that tip.
func NewTips() Tips {
	return Tips{
		{
			Name:    DebuggingDocs,
			Message: "For help troubleshooting errors, visit https://docs.terragrunt.com/troubleshooting/debugging",
		},
		{
			Name:    WindowsSymlinkWarning,
			Message: WindowsSymlinkWarningMessage,
		},
	}
}
