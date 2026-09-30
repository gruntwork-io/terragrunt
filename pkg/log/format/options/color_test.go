package options_test

import (
	"fmt"
	"strings"
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

func TestColorOptionFormatSpecialValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		data     *options.Data
		name     string
		value    string
		expected string
		color    options.ColorValue
	}{
		{
			name:     "disable_strips_existing_sequences",
			color:    options.DisableColor,
			data:     &options.Data{},
			value:    "\x1b[31mhello\x1b[m",
			expected: "hello",
		},
		{
			name:  "preset_uses_the_preset_color",
			color: options.PresetColor,
			data: &options.Data{PresetColorFn: func() options.ColorValue {
				return options.GreenColor
			}},
			value:    "hello",
			expected: "\x1b[32mhello\x1b[m",
		},
		{
			name:     "preset_without_function_is_unstyled",
			color:    options.PresetColor,
			data:     &options.Data{},
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "negative_value_is_unstyled",
			color:    options.ColorValue(-1),
			data:     &options.Data{},
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "value_past_the_table_is_unstyled",
			color:    options.LightWhiteColor + 1,
			data:     &options.Data{},
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "multi_line_value_styles_each_line",
			color:    options.RedColor,
			data:     &options.Data{},
			value:    "a\nbb",
			expected: "\x1b[31ma\x1b[m \n\x1b[31mbb\x1b[m",
		},
		{
			name:     "tab_is_expanded_by_lipgloss",
			color:    options.RedColor,
			data:     &options.Data{},
			value:    "a\tb",
			expected: "\x1b[31ma    b\x1b[m",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Color(tc.color)
			assert.Equal(t, options.ColorOptionName, opt.Name())

			out, err := opt.Format(tc.data, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

// TestColorOptionFormatGradient pins that gradient gives each distinct text its
// own color from a rotating list, keeps that color on repeats, and wraps once
// the list runs out.
func TestColorOptionFormatGradient(t *testing.T) {
	t.Parallel()

	const listLen = 12

	opt := options.Color(options.GradientColor)

	first := formatColor(t, opt, "unit-0")
	assert.Equal(t, "\x1b[38;5;66munit-0\x1b[m", first)

	second := formatColor(t, opt, "unit-1")
	assert.Equal(t, "\x1b[38;5;67munit-1\x1b[m", second)

	assert.Equal(t, first, formatColor(t, opt, "unit-0"))

	for i := 2; i < listLen; i++ {
		formatColor(t, opt, fmt.Sprintf("unit-%d", i))
	}

	assert.Equal(t, "\x1b[38;5;66mwrapped\x1b[m", formatColor(t, opt, "wrapped"))
}

func TestColorOptionParseValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  string
		errPrefix string
	}{
		{
			name:     "named_color",
			input:    "green",
			expected: "\x1b[32mhello\x1b[m",
		},
		{
			name:     "palette_index",
			input:    "66",
			expected: "\x1b[38;5;66mhello\x1b[m",
		},
		{
			name:      "index_out_of_range",
			input:     "256",
			errPrefix: "available values: 0..255,",
		},
		{
			name:      "unknown_name",
			input:     "pink",
			errPrefix: "available values: 0..255,",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Color(options.NoneColor)

			err := opt.ParseValue(tc.input)
			if tc.errPrefix != "" {
				require.Error(t, err)
				assert.True(t, strings.HasPrefix(err.Error(), tc.errPrefix), err.Error())
				assert.Contains(t, err.Error(), "light-blue")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, formatColor(t, opt, "hello"))
		})
	}
}

func TestColorStyleColorFunc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		style    options.ColorStyle
		expected string
	}{
		{
			name:     "named_color",
			style:    "red",
			expected: "\x1b[31mx\x1b[m",
		},
		{
			name:     "bold",
			style:    "red+b",
			expected: "\x1b[1;31mx\x1b[m",
		},
		{
			name:     "faint",
			style:    "red+d",
			expected: "\x1b[2;31mx\x1b[m",
		},
		{
			name:     "bright_bumps_base_color",
			style:    "red+h",
			expected: "\x1b[91mx\x1b[m",
		},
		{
			name:     "bright_leaves_gray_alone",
			style:    "gray+h",
			expected: "\x1b[90mx\x1b[m",
		},
		{
			name:     "palette_index",
			style:    "66",
			expected: "\x1b[38;5;66mx\x1b[m",
		},
		{
			name:     "empty_is_identity",
			style:    "",
			expected: "x",
		},
		{
			name:     "index_out_of_range_is_identity",
			style:    "256",
			expected: "x",
		},
		{
			name:     "unknown_name_is_identity",
			style:    "pink+b",
			expected: "x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.style.ColorFunc()("x"))
		})
	}
}

func formatColor(t *testing.T, opt options.Option, value string) string {
	t.Helper()

	out, err := opt.Format(&options.Data{}, value)
	require.NoError(t, err)

	str, ok := out.(string)
	require.True(t, ok)

	return str
}
