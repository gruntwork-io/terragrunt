package config

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// getDependencyBlockConfigPathsByFilepath returns the config paths of the dependency blocks of the config at cfgPath,
// which pctx parses as a dependency, and records the files the parse read on pctx.FilesRead. The paths are memoized in
// s under [StageDependencyPaths] when the parse is memoized.
func getDependencyBlockConfigPathsByFilepath(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	cfgPath string,
) ([]string, error) {
	scanPctx := pctx.WithDecodeList(DependencyBlock).WithSkipOutputsResolution().WithDiagnosticsSuppressed()
	pc := scanPctx.forStore(v)

	if !pc.memoized {
		return readDependencyBlockConfigPaths(ctx, l, v, s, pc, cfgPath)
	}

	key := Key{Path: filepath.Clean(cfgPath), Context: pc.key, Stage: StageDependencyPaths}
	flight := pc.withOwnReads()

	res, err := s.Memoize(ctx, key, func(ctx context.Context) ([]string, error) {
		paths, err := readDependencyBlockConfigPaths(ctx, l, v, s, flight, cfgPath)
		NoteReads(ctx, flight.FilesRead.Paths()...)

		return paths, err
	})

	for _, path := range res.Reads {
		pc.FilesRead.Add(path)
	}

	return res.Value, err
}

// readDependencyBlockConfigPaths partially parses the config at cfgPath with pc and returns the config paths of its
// dependency blocks.
func readDependencyBlockConfigPaths(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pc *ParsingContext,
	cfgPath string,
) ([]string, error) {
	tgConfig, err := s.ResolvePartialConfig(ctx, l, v, pc, cfgPath)
	if err != nil {
		return nil, err
	}

	paths, unresolved := tgConfig.DependencyPaths()
	for _, name := range unresolved {
		l.Debugf("Skipping dependency %q of %s in cycle check: config_path is not a known string", name, cfgPath)
	}

	return paths, nil
}
