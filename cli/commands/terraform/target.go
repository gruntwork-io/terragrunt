package terraform

import (
	"context"

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

type TargetCallbackType func(ctx context.Context, opts *options.TerragruntOptions, config *config.TerragruntConfig) error

type TargetErrorCallbackType func(opts *options.TerragruntOptions, config *config.TerragruntConfig, e error) error

// Since most terragrunt CLI commands like `render-json`, `aws-provider-patch` ...  require preparatory steps, such as `generate configuration`
// which is already coded in `terraform.runTerraform` and complicated to extract
// into a separate function due to some steps that can be called recursively in case of nested configuration or dependencies.
// Target struct helps to run `terraform.runTerraform` func up to the certain logic point, and the runs target's callback func and returns the flow.
// For example, `terragrunt-info` CLI command requires source to be downloaded before running its specific action. To do this it:
/*
   package terragruntinfo
   ... code omitted

   // creates a new target with `TargetPointDownloadSource` point name, once a source is downloaded `target` will call the `runTerragruntInfo` callback func.
   target := terraform.NewTarget(terraform.TargetPointDownloadSource, runTerragruntInfo)
   terraform.RunWithTarget(opts, target)

   ... code omitted

   func runTerragruntInfo(opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {

   ... code omitted
*/

/*
   package terraform
   ... code omitted

   func runTerraform(terragruntOptions *options.TerragruntOptions, target *Target) error {
   ... code omitted

       // At this point, the terraform source is downloaded to the terragrunt working directory
       if target.isPoint(TargetPointDownloadSource) {
	       return target.runCallback(updatedTerragruntOptions, terragruntConfig)
       }

   ... code omitted
   }
*/

type Target struct {
	point             TargetPointType
	callbackFunc      TargetCallbackType
	errorCallbackFunc TargetErrorCallbackType
}

func NewTarget(point TargetPointType, callbackFunc TargetCallbackType) *Target {
	return &Target{
		point:        point,
		callbackFunc: callbackFunc,
	}
}

func NewTargetWithErrorHandler(point TargetPointType, callbackFunc TargetCallbackType, errorCallbackFunc TargetErrorCallbackType) *Target {
	return &Target{
		point:             point,
		callbackFunc:      callbackFunc,
		errorCallbackFunc: errorCallbackFunc,
	}
}

func (target *Target) isPoint(point TargetPointType) bool {
	return target.point == point
}

func (target *Target) runCallback(ctx context.Context, opts *options.TerragruntOptions, config *config.TerragruntConfig) error {
	return target.callbackFunc(ctx, opts, config)
}

func (target *Target) runErrorCallback(opts *options.TerragruntOptions, config *config.TerragruntConfig, e error) error {
	if target.errorCallbackFunc == nil {
		return e
	}

	return target.errorCallbackFunc(opts, config, e)
}
