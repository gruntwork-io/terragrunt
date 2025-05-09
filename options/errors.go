package options

import "github.com/gruntwork-io/terragrunt/internal/errors"

// ErrRunTerragruntCommandNotSet is a custom error type indicating that the command is not set.
var ErrRunTerragruntCommandNotSet = errors.New("the RunTerragrunt option has not been set on this TerragruntOptions object")
