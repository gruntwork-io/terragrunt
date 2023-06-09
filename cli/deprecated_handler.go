package cli

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
)

func spinUpDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	return origOptions, CmdApplyAll, CmdApplyAll
}

func tearDownDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	return origOptions, CmdDestroyAll, CmdDestroyAll
}

func applyAllDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	opts := origOptions.Clone(origOptions.TerragruntConfigPath)
	opts.TerraformCommand = "apply"
	opts.OriginalTerraformCommand = "apply"
	opts.TerraformCliArgs = append([]string{"apply"}, opts.TerraformCliArgs...)
	newCmdFriendly := fmt.Sprintf("terragrunt run-all %s", strings.Join(opts.TerraformCliArgs, " "))
	return opts, CmdRunAll, newCmdFriendly
}

func destroyAllDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	opts := origOptions.Clone(origOptions.TerragruntConfigPath)
	opts.TerraformCommand = "destroy"
	opts.OriginalTerraformCommand = "destroy"
	opts.TerraformCliArgs = append([]string{"destroy"}, opts.TerraformCliArgs...)
	newCmdFriendly := fmt.Sprintf("terragrunt run-all %s", strings.Join(opts.TerraformCliArgs, " "))
	return opts, CmdRunAll, newCmdFriendly
}

func planAllDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	opts := origOptions.Clone(origOptions.TerragruntConfigPath)
	opts.TerraformCommand = "plan"
	opts.OriginalTerraformCommand = "plan"
	opts.TerraformCliArgs = append([]string{"plan"}, opts.TerraformCliArgs...)
	newCmdFriendly := fmt.Sprintf("terragrunt run-all %s", strings.Join(opts.TerraformCliArgs, " "))
	return opts, CmdRunAll, newCmdFriendly
}

func validateAllDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	opts := origOptions.Clone(origOptions.TerragruntConfigPath)
	opts.TerraformCommand = "validate"
	opts.OriginalTerraformCommand = "validate"
	opts.TerraformCliArgs = append([]string{"validate"}, opts.TerraformCliArgs...)
	newCmdFriendly := fmt.Sprintf("terragrunt run-all %s", strings.Join(opts.TerraformCliArgs, " "))
	return opts, CmdRunAll, newCmdFriendly
}

func outputAllDeprecationHandler(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string) {
	opts := origOptions.Clone(origOptions.TerragruntConfigPath)
	opts.TerraformCommand = "output"
	opts.OriginalTerraformCommand = "output"
	opts.TerraformCliArgs = append([]string{"output"}, opts.TerraformCliArgs...)
	newCmdFriendly := fmt.Sprintf("terragrunt run-all %s", strings.Join(opts.TerraformCliArgs, " "))
	return opts, CmdRunAll, newCmdFriendly
}
