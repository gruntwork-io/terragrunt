package cli

import (
	goErrors "errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/posener/complete/cmd/install"
)

// defaultAutocompleteInstallFlag and defaultAutocompleteUninstallFlag are the
// default values for the autocomplete install and uninstall flags.
const (
	defaultAutocompleteInstallFlag   = "install-autocomplete"
	defaultAutocompleteUninstallFlag = "uninstall-autocomplete"

	envCompleteLine = "COMP_LINE"

	maxDashesInFlag = 2
)

var DefaultComplete = defaultComplete

// AutocompleteInstaller is an interface to be implemented to perform the
// autocomplete installation and uninstallation with a CLI.
//
// This interface is not exported because it only exists for unit tests
// to be able to test that the installation is called properly.
type AutocompleteInstaller interface {
	Install(string) error
	Uninstall(string) error
}

// autocompleteInstaller uses the install package to do the
// install/uninstall.
type autocompleteInstaller struct{}

func (i *autocompleteInstaller) Install(cmd string) error {
	if err := install.Install(cmd); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func (i *autocompleteInstaller) Uninstall(cmd string) error {
	if err := install.Uninstall(cmd); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

// ShowCompletions prints the lists of commands within a given context
func ShowCompletions(ctx *Context) error {
	if cmd := ctx.Command; cmd != nil && cmd.Complete != nil {
		return cmd.Complete(ctx)
	}

	return DefaultComplete(ctx)
}

func defaultComplete(ctx *Context) error {
	arg := ctx.Args().Last()

	if strings.HasPrefix(arg, "-") {
		if cmd := ctx.Command; cmd != nil {
			return printFlagSuggestions(arg, cmd.Flags, ctx.App.Writer)
		}

		return printFlagSuggestions(arg, ctx.App.Flags, ctx.App.Writer)
	}

	return printCommandSuggestions(arg, ctx.Command.Subcommands, ctx.App.Writer)
}

func printCommandSuggestions(arg string, commands []*Command, writer io.Writer) error {
	errs := []error{}

	for _, command := range commands {
		if command.Hidden {
			continue
		}

		for _, name := range command.Names() {
			if name != "" && (arg == "" || strings.HasPrefix(name, arg)) {
				_, err := fmt.Fprintln(writer, name)
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return goErrors.Join(errs...)
	}

	return nil
}

func printFlagSuggestions(arg string, flags []Flag, writer io.Writer) error {
	cur := strings.TrimPrefix(arg, "-")

	errs := []error{}
	for _, flag := range flags {
		for _, name := range flag.Names() {
			name = strings.TrimSpace(name)
			// this will get total count utf8 letters in flag name
			count := utf8.RuneCountInString(name)
			if count > maxDashesInFlag {
				count = maxDashesInFlag // reuse this count to generate single - or -- in flag completion
			}
			// if flag name has more than one utf8 letter and last argument in cli has -- prefix then
			// skip flag completion for short flags example -v or -x
			if strings.HasPrefix(arg, "--") && count == 1 {
				continue
			}
			// match if last argument matches this flag and it is not repeated
			if strings.HasPrefix(name, cur) && cur != name && !cliArgContains(name) {
				flagCompletion := fmt.Sprintf("%s%s", strings.Repeat("-", count), name)
				_, err := fmt.Fprintln(writer, flagCompletion)
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return goErrors.Join(errs...)
	}

	return nil
}

func cliArgContains(flagName string) bool {
	for _, name := range strings.Split(flagName, ",") {
		name = strings.TrimSpace(name)
		count := utf8.RuneCountInString(name)
		if count > maxDashesInFlag {
			count = maxDashesInFlag
		}
		flag := fmt.Sprintf("%s%s", strings.Repeat("-", count), name)
		for _, a := range os.Args {
			if a == flag {
				return true
			}
		}
	}
	return false
}
