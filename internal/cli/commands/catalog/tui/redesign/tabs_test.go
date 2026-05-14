package redesign_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/stretchr/testify/assert"
)

func TestTabKindMatches_TagPromotesAcrossTabs(t *testing.T) {
	t.Parallel()

	template := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindTemplate,
		"github.com/example/repo",
		"templates/svc",
		`<!-- Frontmatter
name: svc
tags: [module, networking]
-->
# svc
`,
	))

	assert.True(t, redesign.MatchesTab(redesign.TabAll, template))
	assert.True(t, redesign.MatchesTab(redesign.TabTemplates, template))
	assert.True(t, redesign.MatchesTab(redesign.TabModules, template), "template tagged module should match TabModules")
	assert.False(t, redesign.MatchesTab(redesign.TabUnits, template))
	assert.False(t, redesign.MatchesTab(redesign.TabStacks, template))
}

func TestTabKindMatches_CaseInsensitive(t *testing.T) {
	t.Parallel()

	stack := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindStack,
		"github.com/example/repo",
		"stacks/svc",
		`<!-- Frontmatter
tags: [Module, UNIT]
-->
# svc
`,
	))

	assert.True(t, redesign.MatchesTab(redesign.TabModules, stack))
	assert.True(t, redesign.MatchesTab(redesign.TabUnits, stack))
}

func TestTabKindMatches_NoTagsOnlyKind(t *testing.T) {
	t.Parallel()

	module := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindModule,
		"github.com/example/repo",
		"modules/vpc",
		"# VPC\nNo frontmatter here.",
	))

	assert.True(t, redesign.MatchesTab(redesign.TabModules, module))
	assert.False(t, redesign.MatchesTab(redesign.TabTemplates, module))
	assert.False(t, redesign.MatchesTab(redesign.TabUnits, module))
	assert.False(t, redesign.MatchesTab(redesign.TabStacks, module))
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

		bar := redesign.RenderTabBar(redesign.TabModules, false)
		assertColored(t, bar, "3D3520", "61;53;32")
	})

	t.Run("templates tab → mauve", func(t *testing.T) {
		t.Parallel()

		bar := redesign.RenderTabBar(redesign.TabTemplates, false)
		assertColored(t, bar, "2A2040", "42;32;64")
	})

	t.Run("units tab → blue", func(t *testing.T) {
		t.Parallel()

		bar := redesign.RenderTabBar(redesign.TabUnits, false)
		assertColored(t, bar, "1E2840", "30;40;64")
	})

	t.Run("stacks tab → green", func(t *testing.T) {
		t.Parallel()

		bar := redesign.RenderTabBar(redesign.TabStacks, false)
		assertColored(t, bar, "1F2D20", "31;45;32")
	})
}

func TestRenderTabBar_AllTabKeepsNeutralStyle(t *testing.T) {
	t.Parallel()

	bar := redesign.RenderTabBar(redesign.TabAll, false)

	for _, c := range []string{"3D3520", "2A2040", "1E2840", "1F2D20"} {
		assert.NotContains(t, bar, c, "All tab should not adopt kind color %s; got %q", c, bar)
	}
}

func TestTabKindMatches_UnrelatedTagDoesNotPromote(t *testing.T) {
	t.Parallel()

	module := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindModule,
		"github.com/example/repo",
		"modules/vpc",
		`<!-- Frontmatter
tags: [networking, aws]
-->
# VPC
`,
	))

	assert.False(t, redesign.MatchesTab(redesign.TabTemplates, module))
	assert.False(t, redesign.MatchesTab(redesign.TabUnits, module))
	assert.False(t, redesign.MatchesTab(redesign.TabStacks, module))
}
