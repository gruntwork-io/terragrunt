package md_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/md"
)

func TestTerminalRendererRendersContent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		background md.Background
	}{
		{name: "dark", background: md.DarkBackground},
		{name: "light", background: md.LightBackground},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out := ansi.Strip(render(t, 80, tc.background, "# VPC\n\nCreates a VPC.\n"))

			assert.Contains(t, out, "VPC")
			assert.Contains(t, out, "Creates a VPC.")
		})
	}
}

// TestTerminalRendererStylesForTheBackground pins the background to something
// the reader sees: the same document drawn for a dark terminal and for a light
// one is drawn differently.
func TestTerminalRendererStylesForTheBackground(t *testing.T) {
	t.Parallel()

	const source = "# VPC\n\nCreates a VPC.\n"

	assert.NotEqual(
		t,
		render(t, 80, md.DarkBackground, source),
		render(t, 80, md.LightBackground, source),
	)
}

func TestTerminalRendererWordWraps(t *testing.T) {
	t.Parallel()

	source := strings.Repeat("word ", 40)

	assert.Greater(
		t,
		strings.Count(render(t, 40, md.DarkBackground, source), "\n"),
		strings.Count(render(t, md.NoWrapWidth, md.DarkBackground, source), "\n"),
		"a width nothing reaches leaves the line as it was written",
	)
}

func render(t *testing.T, width int, background md.Background, source string) string {
	t.Helper()

	r, err := md.NewTerminalRenderer(width, background)
	require.NoError(t, err)

	out, err := r.Render(source)
	require.NoError(t, err)

	return out
}
