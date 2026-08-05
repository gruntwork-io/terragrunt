package cli

import (
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/rcfile"
)

// maxEarlyFlagsParse bounds the early flag scan, so that an unexpected argument list
// cannot keep it spinning.
const maxEarlyFlagsParse = 1000

// setupRCFile discovers the Terragrunt rc file, exports the environment variables it
// declares and installs the flag defaults it declares on the app.
//
// It runs before the application parses its command line, because the environment
// variables the file declares have to be in place before anything reads the environment,
// including the provider cache server, which starts before the Terragrunt configuration
// is read. Discovery starts at the working directory, and the file is only honored while
// the experiment is enabled, so both are resolved from the raw arguments first.
func (app *App) setupRCFile(args []string) error {
	// These are throwaway copies of the flags rather than the ones on the app: parsing a
	// flag marks it as set, and the app's own flags have to look untouched when the
	// command line is parsed for real, or setting them again reports them as set twice.
	earlyFlags := global.NewFlags(app.l, app.opts, nil).Filter(
		global.WorkingDirFlagName,
		global.ExperimentFlagName,
		global.ExperimentModeFlagName,
	)

	if err := parseEarlyFlags(earlyFlags, args); err != nil {
		return err
	}

	startDir, err := rcFileSearchStart(app.opts.WorkingDir)
	if err != nil {
		return err
	}

	path, err := rcfile.Find(startDir)
	if err != nil || path == "" {
		return err
	}

	// The gate is checked before the file is read, so that a mistake in an rc file only
	// fails the runs that asked for the feature. Discovery itself still runs, so that a
	// file nobody enabled is reported rather than silently doing nothing.
	if !app.opts.Experiments.Evaluate(experiment.TerragruntRC) {
		app.l.Warnf(
			"Ignoring %s because the %s experiment is not enabled. Enable it with --experiment %s.",
			path,
			experiment.TerragruntRC,
			experiment.TerragruntRC,
		)

		return nil
	}

	cfg, err := rcfile.Load(path)
	if err != nil {
		return err
	}

	if err := app.applyRCFileEnv(cfg); err != nil {
		return err
	}

	app.FlagDefaults = func(cmdPath [][]string, flag clihelper.Flag) ([]string, bool) {
		return cfg.FlagValues(cmdPath, flag.Names())
	}

	app.l.Debugf("Read flag defaults from %s", cfg.Path)

	return nil
}

// applyRCFileEnv exports the environment variables declared by the rc file. A variable
// that is already set is left alone, so the shell always wins over the file.
//
// Variables are exported to the process, which is what everything reading os.Getenv sees,
// and to the environment Terragrunt hands to the commands it runs.
func (app *App) applyRCFileEnv(cfg *rcfile.Config) error {
	for name, value := range cfg.EnvVars() {
		if _, ok := os.LookupEnv(name); ok {
			continue
		}

		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("failed to set %s declared in %s: %w", name, cfg.Path, err)
		}

		if app.env != nil {
			app.env[name] = value
		}

		app.l.Debugf("Set %s from %s", name, cfg.Path)
	}

	return nil
}

// rcFileSearchStart returns the directory the search for an rc file starts from: the
// working directory given on the command line, or the current directory.
func rcFileSearchStart(workingDir string) (string, error) {
	if workingDir != "" {
		return workingDir, nil
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine the current directory: %w", err)
	}

	return currentDir, nil
}

// parseEarlyFlags parses the given flags from anywhere in args, before the application
// parses its command line for real.
//
// The standard flag package stops at the first argument that is not a known flag, so
// parsing resumes right after it. Errors are deliberately ignored: every argument that
// belongs to another flag or to a command shows up as one here, and the real parse
// reports genuine mistakes with the context needed to explain them.
func parseEarlyFlags(flags clihelper.Flags, args []string) error {
	flagSet, err := flags.NewFlagSet(AppName, func(error) error { return nil })
	if err != nil {
		return err
	}

	// The first argument is the path of the binary itself.
	if len(args) > 0 {
		args = args[1:]
	}

	for range maxEarlyFlagsParse {
		if len(args) == 0 {
			return nil
		}

		parseErr := flagSet.Parse(args)
		args = flagSet.Args()

		// A nil error means parsing stopped at an argument that is not a flag, such as a
		// command name, and that argument still has to be stepped over. An error means the
		// parser already consumed the argument it choked on.
		if parseErr == nil && len(args) > 0 {
			args = args[1:]
		}
	}

	return nil
}
