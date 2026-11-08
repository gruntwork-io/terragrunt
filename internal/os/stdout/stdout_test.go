package stdout_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
)

func TestShouldColor(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		stdoutIsTTY    bool
		colorsDisabled bool
		want           bool
	}{
		{
			name:        "terminal with colors enabled",
			stdoutIsTTY: true,
			want:        true,
		},
		{
			name:           "terminal with colors disabled",
			stdoutIsTTY:    true,
			colorsDisabled: true,
		},
		{
			name: "pipe with colors enabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
			formatter.SetDisabledColors(tc.colorsDisabled)

			l := log.New(log.WithFormatter(formatter))

			v := venvtest.New()
			v.Terminal = &venv.Terminal{StdoutIsTTY: func() bool { return tc.stdoutIsTTY }}

			assert.Equal(t, tc.want, stdout.ShouldColor(l, v))
		})
	}
}
