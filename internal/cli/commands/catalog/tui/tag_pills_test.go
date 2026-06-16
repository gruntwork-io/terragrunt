package tui_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTagPillStyle_KnownKindUsesKindColor verifies that a tag matching a
// component kind ("module") renders with the same yellow palette as the
// module type pill, while an unknown tag renders with the neutral
// version-pill (gray) palette.
func TestTagPillStyle_KnownKindUsesKindColor(t *testing.T) {
	t.Parallel()

	moduleTag := tui.TagPillStyle("module", false).Render("module")
	unknownTag := tui.TagPillStyle("networking", false).Render("networking")

	hasYellow := strings.Contains(moduleTag, "255;218;24") ||
		strings.Contains(moduleTag, "FFDA18") ||
		strings.Contains(moduleTag, "61;53;32") ||
		strings.Contains(moduleTag, "3D3520")
	assert.True(t, hasYellow, "module pill should use yellow palette, got %q", moduleTag)

	assert.NotContains(t, unknownTag, "FFDA18")
	assert.NotContains(t, unknownTag, "255;218;24")
}

func TestKindForTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag      string
		wantKind tui.ComponentKind
		wantOK   bool
	}{
		{tag: "module", wantKind: tui.ComponentKindModule, wantOK: true},
		{tag: "Module", wantKind: tui.ComponentKindModule, wantOK: true},
		{tag: "MODULE", wantKind: tui.ComponentKindModule, wantOK: true},
		{tag: "template", wantKind: tui.ComponentKindTemplate, wantOK: true},
		{tag: "unit", wantKind: tui.ComponentKindUnit, wantOK: true},
		{tag: "stack", wantKind: tui.ComponentKindStack, wantOK: true},
		{tag: "  unit  ", wantKind: tui.ComponentKindUnit, wantOK: true},
		{tag: "modules"},
		{tag: "networking"},
		{tag: ""},
	}

	for _, tc := range tests {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()

			got, ok := tui.KindForTag(tc.tag)
			assert.Equal(t, tc.wantOK, ok)

			if tc.wantOK {
				assert.Equal(t, tc.wantKind, got)
			}
		})
	}
}

func TestRenderTagPills_FitsAllInWideRow(t *testing.T) {
	t.Parallel()

	out := tui.RenderTagPills([]string{"a", "b", "c"}, 200, false)
	require.NotEmpty(t, out)

	plain := stripANSI(out)
	assert.Contains(t, plain, "a")
	assert.Contains(t, plain, "b")
	assert.Contains(t, plain, "c")
}

func TestRenderTagPills_TruncatesWithCountedOverflow(t *testing.T) {
	t.Parallel()

	// Width 15 = label(6) + "aa"(4) + sep(1) + "+2"(4); forces truncation.
	out := tui.RenderTagPills([]string{"aa", "bb", "cc"}, 15, false)
	require.NotEmpty(t, out)

	plain := stripANSI(out)
	assert.Contains(t, plain, "tags:")
	assert.Contains(t, plain, "aa")
	assert.Contains(t, plain, "+2")
	assert.NotContains(t, plain, "+2…")
	assert.NotContains(t, plain, "bb")
	assert.NotContains(t, plain, "cc")
}

func TestBuildMetaRow_TagsAlignAcrossSourceLengths(t *testing.T) {
	t.Parallel()

	const readme = "<!-- Frontmatter\ntags: [networking, aws]\n-->"

	short := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindModule, "ghr://a", "", readme,
	)).WithVersion("v1.0.0").WithSource("github.com/short")

	long := tui.NewComponentEntry(tui.NewComponentForTest(
		tui.ComponentKindModule, "ghr://b", "", readme,
	)).WithVersion("v1.0.0").WithSource("github.com/much-longer-org-name/long-repo")

	rowShort := stripANSI(tui.BuildMetaRow(short, 120, true, false, false))
	rowLong := stripANSI(tui.BuildMetaRow(long, 120, true, false, false))

	colShort, ok := tagsColumn(rowShort)
	require.True(t, ok, "tags: prefix missing from short row: %q", rowShort)

	colLong, ok := tagsColumn(rowLong)
	require.True(t, ok, "tags: prefix missing from long row: %q", rowLong)

	assert.Equal(t, colShort, colLong,
		"tags column should align across rows of differing source URL length\nshort: %q\nlong:  %q",
		rowShort, rowLong)
}

