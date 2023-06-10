package cli

import (
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/options"
)

// The following commands are DEPRECATED
const (
	CmdSpinUp      = "spin-up"
	CmdTearDown    = "tear-down"
	CmdPlanAll     = "plan-all"
	CmdApplyAll    = "apply-all"
	CmdDestroyAll  = "destroy-all"
	CmdOutputAll   = "output-all"
	CmdValidateAll = "validate-all"
)

// deprecatedCommands is a map of deprecated commands to a handler that knows how to convert the command to the known
// alternative. The handler should return the new TerragruntOptions (if any modifications are needed) and command
// string.
var deprecatedCommands = map[string]func(origOptions *options.TerragruntOptions) (*options.TerragruntOptions, string, string){
	CmdSpinUp:      spinUpDeprecationHandler,
	CmdTearDown:    tearDownDeprecationHandler,
	CmdApplyAll:    applyAllDeprecationHandler,
	CmdDestroyAll:  destroyAllDeprecationHandler,
	CmdPlanAll:     planAllDeprecationHandler,
	CmdValidateAll: validateAllDeprecationHandler,
	CmdOutputAll:   outputAllDeprecationHandler,
}

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
