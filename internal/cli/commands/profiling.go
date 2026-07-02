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
// It runs after flag parsing, so all flag forms and TG_PROFILE_* env vars are already bound to opts.
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

// startProfiling starts the profiles configured via opts and returns a stop function that finalizes them.
func startProfiling(l log.Logger, opts *options.TerragruntOptions) (func(), error) {
	if opts.ProfileCPU == "" && opts.ProfileMem == "" && opts.ProfileGoroutine == "" && opts.ProfileDir == "" {
		return noopStop, nil
	}

	if !opts.Experiments.Evaluate(experiment.Profiling) {
		return nil, ErrProfilingRequiresExperiment
	}

	cpuPath, memPath, goroutinePath, err := resolveProfilePaths(opts)
	if err != nil {
		return nil, err
	}

	cpuFile, err := startCPUProfile(cpuPath)
	if err != nil {
		return nil, err
	}

	stop := func() {
		stopCPUProfile(l, cpuFile)
		writeMemProfile(l, memPath)
		writeGoroutineProfile(l, goroutinePath)
	}

	return stop, nil
}

// noopStop is returned when no profiling flags are configured.
func noopStop() {
	// No profiles were started, so there is nothing to finalize.
}

// resolveProfilePaths determines the output path for each profile type, filling in defaults under
// opts.ProfileDir for any profile that wasn't given an explicit path.
func resolveProfilePaths(opts *options.TerragruntOptions) (cpuPath, memPath, goroutinePath string, err error) {
	cpuPath = opts.ProfileCPU
	memPath = opts.ProfileMem
	goroutinePath = opts.ProfileGoroutine

	if opts.ProfileDir == "" {
		return cpuPath, memPath, goroutinePath, nil
	}

	absDir, err := filepath.Abs(opts.ProfileDir)
	if err != nil {
		return "", "", "", fmt.Errorf("could not resolve profile directory: %w", err)
	}

	if err := util.EnsureDirectory(absDir); err != nil {
		return "", "", "", fmt.Errorf("could not create profile directory: %w", err)
	}

	// Absolute paths keep downstream processes with different working directories in the same directory.
	opts.ProfileDir = absDir

	if cpuPath == "" {
		cpuPath = filepath.Join(absDir, defaultCPUProfileName)
	}

	if memPath == "" {
		memPath = filepath.Join(absDir, defaultMemProfileName)
	}

	if goroutinePath == "" {
		goroutinePath = filepath.Join(absDir, defaultGoroutineProfileName)
	}

	return cpuPath, memPath, goroutinePath, nil
}

// startCPUProfile creates the CPU profile file and starts CPU profiling, if a path was given.
func startCPUProfile(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}

	f, err := createProfileFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not create CPU profile: %w", err)
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

	if err := cpuFile.Close(); err != nil {
		l.Warnf("Could not close CPU profile: %v", err)
	}
}

// writeMemProfile writes a heap profile snapshot to the given path. A no-op if path is empty.
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
	defer f.Close() //nolint:errcheck

	// Writes to a local file the user explicitly requested via --profile-mem/TG_PROFILE_MEM, gated
	// behind the 'profiling' experiment; this is not a production debug endpoint (go:S4507 false positive).
	if err := pprof.WriteHeapProfile(f); err != nil { //NOSONAR:go:S4507 -- local, user-requested profile dump
		l.Warnf("Could not write memory profile: %v", err)
	}
}

// writeGoroutineProfile writes a goroutine profile to the given path. A no-op if path is empty.
func writeGoroutineProfile(l log.Logger, path string) {
	if path == "" {
		return
	}

	f, err := createProfileFile(path)
	if err != nil {
		l.Warnf("Could not create goroutine profile: %v", err)

		return
	}
	defer f.Close() //nolint:errcheck

	p := pprof.Lookup("goroutine")
	if p == nil {
		return
	}

	// Writes to a local file the user explicitly requested via --profile-goroutine/TG_PROFILE_GOROUTINE,
	// gated behind the 'profiling' experiment; this is not a production debug endpoint (go:S4507 false positive).
	if err := p.WriteTo(f, 0); err != nil { //NOSONAR:go:S4507 -- local, user-requested profile dump
		l.Warnf("Could not write goroutine profile: %v", err)
	}
}

// createProfileFile creates a profile output file readable only by the owner.
func createProfileFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, profileFileMode)
}