func TestBuildMetaRow_TagsAlignAcrossKinds(t *testing.T) {
	t.Parallel()

	const readme = "<!-- Frontmatter\ntags: [a, b]\n-->"

	rows := map[string]string{}

	for name, kind := range map[string]tui.ComponentKind{
		"module":   tui.ComponentKindModule,
		"template": tui.ComponentKindTemplate,
		"unit":     tui.ComponentKindUnit,
		"stack":    tui.ComponentKindStack,
	} {
		entry := tui.NewComponentEntry(tui.NewComponentForTest(
			kind, "ghr://"+name, "", readme,
		))
		rows[name] = stripANSI(tui.BuildMetaRow(entry, 120, true, false, false))
	}

	cols := map[string]int{}

	for name, row := range rows {
		col, ok := tagsColumn(row)
		require.True(t, ok, "tags: prefix missing from %s row: %q", name, row)

		cols[name] = col
	}

	for name, col := range cols {
		assert.Equal(t, cols["template"], col,
			"tags column for %s row does not match template row\n%s: %q\ntemplate: %q",
			name, name, rows[name], rows["template"])
	}
}

// tagsColumn returns the rune-counted column of the "tags:" label.
func tagsColumn(row string) (int, bool) {
	prefix, _, ok := strings.Cut(row, "tags:")
	if !ok {
		return 0, false
	}

	return utf8.RuneCountInString(prefix), true
}

func TestRenderTagPills_PrivilegedTagsSortFirst(t *testing.T) {
	t.Parallel()

	out := tui.RenderTagPills([]string{"networking", "module", "aws"}, 200, false)
	plain := stripANSI(out)

	moduleIdx := strings.Index(plain, "module")
	networkingIdx := strings.Index(plain, "networking")
	awsIdx := strings.Index(plain, "aws")

	require.NotEqual(t, -1, moduleIdx)
	require.NotEqual(t, -1, networkingIdx)
	require.NotEqual(t, -1, awsIdx)

	assert.Less(t, moduleIdx, networkingIdx, "privileged tag should sort before neutral")
	assert.Less(t, networkingIdx, awsIdx, "neutral tags keep authoring order")
}

func TestRenderTagPills_LabelPrependedWhenTagsFit(t *testing.T) {
	t.Parallel()

	out := tui.RenderTagPills([]string{"a", "b"}, 200, false)
	plain := stripANSI(out)
	assert.True(t, strings.HasPrefix(plain, "tags: "), "expected tags: prefix, got %q", plain)
}

func TestRenderTagPills_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, tui.RenderTagPills(nil, 100, false))
	assert.Empty(t, tui.RenderTagPills([]string{}, 100, false))
}

func TestRenderDetailTagPills(t *testing.T) {
	t.Parallel()

	out := tui.RenderDetailTagPills([]string{"module", "networking"})
	require.NotEmpty(t, out)

	plain := stripANSI(out)
	assert.True(t, strings.HasPrefix(plain, "tags: "), "expected tags: prefix, got %q", plain)
	assert.Contains(t, plain, "module")
	assert.Contains(t, plain, "networking")
}

func TestTagsMarkdownSection(t *testing.T) {
	t.Parallel()

	got := tui.TagsMarkdownSection([]string{"a", "b"})
	assert.Equal(t, "\n\n## Tags\n\n- a\n- b\n", got)

	assert.Empty(t, tui.TagsMarkdownSection(nil))
}

func TestEnvTagsListLayoutToggle(t *testing.T) {
	t.Setenv(tui.EnvTagsListLayout, "row")
	assert.True(t, (tui.ResolveTagsListLayout() == tui.TagsListLayoutRow))

	t.Setenv(tui.EnvTagsListLayout, "META")
	assert.True(t, (tui.ResolveTagsListLayout() == tui.TagsListLayoutMeta))

	t.Setenv(tui.EnvTagsListLayout, "")
	assert.True(t, (tui.ResolveTagsListLayout() == tui.TagsListLayoutMeta))

	t.Setenv(tui.EnvTagsListLayout, "garbage")
	assert.True(t, (tui.ResolveTagsListLayout() == tui.TagsListLayoutMeta))
}

func TestEnvTagsDetailStyleToggle(t *testing.T) {
	t.Setenv(tui.EnvTagsDetailStyle, "section")
	assert.True(t, (tui.ResolveTagsDetailStyle() == tui.TagsDetailStyleSection))

	t.Setenv(tui.EnvTagsDetailStyle, "")
	assert.True(t, (tui.ResolveTagsDetailStyle() == tui.TagsDetailStylePills))

	t.Setenv(tui.EnvTagsDetailStyle, "garbage")
	assert.True(t, (tui.ResolveTagsDetailStyle() == tui.TagsDetailStylePills))
}
