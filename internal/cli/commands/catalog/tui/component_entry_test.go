package tui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
)

// unsanitizedReadme puts escape sequences where the catalog reads a
// component's title and description from, the first heading and the paragraph
// under it. A terminal acts on both of them, clearing a line and setting the
// window title.
const unsanitizedReadme = "# VPC\x1b[2K App\n\nA VPC\x1b]0;title\x07 for application workloads.\n"

func TestComponentEntrySanitizesWhatTheListDraws(t *testing.T) {
	t.Parallel()

	c := tui.NewComponentForTest(component.KindModule, "github.com/acme/repo", "modules/vpc", unsanitizedReadme)
	entry := tui.NewComponentEntry(c)

	assert.NotContains(t, entry.Title(), "\x1b")
	assert.NotContains(t, entry.Description(), "\x1b")

	// The list matches against FilterValue and highlights the offsets it
	// returns in the drawn title, so the two have to agree.
	assert.Equal(t, entry.Title(), entry.FilterValue())

	assert.Contains(t, c.Title(), "\x1b", "the component keeps the text as written for non-terminal output")
}

func TestRenderTagPillsSanitizesTags(t *testing.T) {
	t.Parallel()

	tags := []string{"net\x1b]0;title\x07work", "module"}

	assert.NotContains(t, tui.RenderTagPills(tags, 200, false), "\x1b]")
	assert.NotContains(t, tui.RenderDetailTagPills(tags), "\x1b]")
}

func TestRenderTagPillsKeepsTagText(t *testing.T) {
	t.Parallel()

	assert.Contains(t, tui.RenderDetailTagPills([]string{"networking"}), "networking")
}
