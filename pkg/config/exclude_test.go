package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestExcludeConfig_ShouldPreventRun_Output(t *testing.T) {
	t.Parallel()

	boolTrue := true
	boolFalse := false

	tests := []struct {
		name    string
		action  string
		exclude config.ExcludeConfig
		want    bool
	}{
		{
			name: "output in actions with if=true prevents output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan", "apply", "destroy", "output"},
			},
			action: "output",
			want:   true,
		},
		{
			name: "output in actions with if=false does not prevent output",
			exclude: config.ExcludeConfig{
				If:      false,
				Actions: []string{"output"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "output not in actions does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan", "apply"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "all actions prevents output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all"},
				NoRun:   &boolTrue,
			},
			action: "output",
			want:   true,
		},
		{
			name: "all_except_output does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all_except_output"},
				NoRun:   &boolTrue,
			},
			action: "output",
			want:   false,
		},
		{
			name: "no_run=false never prevents",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"output"},
				NoRun:   &boolFalse,
			},
			action: "output",
			want:   false,
		},
		{
			name: "empty actions does not prevent",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{},
			},
			action: "output",
			want:   false,
		},
		{
			name: "plan action does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan"},
			},
			action: "output",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.exclude.ShouldPreventRun(tt.action)
			assert.Equal(t, tt.want, got)
		})
	}
}
