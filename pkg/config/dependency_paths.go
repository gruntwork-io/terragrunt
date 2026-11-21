package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// getDependencyBlockConfigPathsByFilepath returns the config paths of the dependency blocks of the config at cfgPath,
// which pctx parses as a dependency, and records the files the parse read on pctx.FilesRead.
func getDependencyBlockConfigPathsByFilepath(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	cfgPath string,
) ([]string, error) {
	scanPctx := pctx.WithDecodeList(DependencyBlock).WithSkipOutputsResolution().WithDiagnosticsSuppressed()

	tgConfig, err := s.ResolvePartialConfig(ctx, l, v, NewParseContext(v, scanPctx), cfgPath)
	if err != nil {
		return nil, err
	}

	paths, unresolved := tgConfig.DependencyPaths()
	for _, name := range unresolved {
		l.Debugf("Skipping dependency %q of %s in cycle check: config_path is not a known string", name, cfgPath)
	}

	return paths, nil
}
