package cli_test

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cliCommand is a command in [cliInventory].
type cliCommand struct {
	name          string
	category      string
	aliases       []string
	flags         []cliFlag
	subcommands   []cliCommand
	hidden        bool
	showsTofuHelp bool
}

// cliFlag is a flag in [cliInventory]. A flag without names is set through
// its environment variables only.
type cliFlag struct {
	names             []string
	envVars           []string
	deprecatedNames   []string
	deprecatedEnvVars []string
	hidden            bool
}

// TestCLIInventory pins every command and flag the CLI registers, with the
// names and environment variables each one answers to, against [cliInventory].
func TestCLIInventory(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	app := cli.NewApp(logger.CreateLogger(), options.NewTerragruntOptions(v.Exec), v)

	got := flattenCommands(&cliCommand{
		name:        app.Name,
		flags:       inventoryFlags(app.Flags),
		subcommands: inventoryCommands(app.Commands),
	})
	want := flattenCommands(&cliInventory)

	assert.ElementsMatch(t, mapKeys(want), mapKeys(got))

	for path, wantCmd := range want {
		gotCmd, ok := got[path]
		if !ok {
			continue
		}

		t.Run(path, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, wantCmd.flat(), gotCmd.flat())
		})
	}
}

// TestCLIHelpMatchesInventory pins that the help of every command lists the
// visible subcommands and flags of [cliInventory], with each flag's names and
// environment variables.
func TestCLIHelpMatchesInventory(t *testing.T) {
	t.Parallel()

	globalHelp := helpFlags(cliInventory.flags)

	for path, cmd := range flattenCommands(&cliInventory) {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			args := slices.Concat(strings.Fields(path)[1:], []string{"--help"})

			out, err := runCLI(t, venvtest.New(), args...)
			require.NoError(t, err)

			assert.Equal(t, wantHelp(path, cmd, globalHelp), parseHelp(out))
		})
	}
}

// wantHelp returns the sections, in the form [parseHelp] returns them, that
// the help of cmd at path lists.
func wantHelp(path string, cmd *cliCommand, globalHelp []string) map[string][]string {
	if cmd.showsTofuHelp {
		return map[string][]string{}
	}

	sections := map[string][]string{"Global Options": globalHelp}

	if path == cliInventory.name {
		for i := range cmd.subcommands {
			sub := &cmd.subcommands[i]
			if !sub.hidden {
				sections[sub.category] = append(sections[sub.category], helpCommand(sub))
			}
		}
	}

	if path != cliInventory.name {
		addSection(sections, "Commands", helpCommands(cmd.subcommands))
		addSection(sections, "Options", helpFlags(cmd.flags))
	}

	for name, entries := range sections {
		sections[name] = slices.Sorted(slices.Values(entries))
	}

	return sections
}

// flatCommand is a [cliCommand] without its subcommands.
type flatCommand struct {
	category string
	aliases  []string
	flags    []cliFlag
	hidden   bool
}

func (cmd *cliCommand) flat() flatCommand {
	return flatCommand{
		category: cmd.category,
		aliases:  cmd.aliases,
		flags:    cmd.flags,
		hidden:   cmd.hidden,
	}
}

// flattenCommands maps the path of root and of each of its descendants, such
// as "terragrunt hcl format", to a copy of the command with its flags sorted.
func flattenCommands(root *cliCommand) map[string]*cliCommand {
	commands := map[string]*cliCommand{}

	var walk func(prefix string, cmd *cliCommand)

	walk = func(prefix string, cmd *cliCommand) {
		path := strings.TrimSpace(prefix + " " + cmd.name)

		sorted := *cmd
		sorted.flags = slices.Clone(cmd.flags)
		slices.SortFunc(sorted.flags, func(a, b cliFlag) int {
			return cmp.Or(
				slices.Compare(a.names, b.names),
				slices.Compare(a.envVars, b.envVars),
			)
		})
		commands[path] = &sorted

		for i := range cmd.subcommands {
			walk(path, &cmd.subcommands[i])
		}
	}

	walk("", root)

	return commands
}

func mapKeys[V any](m map[string]V) []string {
	return slices.Sorted(func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	})
}

