package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	defaultCPUProfileName       = "terragrunt_cpu.prof"
	defaultMemProfileName       = "terragrunt_mem.prof"
	defaultGoroutineProfileName = "terragrunt_goroutine.prof"

	profileFileMode = 0o600
)

// ErrProfilingRequiresExperiment is returned when profiling flags are used without the 'profiling' experiment.
var ErrProfilingRequiresExperiment = errors.New(
	"profiling flags require the 'profiling' experiment to be enabled (e.g., --experiment=profiling)",
)

// WrapWithProfiling wraps command actions with profile collection driven by the profiling flags.
func WrapWithProfiling(
	l log.Logger,
	opts *options.TerragruntOptions,
) func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
	var profiling bool

	return func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
		if profiling {
			return action(ctx, cliCtx)
		}

		stopProfiling, err := startProfiling(l, opts)
		if err != nil {
			return err
		}

		profiling = true

		defer func() {
			stopProfiling()

			profiling = false
		}()

		return action(ctx, cliCtx)
	}
}

// profilePaths holds the resolved output path for each profile type.
type profilePaths struct {
	cpu       string
	mem       string
	goroutine string
}

// startProfiling starts the profiles configured via opts and returns a stop function that finalizes them.
func startProfiling(l log.Logger, opts *options.TerragruntOptions) (func(), error) {
	if opts.ProfileCPU == "" && opts.ProfileMem == "" && opts.ProfileGoroutine == "" && opts.ProfileDir == "" {
		return noopStop, nil
	}

	if !opts.Experiments.Evaluate(experiment.Profiling) {
		return nil, ErrProfilingRequiresExperiment
	}

	paths, err := resolveProfilePaths(opts)
	if err != nil {
		return nil, err
	}

	if err := validateProfilePath(paths.mem, "memory"); err != nil {
		return nil, err
	}

	if err := validateProfilePath(paths.goroutine, "goroutine"); err != nil {
		return nil, err
	}

	cpuFile, err := startCPUProfile(paths.cpu)
	if err != nil {
		return nil, err
	}

	stop := func() {
		stopCPUProfile(l, cpuFile)
		writeMemProfile(l, paths.mem)
		writeGoroutineProfile(l, paths.goroutine)
	}

	return stop, nil
}

// validateProfilePath verifies the named profile path is writable without holding the file open for the run.
func validateProfilePath(path, name string) error {
	f, err := createNamedProfileFile(path, name)
	if err != nil || f == nil {
		return err
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("could not create %s profile: %w", name, err)
	}

	return nil
}

// noopStop is returned when no profiling flags are configured.
func noopStop() {
	// No profiles were started, so there is nothing to finalize.
}

// resolveProfilePaths resolves the output path for each profile type, filling in defaults under opts.ProfileDir.
func resolveProfilePaths(opts *options.TerragruntOptions) (profilePaths, error) {
	paths := profilePaths{
		cpu:       opts.ProfileCPU,
		mem:       opts.ProfileMem,
		goroutine: opts.ProfileGoroutine,
	}

	if opts.ProfileDir == "" {
		return paths, nil
	}

	absDir, err := filepath.Abs(opts.ProfileDir)
	if err != nil {
		return profilePaths{}, fmt.Errorf("could not resolve profile directory: %w", err)
	}

	if err := util.EnsureDirectory(absDir); err != nil {
		return profilePaths{}, fmt.Errorf("could not create profile directory: %w", err)
	}

	// Absolute paths keep downstream processes with different working directories in the same directory.
	opts.ProfileDir = absDir

	if paths.cpu == "" {
		paths.cpu = filepath.Join(absDir, defaultCPUProfileName)
	}

	if paths.mem == "" {
		paths.mem = filepath.Join(absDir, defaultMemProfileName)
	}

	if paths.goroutine == "" {
		paths.goroutine = filepath.Join(absDir, defaultGoroutineProfileName)
	}

	return paths, nil
}

// startCPUProfile creates the CPU profile file and starts CPU profiling, no-op when path is empty.
func startCPUProfile(path string) (*os.File, error) {
	f, err := createNamedProfileFile(path, "CPU")
	if err != nil || f == nil {
		return f, err
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close() //nolint:errcheck

		return nil, fmt.Errorf("could not start CPU profile: %w", err)
	}

	return f, nil
}

// stopCPUProfile finalizes CPU profiling and closes the profile file, if one was started.
func stopCPUProfile(l log.Logger, cpuFile *os.File) {
	if cpuFile == nil {
		return
	}

	pprof.StopCPUProfile()
	closeProfileFile(l, "CPU", cpuFile)
}

// writeMemProfile writes a heap profile snapshot to the given path, no-op when empty.
func writeMemProfile(l log.Logger, path string) {
	if path == "" {
		return
	}

	runtime.GC()

	f, err := createProfileFile(path)
	if err != nil {
		l.Warnf("Could not create memory profile: %v", err)

		return
	}

	// Writes to a local file the user explicitly requested via --profile-mem/TG_PROFILE_MEM, gated
	// behind the 'profiling' experiment; this is not a production debug endpoint (go:S4507 false positive).
	if err := pprof.WriteHeapProfile(f); err != nil { //NOSONAR:go:S4507 -- local, user-requested profile dump
		l.Warnf("Could not write memory profile: %v", err)
	}

	closeProfileFile(l, "memory", f)
}

// writeGoroutineProfile writes a goroutine profile to the given path, no-op when empty.
func writeGoroutineProfile(l log.Logger, path string) {
	if path == "" {
		return
	}

	f, err := createProfileFile(path)
	if err != nil {
		l.Warnf("Could not create goroutine profile: %v", err)

		return
	}

	// Writes to a local file the user explicitly requested via --profile-goroutine/TG_PROFILE_GOROUTINE,
	// gated behind the 'profiling' experiment; this is not a production debug endpoint (go:S4507 false positive).
	if err := pprof.Lookup("goroutine").WriteTo(f, 0); err != nil { //NOSONAR:go:S4507 -- local, user-requested profile dump
		l.Warnf("Could not write goroutine profile: %v", err)
	}

	closeProfileFile(l, "goroutine", f)
}

// createNamedProfileFile creates the output file for the named profile, nil when path is empty.
func createNamedProfileFile(path, name string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}

	f, err := createProfileFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not create %s profile: %w", name, err)
	}

	return f, nil
}

// createProfileFile creates a profile output file readable only by the owner.
func createProfileFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, profileFileMode)
}

// closeProfileFile closes a finished profile file, logging a warning if the final flush fails.
func closeProfileFile(l log.Logger, name string, f *os.File) {
	if f == nil {
		return
	}

	if err := f.Close(); err != nil {
		l.Warnf("Could not close %s profile: %v", name, err)
	}
}
