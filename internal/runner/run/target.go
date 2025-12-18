package run

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// targetPointType represents stages in the OpenTofu/Terraform preparation pipeline.
// These are used internally by the run() function for flow control.
const (
	targetPointParseConfig targetPointType = iota + 1
	targetPointDownloadSource
	targetPointGenerateConfig
	targetPointSetInputsAsEnvVars
	targetPointInitCommand
)

type targetPointType byte

type targetCallbackType func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, config *config.TerragruntConfig) error

type targetErrorCallbackType func(l log.Logger, opts *options.TerragruntOptions, config *config.TerragruntConfig, e error) error

// target is an internal flow control mechanism used by run() to manage
// the terraform preparation pipeline stages.
type target struct {
	callbackFunc      targetCallbackType
	errorCallbackFunc targetErrorCallbackType
	point             targetPointType
}

func (t *target) isPoint(point targetPointType) bool {
	if t == nil {
		return false
	}
	return t.point == point
}

func (t *target) runCallback(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, config *config.TerragruntConfig) error {
	if t == nil || t.callbackFunc == nil {
		return nil
	}
	return t.callbackFunc(ctx, l, opts, config)
}

func (t *target) runErrorCallback(l log.Logger, opts *options.TerragruntOptions, config *config.TerragruntConfig, e error) error {
	if t == nil || t.errorCallbackFunc == nil {
		return e
	}
	return t.errorCallbackFunc(l, opts, config, e)
}