func inventoryCommands(cmds clihelper.Commands) []cliCommand {
	if len(cmds) == 0 {
		return nil
	}

	inventory := make([]cliCommand, 0, len(cmds))

	for _, cmd := range cmds {
		var category string
		if cmd.Category != nil {
			category = cmd.Category.Name
		}

		inventory = append(inventory, cliCommand{
			name:        cmd.Name,
			category:    category,
			aliases:     nonEmpty(cmd.Aliases),
			flags:       inventoryFlags(cmd.Flags),
			subcommands: inventoryCommands(cmd.Subcommands),
			hidden:      cmd.Hidden,
		})
	}

	slices.SortFunc(inventory, func(a, b cliCommand) int {
		return strings.Compare(a.name, b.name)
	})

	return inventory
}

func inventoryFlags(flags clihelper.Flags) []cliFlag {
	if len(flags) == 0 {
		return nil
	}

	inventory := make([]cliFlag, 0, len(flags))

	for _, flag := range flags {
		f := cliFlag{
			names:   nonEmpty(flag.Names()),
			envVars: nonEmpty(flag.GetEnvVars()),
			hidden:  flag.GetHidden(),
		}

		if deprecated, ok := flag.(interface{ DeprecatedNames() []string }); ok {
			f.deprecatedNames = nonEmpty(deprecated.DeprecatedNames())
		}

		if deprecated, ok := flag.(interface{ DeprecatedEnvVars() []string }); ok {
			f.deprecatedEnvVars = nonEmpty(deprecated.DeprecatedEnvVars())
		}

		inventory = append(inventory, f)
	}

	return inventory
}

func nonEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}

	return s
}

func addSection(sections map[string][]string, name string, entries []string) {
	if len(entries) > 0 {
		sections[name] = entries
	}
}

func helpCommands(cmds []cliCommand) []string {
	var entries []string

	for i := range cmds {
		if !cmds[i].hidden {
			entries = append(entries, helpCommand(&cmds[i]))
		}
	}

	return entries
}

// helpCommand renders the names help prints for cmd, such as "find, fd".
func helpCommand(cmd *cliCommand) string {
	return strings.Join(append([]string{cmd.name}, cmd.aliases...), ", ")
}

func helpFlags(flags []cliFlag) []string {
	var entries []string

	for _, flag := range flags {
		if flag.hidden || len(flag.names) == 0 {
			continue
		}

		entries = append(entries, helpFlag(flag.names, flag.envVars))
	}

	return entries
}

// helpFlag renders a flag as [parseHelp] reads it back, such as
// "--all, -a [TG_ALL]".
func helpFlag(names, envVars []string) string {
	dashed := make([]string, 0, len(names))

	for _, name := range names {
		if len(name) == 1 {
			dashed = append(dashed, "-"+name)
			continue
		}

		dashed = append(dashed, "--"+name)
	}

	entry := strings.Join(dashed, ", ")
	if len(envVars) == 0 {
		return entry
	}

	return entry + " [" + strings.Join(envVars, ", ") + "]"
}

var (
	helpSectionRe = regexp.MustCompile(`^([A-Z][\w ]*):$`)
	helpEntryRe   = regexp.MustCompile(`^   (\S.*?)(?:  +(.*))?$`)
	helpEnvVarsRe = regexp.MustCompile(`\[([$%]\w+%?(?:, [$%]\w+%?)*)\]$`)
)

// parseHelp maps each section of help output that lists commands or flags to
// its entries, sorted. A command entry is its names, such as "find, fd". A
// flag entry is its names and environment variables, in the form [helpFlag]
// renders.
func parseHelp(out string) map[string][]string {
	sections := map[string][]string{}

	var section string

	for line := range strings.SplitSeq(out, "\n") {
		if m := helpSectionRe.FindStringSubmatch(line); m != nil {
			section = m[1]
			continue
		}

		m := helpEntryRe.FindStringSubmatch(line)
		if m == nil || section == "" || section == "Examples" {
			continue
		}

		names := strings.Split(m[1], ", ")
		if !strings.HasPrefix(names[0], "-") {
			sections[section] = append(sections[section], m[1])
			continue
		}

		for i, name := range names {
			names[i] = strings.TrimLeft(strings.TrimSuffix(name, " value"), "-")
		}

		var envVars []string
		if env := helpEnvVarsRe.FindStringSubmatch(m[2]); env != nil {
			envVars = strings.Split(strings.NewReplacer("$", "", "%", "").Replace(env[1]), ", ")
		}

		sections[section] = append(sections[section], helpFlag(names, envVars))
	}

	for _, entries := range sections {
		slices.Sort(entries)
	}

	return sections
}
