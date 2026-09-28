package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestColorOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		value    options.ColorValue
		disabled bool
	}{
		{
			name:     "named_color_wraps_in_ansi",
			value:    options.RedColor,
			expected: "\x1b[31mhello\x1b[m",
		},
		{
			name:     "light_variant_uses_bright_palette",
			value:    options.LightBlueColor,
			expected: "\x1b[94mhello\x1b[m",
		},
		{
			name:     "numeric_value_uses_256_color_palette",
			value:    66,
			expected: "\x1b[38;5;66mhello\x1b[m",
		},
		{
			name:     "none_is_passthrough",
			value:    options.NoneColor,
			expected: "hello",
		},
		{
			name:     "disabled_colors_strips_the_sequence",
			value:    options.RedColor,
			disabled: true,
			expected: "hello",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := options.Color(tc.value).Format(&options.Data{DisabledColors: tc.disabled}, "hello")
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

// TestColorOptionsRenderIndependently pins that options built separately render
// their own color. They read one shared table, so an entry written at runtime
// would show up here as one option's color leaking into another's output.
func TestColorOptionsRenderIndependently(t *testing.T) {
	t.Parallel()

	red, err := options.Color(options.RedColor).Format(&options.Data{}, "hello")
	require.NoError(t, err)

	green, err := options.Color(options.GreenColor).Format(&options.Data{}, "hello")
	require.NoError(t, err)

	redAgain, err := options.Color(options.RedColor).Format(&options.Data{}, "hello")
	require.NoError(t, err)

	assert.Equal(t, red, redAgain)
	assert.NotEqual(t, red, green)
}

// TestColorOptionsBuildConcurrentlyWithRacing pins that formatting shares no
// mutable state. A `run --all` formats from a goroutine per unit, so a write
// behind Format would surface as a data race rather than a wrong color.
func TestColorOptionsBuildConcurrentlyWithRacing(t *testing.T) {
	t.Parallel()

	const builders = 16

	var group errgroup.Group

	for range builders {
		group.Go(func() error {
			out, err := options.Color(options.RedColor).Format(&options.Data{}, "hello")
			if err != nil {
				return err
			}

			assert.Equal(t, "\x1b[31mhello\x1b[m", out)

			return nil
		})
	}

	require.NoError(t, group.Wait())
}

// BenchmarkColorOption measures building one color option. Both log formats build
// one per placeholder, about twenty in total, before a process writes its first
// line.
func BenchmarkColorOption(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = options.Color(options.NoneColor)
	}
}
