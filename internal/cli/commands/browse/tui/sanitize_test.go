package tui_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostileName is a file name carrying a clipboard write and a screen clear, the
// shape a malicious module source would use to reach the terminal through a
// rendered listing.
const hostileName = "evil\x1b]52;c;cHduZWQ=\aname\x1b[2J.tf"

// stylingEscape matches an SGR sequence: every color and attribute lipgloss and
// chroma apply is one.
var stylingEscape = regexp.MustCompile(`^\x1b\[[0-9;:]*m`)

// assertOnlyStylingEscapes fails when content carries an escape sequence that
// isn't ours, which is what an injected name or file body would look like once
// it reached the screen. Callers must keep Markdown links out of their fixtures:
// those render as OSC 8 hyperlinks, the one other escape a pane emits.
func assertOnlyStylingEscapes(t *testing.T, content string) {
	t.Helper()

	for i, r := range content {
		if r != 0x1b {
			continue
		}

		rest := content[i:]
		assert.Truef(t, stylingEscape.MatchString(rest), "non-styling escape rendered: %q", rest[:min(len(rest), 16)])
	}
}

func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fs, "/repo/"+hostileName, []byte("x = 1\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/repo/"+hostileName+"-unit/terragrunt.hcl", nil, 0o644))

	m := newModel(t, fs, tui.NewRoot("/repo"), tui.ColorEnabled)

	// The highlighted entry's own path is what the header bar draws.
	content := m.View().Content
	assertOnlyStylingEscapes(t, content)

	m = press(t, m, 'j')
	assertOnlyStylingEscapes(t, m.View().Content)
}

func TestHostileComponentMetadataRendersInert(t *testing.T) {
	t.Parallel()

	source := "example.com/mod\x1b]0;pwned\a/aws"
	unit := component.NewUnit("/repo/vpc").
		WithConfig(&config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: &source}}).
		WithReading("/repo/vpc/read\x1b[2J.hcl")
	unit.AddDependency(component.NewUnit("/repo/dep\x1b[2J"))
	unit.AddDependent(component.NewUnit("/repo/dependent\x1b[2J"))

	m := newModel(t, vfs.NewMemMapFS(), tui.BuildTree("/repo", component.Components{unit}), tui.ColorEnabled)
	require.Equal(t, "vpc", m.Selected().Name())

	assertOnlyStylingEscapes(t, m.View().Content)
}

func TestHostileStackDefinitionRendersInert(t *testing.T) {
	t.Parallel()

	stack := component.NewStack("/repo/live")
	stack.StoreConfig(&config.StackConfig{
		Units: []*config.Unit{
			{Name: "db\x1b[2J", Source: "./mods\x1b]0;pwned\a/db", Path: "db\x1b[2J"},
		},
	})

	m := newModel(t, vfs.NewMemMapFS(), tui.BuildTree("/repo", component.Components{stack}), tui.ColorEnabled)
	require.Equal(t, tui.KindStack, m.Selected().Kind())

	assertOnlyStylingEscapes(t, m.View().Content)
}

// FuzzHostileNames renders arbitrary file and directory names and asserts none
// of them can put an escape sequence of their own on screen. The names come
// straight off the filesystem, so a module a user fetched decides them.
func FuzzHostileNames(f *testing.F) {
	f.Add("main.tf")
	f.Add(hostileName)
	f.Add("a\r\nb")
	f.Add("\xff\xfe")

	f.Fuzz(func(t *testing.T, name string) {
		if name == "" || name == "." || name == ".." ||
			strings.ContainsAny(name, "/\x00") {
			t.Skip("not a single path segment")
		}

		fs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fs, "/repo/"+name, []byte("x = 1\n"), 0o644))
		require.NoError(t, vfs.WriteFile(fs, "/repo/dir-"+name+"/terragrunt.hcl", nil, 0o644))

		m := newModel(t, fs, tui.NewRoot("/repo"), tui.ColorEnabled)

		assertOnlyStylingEscapes(t, m.View().Content)

		m = press(t, m, 'j')
		assertOnlyStylingEscapes(t, m.View().Content)
	})
}
