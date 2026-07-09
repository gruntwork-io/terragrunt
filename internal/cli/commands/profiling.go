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
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	defaultCPUProfileName       = "terragrunt_cpu.prof"
	defaultMemProfileName       = "terragrunt_mem.prof"
	defaultGoroutineProfileName = "terragrunt_goroutine.prof"

	profileFileMode = 0o600
	profileDirMode  = 0o700
)

// ErrProfilingRequiresExperiment is returned when profiling flags are used without the 'profiling' experiment.
var ErrProfilingRequiresExperiment = errors.New(
	"profiling flags require usage of the 'profiling' experiment (e.g., --experiment=profiling)",
)

// WrapWithProfiling wraps command actions with profile collection driven by the profiling flags.
func WrapWithProfiling(
	l log.Logger,
	opts *options.TerragruntOptions,
	v venv.Venv,
) func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
	var profilingInProgress bool

	return func(ctx context.Context, cliCtx *clihelper.Context, action clihelper.ActionFunc) error {
		if profilingInProgress {
			return action(ctx, cliCtx)
		}

		stopProfiling, err := startProfiling(l, v.FS, opts)
		if err != nil {
			return err
		}

		profilingInProgress = true

		defer func() {
			stopProfiling()

			profilingInProgress = false
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
func startProfiling(l log.Logger, fs vfs.FS, opts *options.TerragruntOptions) (func(), error) {
	if opts.ProfileCPU == "" && opts.ProfileMem == "" && opts.ProfileGoroutine == "" && opts.ProfileDir == "" {
		return noopStop, nil
	}

	if !opts.Experiments.Evaluate(experiment.Profiling) {
		return nil, ErrProfilingRequiresExperiment
	}

	paths, err := resolveProfilePaths(fs, opts)
	if err != nil {
		return nil, err
	}

	var memFile, goroutineFile, cpuFile vfs.File

	if paths.mem != "" {
		if memFile, err = createNamedProfileFile(l, fs, paths.mem, "memory"); err != nil {
			return nil, err
		}
	}

	if paths.goroutine != "" {
		if goroutineFile, err = createNamedProfileFile(l, fs, paths.goroutine, "goroutine"); err != nil {
			if memFile != nil {
				discardProfileFile(l, fs, paths.mem, memFile)
			}

			return nil, err
		}
	}

	if paths.cpu != "" {
		if cpuFile, err = startCPUProfile(l, fs, paths.cpu); err != nil {
			if memFile != nil {
				discardProfileFile(l, fs, paths.mem, memFile)
			}

			if goroutineFile != nil {
				discardProfileFile(l, fs, paths.goroutine, goroutineFile)
			}

			return nil, err
		}
	}

	stop := func() {
		if cpuFile != nil {
			stopCPUProfile(l, cpuFile)
		}

		if memFile != nil {
			writeMemProfile(l, memFile)
		}

		if goroutineFile != nil {
			writeGoroutineProfile(l, goroutineFile)
		}
	}

	return stop, nil
}

// noopStop is returned when no profiling flags are configured.
func noopStop() {
	// No profiles were started, so there is nothing to finalize.
}

// resolveProfilePaths resolves the output path for each profile type, filling in defaults under opts.ProfileDir.
func resolveProfilePaths(fs vfs.FS, opts *options.TerragruntOptions) (profilePaths, error) {
	paths := profilePaths{
		cpu:       opts.ProfileCPU,
		mem:       opts.ProfileMem,
		goroutine: opts.ProfileGoroutine,
	}

	if opts.ProfileDir == "" {
		return paths, nil
	}

	dir := opts.ProfileDir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(opts.WorkingDir, dir)
	}

	if err := fs.MkdirAll(dir, profileDirMode); err != nil {
		return profilePaths{}, fmt.Errorf("could not create profile directory: %w", err)
	}

	// Absolute paths keep downstream processes with different working directories in the same directory.
	opts.ProfileDir = dir

	if paths.cpu == "" {
		paths.cpu = filepath.Join(dir, defaultCPUProfileName)
	}

	if paths.mem == "" {
		paths.mem = filepath.Join(dir, defaultMemProfileName)
	}

	if paths.goroutine == "" {
		paths.goroutine = filepath.Join(dir, defaultGoroutineProfileName)
	}

	return paths, nil
}

// startCPUProfile creates the CPU profile file at the given non-empty path and starts CPU profiling.
func startCPUProfile(l log.Logger, fs vfs.FS, path string) (vfs.File, error) {
	f, err := createNamedProfileFile(l, fs, path, "CPU")
	if err != nil {
		return nil, err
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		discardProfileFile(l, fs, path, f)

		return nil, fmt.Errorf("could not start CPU profile: %w", err)
	}

	return f, nil
}

// stopCPUProfile finalizes CPU profiling and closes the profile file.
func stopCPUProfile(l log.Logger, cpuFile vfs.File) {
	pprof.StopCPUProfile()
	closeProfileFile(l, "CPU", cpuFile)
}

// writeMemProfile writes a heap profile snapshot to the given file and closes it.
func writeMemProfile(l log.Logger, f vfs.File) {
	runtime.GC()

	// Writes to a local file the user explicitly requested via --profile-mem/TG_PROFILE_MEM, gated
	// behind the 'profiling' experiment; this is not a production debug endpoint (go:S4507 false positive).
	if err := pprof.WriteHeapProfile(f); err != nil { //NOSONAR:go:S4507 -- local, user-requested profile dump
		l.Warnf("Could not write memory profile: %v", err)
	}

	closeProfileFile(l, "memory", f)
}

// writeGoroutineProfile writes a goroutine profile to the given file and closes it.
func writeGoroutineProfile(l log.Logger, f vfs.File) {
	// Writes to a local file the user explicitly requested via --profile-goroutine/TG_PROFILE_GOROUTINE,
	// gated behind the 'profiling' experiment; this is not a production debug endpoint (go:S4507 false positive).
	if err := pprof.Lookup("goroutine").WriteTo(f, 0); err != nil { //NOSONAR:go:S4507 -- local, user-requested profile dump
		l.Warnf("Could not write goroutine profile: %v", err)
	}

	closeProfileFile(l, "goroutine", f)
}

// createNamedProfileFile creates the output file for the named profile at the given non-empty path.
func createNamedProfileFile(l log.Logger, fs vfs.FS, path, name string) (vfs.File, error) {
	f, err := createProfileFile(l, fs, path)
	if err != nil {
		return nil, fmt.Errorf("could not create %s profile: %w", name, err)
	}

	return f, nil
}

// createProfileFile creates a profile output file readable only by the owner, tightening permissions on pre-existing files.
func createProfileFile(l log.Logger, fs vfs.FS, path string) (vfs.File, error) {
	f, err := fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, profileFileMode)
	if err != nil {
		return nil, err
	}

	if err := fs.Chmod(path, profileFileMode); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			l.Debugf("Could not close profile %s: %v", path, closeErr)
		}

		return nil, err
	}

	return f, nil
}

// closeProfileFile closes a finished profile file, logging a warning if the final flush fails.
func closeProfileFile(l log.Logger, name string, f vfs.File) {
	if err := f.Close(); err != nil {
		l.Warnf("Could not close %s profile: %v", name, err)
	}
}

// discardProfileFile closes and removes a partially created profile file after a startup failure.
func discardProfileFile(l log.Logger, fs vfs.FS, path string, f vfs.File) {
	if err := f.Close(); err != nil {
		l.Debugf("Could not close discarded profile %s: %v", path, err)
	}

	if err := fs.Remove(path); err != nil {
		l.Debugf("Could not remove discarded profile %s: %v", path, err)
	}
}
