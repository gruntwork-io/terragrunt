package tui_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestComponent_TitlePrefersDocThenDirBasename(t *testing.T) {
	t.Parallel()

	readme := `<!-- Frontmatter
name: VPC App
-->
# Heading
`
	withDoc := tui.NewComponentForTest(tui.ComponentKindModule, "github.com/acme/repo", "modules/vpc", readme)
	assert.Equal(t, "VPC App", withDoc.Title(), "front-matter name wins")

	noDocTitle := tui.NewComponentForTest(tui.ComponentKindModule, "github.com/acme/repo", "modules/vpc", "")
	assert.Equal(t, "vpc", noDocTitle.Title(), "falls back to the directory basename")
}

func TestComponent_DescriptionFallsBackToDefault(t *testing.T) {
	t.Parallel()

	readme := `<!-- Frontmatter
name: VPC App
description: A VPC for application workloads.
-->
# VPC App
`
	withDesc := tui.NewComponentForTest(tui.ComponentKindModule, "github.com/acme/repo", "vpc", readme)
	assert.Equal(t, "A VPC for application workloads.", withDesc.Description())

	noDesc := tui.NewComponentForTest(tui.ComponentKindModule, "github.com/acme/repo", "vpc", "")
	assert.Equal(t, "(no description found)", noDesc.Description())
}

func TestComponent_TagsAndMarkdownAndContent(t *testing.T) {
	t.Parallel()

	readme := `<!-- Frontmatter
name: VPC App
tags: [networking, aws]
-->
# VPC App
Use ` + "`terragrunt`" + ` to apply.
`
	c := tui.NewComponentForTest(tui.ComponentKindModule, "github.com/acme/repo", "vpc", readme)

	assert.Equal(t, []string{"networking", "aws"}, c.Tags())
	assert.True(t, c.IsMarkDown(), "NewComponentForTest builds a Markdown doc")

	raw := c.Content(false)
	assert.Contains(t, raw, "`terragrunt`", "unstripped content keeps Markdown markup")

	stripped := c.Content(true)
	assert.Contains(t, stripped, "terragrunt", "stripped content keeps the word")
	assert.NotContains(t, stripped, "`terragrunt`", "stripped content drops the backticks")
}

func TestComponentDoc_TitleFromFirstHeadingWithoutFrontmatter(t *testing.T) {
	t.Parallel()

	doc := tui.NewComponentDoc("# Just A Heading\n\nbody\n", ".md")
	assert.Equal(t, "Just A Heading", doc.Title())
}

func TestComponentDoc_DescriptionTruncatesToMaxLength(t *testing.T) {
	t.Parallel()

	long := "First sentence is short. " + strings.Repeat("word ", 60) + "tail."
	readme := "<!-- Frontmatter\ndescription: " + long + "\n-->\n# Title\n"

	doc := tui.NewComponentDoc(readme, ".md")

	full := doc.Description(0)
	capped := doc.Description(40)

	assert.Equal(t, long, full, "maxLength 0 returns the whole description")
	assert.Less(t, len(capped), len(full), "a small maxLength truncates")
	assert.True(t, strings.HasSuffix(capped, "."), "truncation closes on a sentence boundary")
}

func TestFindComponentDoc(t *testing.T) {
	t.Parallel()

	t.Run("reads README.md", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		dir := "/repo/vpc"
		require.NoError(t, vfs.WriteFile(fsys, dir+"/README.md", []byte("# VPC\n"), 0o644))

		doc, err := tui.FindComponentDoc(fsys, dir)
		require.NoError(t, err)
		assert.True(t, doc.IsMarkDown())
		assert.Equal(t, "VPC", doc.Title())
	})

	t.Run("returns empty doc when no README", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		dir := "/repo/empty"
		require.NoError(t, vfs.WriteFile(fsys, dir+"/main.tf", []byte("# not a readme\n"), 0o644))

		doc, err := tui.FindComponentDoc(fsys, dir)
		require.NoError(t, err)
		assert.Empty(t, doc.Title(), "a component without a README yields a zero-value doc")
	})
}
