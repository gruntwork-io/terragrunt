package terraform

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	TargetPointParseConfig TargetPointType = iota + 1
	TargetPointDownloadSource
	TargetPointGenerateConfig
	TargetPointInitCommand
)

type TargetPointType byte

type TargetCallbackType func(opts *options.TerragruntOptions, config *config.TerragruntConfig) error

// Since most terragrunt commands like `render-json`, `aws-provider-patch` ...  require preparatory steps.
// Target helps to execute a command up to a certain point and then run a callback function.
type Target struct {
	point        TargetPointType
	callbackFunc TargetCallbackType
}

func NewTarget(point TargetPointType, callbackFunc TargetCallbackType) *Target {
	return &Target{
		point:        point,
		callbackFunc: callbackFunc,
	}
}

func (target *Target) isPoint(point TargetPointType) bool {
	return target.point == point
}

func (target *Target) runCallback(opts *options.TerragruntOptions, config *config.TerragruntConfig) error {
	return target.callbackFunc(opts, config)
}
