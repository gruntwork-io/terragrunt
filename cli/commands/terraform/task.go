package terraform

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	TaskTargetParseConfig TaskTargetType = iota + 1
	TaskTargetDownloadSource
	TaskTargetGenerateConfig
	TaskTargetInitCommand
)

type TaskTargetType byte

type TaskCallbackType func(opts *options.TerragruntOptions, config *config.TerragruntConfig) error

type Task struct {
	target       TaskTargetType
	callbackFunc TaskCallbackType
}

func NewTask(target TaskTargetType, callbackFunc TaskCallbackType) *Task {
	return &Task{
		target:       target,
		callbackFunc: callbackFunc,
	}
}

func (task *Task) isTarget(target TaskTargetType) bool {
	return task.target == target
}

func (task *Task) runCallback(opts *options.TerragruntOptions, config *config.TerragruntConfig) error {
	return task.callbackFunc(opts, config)
}
