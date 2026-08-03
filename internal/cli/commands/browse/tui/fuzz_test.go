package tui_test

import (
	"errors"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/stretchr/testify/require"
)

// errFuzzDiscovery stands in for a discovery failure when fuzzing that path.
var errFuzzDiscovery = errors.New("fuzz discovery failure")

// fuzzComponents is the discovery result fed while fuzzing; it matches the fixed
// tree so the component-attach, reading-highlight, and count paths are exercised.
var fuzzComponents = component.Components{
	component.NewUnit("/repo/vpc").WithReading("/repo/vpc/main.tf"),
	component.NewStack("/repo/live"),
}

// fuzzFS builds a fixed in-memory estate covering the classifier's branches: a
// unit, a stack, plain and Markdown files, a hidden directory, and a cache
// directory whose contents must never be taken for units.
func fuzzFS(t *testing.T) vfs.FS {
	t.Helper()

	fs := vfs.NewMemMapFS()

	write := func(path, body string) {
		require.NoError(t, vfs.WriteFile(fs, path, []byte(body), 0o644))
	}

	write("/repo/vpc/terragrunt.hcl", "inputs = {}\n")
	write("/repo/vpc/main.tf", "resource \"null_resource\" \"x\" {}\n")
	write("/repo/vpc/README.md", "# vpc\n\nbody\n")
	write("/repo/live/terragrunt.stack.hcl", "unit \"x\" {\n  source = \"./x\"\n}\n")
	write("/repo/data.json", "{}\n")
	write("/repo/mod/.terragrunt-cache/y/terragrunt.hcl", "")

	require.NoError(t, fs.MkdirAll("/repo/.git", 0o755))

	return fs
}

// fuzzMsgs is the alphabet of messages a fuzz byte selects from: navigation and
// search keys, a range of terminal sizes (including degenerate ones), and the
// background messages the browse command delivers. Commands returned by Update
// are dropped rather than run, so the model is driven purely by synthesized
// messages and incurs no real side effect.
func fuzzMsgs() []tea.Msg {
	return []tea.Msg{
		tea.KeyPressMsg{Code: 'j', Text: "j"},
		tea.KeyPressMsg{Code: 'k', Text: "k"},
		tea.KeyPressMsg{Code: 'h', Text: "h"},
		tea.KeyPressMsg{Code: 'l', Text: "l"},
		tea.KeyPressMsg{Code: 'g', Text: "g"},
		tea.KeyPressMsg{Code: 'G', Text: "G"},
		tea.KeyPressMsg{Code: 'n', Text: "n"},
		tea.KeyPressMsg{Code: 'N', Text: "N"},
		tea.KeyPressMsg{Code: '/', Text: "/"},
		tea.KeyPressMsg{Code: 'a', Text: "a"},
		tea.KeyPressMsg{Code: '.', Text: "."},
		tea.KeyPressMsg{Code: tea.KeyEnter},
		tea.KeyPressMsg{Code: tea.KeyEscape},
		tea.KeyPressMsg{Code: tea.KeyBackspace},
		tea.KeyPressMsg{Code: tea.KeyPgUp},
		tea.KeyPressMsg{Code: tea.KeyPgDown},
		tea.WindowSizeMsg{Width: 1, Height: 1},
		tea.WindowSizeMsg{Width: 8, Height: 5},
		tea.WindowSizeMsg{Width: 40, Height: 12},
		tea.WindowSizeMsg{Width: 120, Height: 40},
		tui.DiscoveryResult{Components: fuzzComponents},
		tui.DiscoveryResult{Err: errFuzzDiscovery},
		viewtui.Warning{Message: "warn"},
		viewtui.ToastExpired{ID: 1},
	}
}

// maxFuzzSteps bounds how many messages one input drives, keeping each execution
// fast so the fuzzer explores more inputs.
const maxFuzzSteps = 1024

// FuzzModel drives the browser through arbitrary sequences of input events and
// background messages, asserting it never panics and that the selection stays
// within the current directory. Every side effect is controlled: the filesystem
// is in memory and the commands Update returns are dropped rather than run.
func FuzzModel(f *testing.F) {
	f.Add([]byte{3, 3, 3, 11})
	f.Add([]byte{20, 3, 0, 0, 8, 9, 11})
	f.Add([]byte{16, 17, 18, 19, 3, 0})
	f.Add([]byte("browse the tree"))

	msgs := fuzzMsgs()

	f.Fuzz(func(t *testing.T, ops []byte) {
		m := newModel(t, fuzzFS(t), tui.NewRoot("/repo"), tui.ColorDisabled)

		for i, b := range ops {
			if i >= maxFuzzSteps {
				break
			}

			m = update(t, m, msgs[int(b)%len(msgs)])

			_ = m.View()

			if sel := m.Selected(); sel != nil {
				require.Truef(t, slices.Contains(m.Current().Children(), sel),
					"selected %q is not among the current directory's children", sel.Name())
			}
		}
	})
}

// FuzzFilePreview writes arbitrary bytes to a Markdown and a source file and
// renders each, ensuring the binary sniff, chroma highlighting, and glamour
// Markdown rendering stay panic-free on hostile input.
func FuzzFilePreview(f *testing.F) {
	f.Add([]byte("inputs = {}\n"))
	f.Add([]byte("# Title\n\n```\nunclosed fence"))
	f.Add([]byte{0x00, 0x01, 0xff, 0xfe})
	f.Add([]byte("\x1b[31mnot really ansi"))

	f.Fuzz(func(t *testing.T, content []byte) {
		fs := vfs.NewMemMapFS()
		require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/doc.md", content, 0o644))
		require.NoError(t, vfs.WriteFile(fs, "/repo/vpc/src.tf", content, 0o644))

		root := tui.BuildTree("/repo", component.Components{component.NewUnit("/repo/vpc")})

		// Color on so both glamour (Markdown) and chroma (source) actually render.
		m := newModel(t, fs, root, tui.ColorEnabled)

		m = press(t, m, 'l')
		_ = m.View()

		m = press(t, m, 'j')
		_ = m.View()
	})
}
