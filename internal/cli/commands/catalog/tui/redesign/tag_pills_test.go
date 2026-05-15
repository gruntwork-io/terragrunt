package redesign_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTagPillStyle_KnownKindUsesKindColor verifies that a tag matching a
// component kind ("module") renders with the same yellow palette as the
// module type pill, while an unknown tag renders with the neutral
// version-pill (gray) palette.
func TestTagPillStyle_KnownKindUsesKindColor(t *testing.T) {
	t.Parallel()

	moduleTag := redesign.TagPillRenderForTest("module", false)
	unknownTag := redesign.TagPillRenderForTest("networking", false)

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
		wantKind redesign.ComponentKind
		wantOK   bool
	}{
		{"module", redesign.ComponentKindModule, true},
		{"Module", redesign.ComponentKindModule, true},
		{"MODULE", redesign.ComponentKindModule, true},
		{"template", redesign.ComponentKindTemplate, true},
		{"unit", redesign.ComponentKindUnit, true},
		{"stack", redesign.ComponentKindStack, true},
		{"  unit  ", redesign.ComponentKindUnit, true},
		{"modules", 0, false},
		{"networking", 0, false},
		{"", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()

			got, ok := redesign.KindForTagForTest(tc.tag)
			assert.Equal(t, tc.wantOK, ok)

			if tc.wantOK {
				assert.Equal(t, tc.wantKind, got)
			}
		})
	}
}

func TestRenderTagPills_FitsAllInWideRow(t *testing.T) {
	t.Parallel()

	out := redesign.RenderTagPillsForTest([]string{"a", "b", "c"}, 200, false)
	require.NotEmpty(t, out)

	plain := stripANSI(out)
	assert.Contains(t, plain, "a")
	assert.Contains(t, plain, "b")
	assert.Contains(t, plain, "c")
}

func TestRenderTagPills_TruncatesWithCountedOverflow(t *testing.T) {
	t.Parallel()

	// Width 15 = label(6) + "aa"(4) + sep(1) + "+2"(4); forces truncation.
	out := redesign.RenderTagPillsForTest([]string{"aa", "bb", "cc"}, 15, false)
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

	short := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindModule, "ghr://a", "", readme,
	)).WithVersion("v1.0.0").WithSource("github.com/short")

	long := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindModule, "ghr://b", "", readme,
	)).WithVersion("v1.0.0").WithSource("github.com/much-longer-org-name/long-repo")

	rowShort := stripANSI(redesign.BuildMetaRow(short, 120, true, false, false))
	rowLong := stripANSI(redesign.BuildMetaRow(long, 120, true, false, false))

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

	for name, kind := range map[string]redesign.ComponentKind{
		"module":   redesign.ComponentKindModule,
		"template": redesign.ComponentKindTemplate,
		"unit":     redesign.ComponentKindUnit,
		"stack":    redesign.ComponentKindStack,
	} {
		entry := redesign.NewComponentEntry(redesign.NewComponentForTest(
			kind, "ghr://"+name, "", readme,
		))
		rows[name] = stripANSI(redesign.BuildMetaRow(entry, 120, true, false, false))
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

	out := redesign.RenderTagPillsForTest([]string{"networking", "module", "aws"}, 200, false)
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

	out := redesign.RenderTagPillsForTest([]string{"a", "b"}, 200, false)
	plain := stripANSI(out)
	assert.True(t, strings.HasPrefix(plain, "tags: "), "expected tags: prefix, got %q", plain)
}

func TestRenderTagPills_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()

	assert.Empty(t, redesign.RenderTagPillsForTest(nil, 100, false))
	assert.Empty(t, redesign.RenderTagPillsForTest([]string{}, 100, false))
}

func TestRenderDetailTagPills(t *testing.T) {
	t.Parallel()

	out := redesign.RenderDetailTagPillsForTest([]string{"module", "networking"})
	require.NotEmpty(t, out)

	plain := stripANSI(out)
	assert.True(t, strings.HasPrefix(plain, "tags: "), "expected tags: prefix, got %q", plain)
	assert.Contains(t, plain, "module")
	assert.Contains(t, plain, "networking")
}

func TestTagsMarkdownSection(t *testing.T) {
	t.Parallel()

	got := redesign.TagsMarkdownSectionForTest([]string{"a", "b"})
	assert.Equal(t, "\n\n## Tags\n\n- a\n- b\n", got)

	assert.Empty(t, redesign.TagsMarkdownSectionForTest(nil))
}

func TestEnvTagsListLayoutToggle(t *testing.T) {
	t.Parallel()

	assert.True(t, redesign.ResolveTagsListLayoutRowForTest(map[string]string{redesign.EnvTagsListLayoutForTest: "row"}))
	assert.True(t, redesign.ResolveTagsListLayoutMetaForTest(map[string]string{redesign.EnvTagsListLayoutForTest: "META"}))
	assert.True(t, redesign.ResolveTagsListLayoutMetaForTest(map[string]string{redesign.EnvTagsListLayoutForTest: ""}))
	assert.True(t, redesign.ResolveTagsListLayoutMetaForTest(map[string]string{redesign.EnvTagsListLayoutForTest: "garbage"}))
}

func TestEnvTagsDetailStyleToggle(t *testing.T) {
	t.Parallel()

	assert.True(t, redesign.ResolveTagsDetailStyleSectionForTest(map[string]string{redesign.EnvTagsDetailStyleForTest: "section"}))
	assert.True(t, redesign.ResolveTagsDetailStylePillsForTest(map[string]string{redesign.EnvTagsDetailStyleForTest: ""}))
	assert.True(t, redesign.ResolveTagsDetailStylePillsForTest(map[string]string{redesign.EnvTagsDetailStyleForTest: "garbage"}))
}
