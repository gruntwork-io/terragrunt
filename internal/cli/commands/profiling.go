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
		return func() {}, nil
	}

	if !opts.Experiments.Evaluate(experiment.Profiling) {
		return nil, ErrProfilingRequiresExperiment
	}

	cpuPath := opts.ProfileCPU
	memPath := opts.ProfileMem
	goroutinePath := opts.ProfileGoroutine

	if opts.ProfileDir != "" {
		absDir, err := filepath.Abs(opts.ProfileDir)
		if err != nil {
			return nil, fmt.Errorf("could not resolve profile directory: %w", err)
		}

		if err := util.EnsureDirectory(absDir); err != nil {
			return nil, fmt.Errorf("could not create profile directory: %w", err)
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
	}

	var cpuFile *os.File

	if cpuPath != "" {
		f, err := createProfileFile(cpuPath)
		if err != nil {
			return nil, fmt.Errorf("could not create CPU profile: %w", err)
		}

		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close() //nolint:errcheck

			return nil, fmt.Errorf("could not start CPU profile: %w", err)
		}

		cpuFile = f
	}

	stop := func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()

			if err := cpuFile.Close(); err != nil {
				l.Warnf("Could not close CPU profile: %v", err)
			}
		}

		if memPath != "" {
			writeMemProfile(l, memPath)
		}

		if goroutinePath != "" {
			writeGoroutineProfile(l, goroutinePath)
		}
	}

	return stop, nil
}

// writeMemProfile writes a heap profile snapshot to the given path.
func writeMemProfile(l log.Logger, path string) {
	runtime.GC()

	f, err := createProfileFile(path)
	if err != nil {
		l.Warnf("Could not create memory profile: %v", err)

		return
	}
	defer f.Close() //nolint:errcheck

	if err := pprof.WriteHeapProfile(f); err != nil {
		l.Warnf("Could not write memory profile: %v", err)
	}
}

// writeGoroutineProfile writes a goroutine profile to the given path.
func writeGoroutineProfile(l log.Logger, path string) {
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

	if err := p.WriteTo(f, 0); err != nil {
		l.Warnf("Could not write goroutine profile: %v", err)
	}
}

// createProfileFile creates a profile output file readable only by the owner.
func createProfileFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, profileFileMode)
}
