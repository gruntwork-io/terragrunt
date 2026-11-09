package tips

const (
	// DebuggingDocs is the tip that points users to the debugging documentation.
	DebuggingDocs = "debugging-docs"

	// WindowsSymlinkWarning is the tip that warns Windows users about symlinks.
	WindowsSymlinkWarning = "windows-symlink-warning"

	// StackTargetMissingTypeStack is the tip shown when a `--filter` path targets a
	// stack directory but the filter is not restricted to stacks via `| type=stack`.
	StackTargetMissingTypeStack = "stack-target-missing-type-stack"

	// StackTargetMissingTypeStackMessage is the default message for the
	// stack-target-missing-type-stack tip. The runtime message that is shown to the
	// user is assembled at evaluation time so it can list the offending filters and
	// the suggested rewrites.
	StackTargetMissingTypeStackMessage = "One or more --filter paths target a stack directory " +
		"but the filter is not restricted to stacks. Without `| type=stack`, " +
		"`stack generate` will ignore the filter and `run` will not generate just that stack. " +
		"See https://docs.terragrunt.com/features/filter/#stack-generate"

	// StackNestedStacksNotGenerated is the tip shown when a literal (non-glob) path
	// with `| type=stack` targets a stack whose nested stacks were not themselves
	// recursively generated.
	StackNestedStacksNotGenerated = "stack-filter-nested-not-generated"

	// StackNestedStacksNotGeneratedMessage is the default message for the
	// stack-filter-nested-not-generated tip. The runtime message is assembled at
	// evaluation time so it can list the suggested recursive filters.
	StackNestedStacksNotGeneratedMessage = "Filtering a stack with `| type=stack` generates only that stack, " +
		"not the nested stacks it contains. To generate the nested stacks too, also add a recursive path filter."

	// MissingDependencyConfig is the tip shown when run --all fails because a `dependency`
	// block points at a unit that the diff of a Git-based filter deleted or moved.
	MissingDependencyConfig = "missing-dependency-config"

	// MissingDependencyConfigMessage is the default message for the
	// missing-dependency-config tip. The runtime message is assembled at
	// evaluation time so it can name the deleted component and the reference it exists at.
	MissingDependencyConfigMessage = "A `dependency` block points at a unit or stack " +
		"that the Git diff deleted or moved. " + missingDependencyConfigFix

	// missingDependencyConfigFix is the advice both missing-dependency-config messages end with.
	missingDependencyConfigFix = "Update or remove that block. `terragrunt hcl validate` lists every such block."

	// InputsReadNotTracked is the tip shown when read tracking is on and a configuration calls a
	// file-reading function in `inputs`, where Terragrunt does not record reads.
	InputsReadNotTracked = "inputs-read-not-tracked"

	// InputsReadNotTrackedMessage is the default message for the inputs-read-not-tracked tip.
	// The runtime message is assembled at evaluation time so it can list the reads it found.
	InputsReadNotTrackedMessage = "Terragrunt does not track files read in `inputs`, " +
		"so reading and Git filters miss the units and stacks that read them. " +
		"Read each file in `locals` and reference the local from `inputs`. " +
		"In a stack file, read it in the stack's `locals` and reference the local from the `autoinclude` block. " +
		"See https://docs.terragrunt.com/features/filter/attributes#reading-based-expressions"

	OutputsBlock = "outputs-block"

	OutputsBlockMessage = "A `dependency` block reads a unit that declares no `outputs` block. " + outputsBlockFix

	outputsBlockFix = "Declare its outputs and their mock values there once, so every dependent shares them. " +
		"See https://docs.terragrunt.com/reference/experiments/active#outputs-block"

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
		{
			Name:    StackTargetMissingTypeStack,
			Message: StackTargetMissingTypeStackMessage,
		},
		{
			Name:    StackNestedStacksNotGenerated,
			Message: StackNestedStacksNotGeneratedMessage,
		},
		{
			Name:    MissingDependencyConfig,
			Message: MissingDependencyConfigMessage,
		},
		{
			Name:    InputsReadNotTracked,
			Message: InputsReadNotTrackedMessage,
		},
		{
			Name:    OutputsBlock,
			Message: OutputsBlockMessage,
		},
	}
}
