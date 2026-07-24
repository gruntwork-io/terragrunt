package tui_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/stretchr/testify/assert"
)

func TestTabKindMatches_TagPromotesAcrossTabs(t *testing.T) {
	t.Parallel()

	template := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindTemplate,
		"github.com/example/repo",
		"templates/svc",
		`<!-- Frontmatter
name: svc
tags: [module, networking]
-->
# svc
`,
	))

	assert.True(t, tui.TabAll.Matches(template))
	assert.True(t, tui.TabTemplates.Matches(template))
	assert.True(
		t,
		tui.TabModules.Matches(template),
		"template tagged module should match TabModules",
	)
	assert.False(t, tui.TabUnits.Matches(template))
	assert.False(t, tui.TabStacks.Matches(template))
}

func TestTabKindMatches_CaseInsensitive(t *testing.T) {
	t.Parallel()

	stack := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindStack,
		"github.com/example/repo",
		"stacks/svc",
		`<!-- Frontmatter
tags: [Module, UNIT]
-->
# svc
`,
	))

	assert.True(t, tui.TabModules.Matches(stack))
	assert.True(t, tui.TabUnits.Matches(stack))
}

func TestTabKindMatches_NoTagsOnlyKind(t *testing.T) {
	t.Parallel()

	module := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindModule,
		"github.com/example/repo",
		"modules/vpc",
		"# VPC\nNo frontmatter here.",
	))

	assert.True(t, tui.TabModules.Matches(module))
	assert.False(t, tui.TabTemplates.Matches(module))
	assert.False(t, tui.TabUnits.Matches(module))
	assert.False(t, tui.TabStacks.Matches(module))
}

func TestRenderTabBar_ActiveKindTabUsesKindColor(t *testing.T) {
	t.Parallel()

	// Lipgloss may emit either the hex or the decimal R;G;B form in the SGR.
	assertColored := func(t *testing.T, bar, bgHex, bgRGB string) {
		t.Helper()

		hasColor := strings.Contains(bar, bgHex) || strings.Contains(bar, bgRGB)
		assert.True(t, hasColor,
			"active tab should render with kind background color (hex %s or rgb %s); got %q",
			bgHex, bgRGB, bar)
	}

	t.Run("modules tab → yellow", func(t *testing.T) {
		t.Parallel()

		bar := tui.RenderTabBar(tui.TabModules, false)
		assertColored(t, bar, "3D3520", "61;53;32")
	})

	t.Run("templates tab → mauve", func(t *testing.T) {
		t.Parallel()

		bar := tui.RenderTabBar(tui.TabTemplates, false)
		assertColored(t, bar, "2A2040", "42;32;64")
	})

	t.Run("units tab → blue", func(t *testing.T) {
		t.Parallel()

		bar := tui.RenderTabBar(tui.TabUnits, false)
		assertColored(t, bar, "1E2840", "30;40;64")
	})

	t.Run("stacks tab → green", func(t *testing.T) {
		t.Parallel()

		bar := tui.RenderTabBar(tui.TabStacks, false)
		assertColored(t, bar, "1F2D20", "31;45;32")
	})
}

func TestRenderTabBar_AllTabKeepsNeutralStyle(t *testing.T) {
	t.Parallel()

	bar := tui.RenderTabBar(tui.TabAll, false)

	for _, c := range []string{"3D3520", "2A2040", "1E2840", "1F2D20"} {
		assert.NotContains(t, bar, c, "All tab should not adopt kind color %s; got %q", c, bar)
	}
}

func TestTabKindMatches_UnrelatedTagDoesNotPromote(t *testing.T) {
	t.Parallel()

	module := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindModule,
		"github.com/example/repo",
		"modules/vpc",
		`<!-- Frontmatter
tags: [networking, aws]
-->
# VPC
`,
	))

	assert.False(t, tui.TabTemplates.Matches(module))
	assert.False(t, tui.TabUnits.Matches(module))
	assert.False(t, tui.TabStacks.Matches(module))
}
