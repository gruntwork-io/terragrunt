package format

import "github.com/gruntwork-io/terragrunt/pkg/log/format/preset"

type Option func(*Formatter)

func WithPresets(presets ...*preset.Preset) Option {
	return func(formatter *Formatter) {
		formatter.presets = presets
	}
}
