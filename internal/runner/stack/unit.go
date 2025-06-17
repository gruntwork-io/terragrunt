package stack

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Unit represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type Unit struct {
	Stack                Stack
	TerragruntOptions    *options.TerragruntOptions
	Logger               log.Logger
	Path                 string
	Dependencies         Units
	Config               config.TerragruntConfig
	AssumeAlreadyApplied bool
	FlagExcluded         bool
}

type Units []*Unit

type UnitsMap map[string]*Unit
