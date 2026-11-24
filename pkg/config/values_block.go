package config

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ConfigRole is whether a parsed file is the config of a component or a file another config includes.
type ConfigRole int

const (
	// ComponentConfig is the terragrunt.hcl of a unit, the terragrunt.stack.hcl of a stack, or a file that
	// read_terragrunt_config reads.
	ComponentConfig ConfigRole = iota
	// IncludedConfig is a file that an include block names, or a unit or stack autoinclude file.
	IncludedConfig
)

// valuesFileContent is a parsed and evaluated terragrunt.values.hcl.
type valuesFileContent struct {
	file  *hclparse.File
	attrs hcl.Attributes
	value cty.Value
}

// RoleOf returns the role of file parsed for include, the include block that names it. It is [IncludedConfig] when
// include is non-nil, and the role the name of file gives it otherwise.
func RoleOf(file *hclparse.File, include *IncludeConfig) ConfigRole {
	if include != nil {
		return IncludedConfig
	}

	return fileRole(file)
}

// ReadValues returns the values file evaluates with: the attributes of the terragrunt.values.hcl beside file, resolved
// against the values block of file.
//
// When file has no values block, the values are resolved against the values block of the unit or stack config beside
// file, so a file that a unit includes from its own directory reads the values the unit reads. When that config has no
// values block either, the values are the attributes as the values file sets them, or nil when there is no values file.
//
// Returns [ValuesBlockInIncludeError] when file declares a values block and role is [IncludedConfig], and
// [ValuesBlockRequiresExperimentError] when file declares one and its experiment is off.
//
// Reports a diagnostic, through the handlers of the file it points at, for each key the values file sets that the
// block does not declare, each value that does not convert to its type, and each required value the file does not set.
// When the handlers drop them, each value that fails is unknown.
func ReadValues(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	role ConfigRole,
) (*cty.Value, error) {
	decls, diags := valuesblock.Decode(file.Body)
	if decls == nil {
		if err := file.HandleDiagnostics(diags); err != nil {
			return nil, err
		}

		return directoryValues(ctx, l, v, s, pctx, file)
	}

	if role == IncludedConfig {
		return nil, ValuesBlockInIncludeError{ConfigPath: file.ConfigPath}
	}

	if !pctx.Experiments.Evaluate(experiment.ValuesBlock) {
		return nil, ValuesBlockRequiresExperimentError{ConfigPath: file.ConfigPath}
	}

	if err := file.HandleDiagnostics(diags); err != nil {
		return nil, err
	}

	return resolveValues(ctx, l, v, s, pctx, file, decls)
}

// directoryValues returns the values of the directory of file, which has no values block.
func directoryValues(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
) (*cty.Value, error) {
	if pctx.Experiments.Evaluate(experiment.ValuesBlock) {
		component, decls, err := declaringComponent(ctx, l, v, s, pctx, file)
		if err != nil {
			return nil, err
		}

		if component != nil {
			return resolveValues(ctx, l, v, s, pctx, component, decls)
		}
	}

	content, err := readValuesFile(ctx, l, v, s, pctx, filepath.Dir(file.ConfigPath))
	if err != nil || content == nil {
		return nil, err
	}

	return &content.value, nil
}

// declaringComponent returns the unit or stack config beside file, other than file, that has a values block, along
// with its declarations. It returns nil when there is none. A config that fails to parse declares nothing.
func declaringComponent(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
) (*hclparse.File, *valuesblock.Declarations, error) {
	dir := filepath.Dir(file.ConfigPath)

	for _, path := range []string{GetDefaultConfigPath(v.FS, dir), filepath.Join(dir, DefaultStackFile)} {
		if filepath.Clean(path) == filepath.Clean(file.ConfigPath) || !vfs.IsFile(v.FS, path) {
			continue
		}

		parsed, err := configFile(ctx, l, v, s, pctx.WithDiagnosticsDiscarded(), path)
		if err != nil {
			l.Debugf("Not reading the values block of %s, which does not parse: %v", path, err)
			continue
		}

		decls, diags := valuesblock.Decode(parsed.Body)
		if decls == nil {
			continue
		}

		component := parsed.Rebind(pctx.NewParser(l, v))

		if err := component.HandleDiagnostics(diags); err != nil {
			return nil, nil, err
		}

		return component, decls, nil
	}

	return nil, nil, nil
}

// resolveValues resolves the terragrunt.values.hcl beside file against decls, the values block of file.
func resolveValues(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	decls *valuesblock.Declarations,
) (*cty.Value, error) {
	content, err := readValuesFile(ctx, l, v, s, pctx, filepath.Dir(file.ConfigPath))
	if err != nil {
		return nil, err
	}

	files := []*hclparse.File{file}
	supplied := valuesblock.NothingSupplied(decls.DefRange)

	if content != nil {
		files = append(files, content.file)
		supplied = valuesblock.SuppliedByAttributes(content.value, content.attrs, content.file.Body.MissingItemRange())
	}

	resolved, diags := decls.Resolve(supplied, valuesblock.Component{
		Kind:     declaringKind(file.ConfigPath),
		Location: util.RelPathForLog(pctx.RootWorkingDir, file.ConfigPath, pctx.LogShowAbsPaths),
	})

	if err := handleValuesDiagnostics(files, diags); err != nil {
		return nil, err
	}

	return &resolved, nil
}

// declaringKind returns the kind of the component whose config is at configPath.
func declaringKind(configPath string) valuesblock.Kind {
	if filepath.Base(configPath) == DefaultStackFile {
		return valuesblock.KindStack
	}

	return valuesblock.KindUnit
}

// readValuesFile parses and evaluates the terragrunt.values.hcl in dir. It returns nil when dir has none.
func readValuesFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	dir string,
) (*valuesFileContent, error) {
	filePath := filepath.Join(dir, valuesFile)

	exists, err := vfs.FileExists(v.FS, filePath)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil
	}

	l.Debugf("Reading Terragrunt values file at %s", filePath)

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, filePath)
	if err != nil {
		return nil, err
	}

	pctx, err = withValuesPathVariables(ctx, l, v, s, pctx, file, dir)
	if err != nil {
		return nil, err
	}

	evalCtx := evalContextWithoutIncludes(ctx, l, v, s, pctx, file.ConfigPath, file.FunctionCalls())

	attrs := hcl.Attributes{}
	if err := file.Decode(&attrs, evalCtx); err != nil {
		return nil, err
	}

	value, diags := valuesblock.EvaluateAttributes(attrs, evalCtx)
	if err := file.HandleDiagnostics(diags); err != nil {
		return nil, err
	}

	return &valuesFileContent{file: file, attrs: attrs, value: value}, nil
}

// handleValuesDiagnostics passes each of diags through the handlers of the file in files that its subject points at,
// and joins the errors the handlers return. Each subject lies in one of files, as resolveValues builds the supply from
// the values file, or from the values block of the first file when there is none.
func handleValuesDiagnostics(files []*hclparse.File, diags hcl.Diagnostics) error {
	byFile := make([]hcl.Diagnostics, len(files))

	for _, diag := range diags {
		i := slices.IndexFunc(files, func(file *hclparse.File) bool {
			return file.Body.MissingItemRange().Filename == diag.Subject.Filename
		})

		byFile[i] = append(byFile[i], diag)
	}

	errs := make([]error, 0, len(files))

	for i, file := range files {
		if err := file.HandleDiagnostics(byFile[i]); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// fileRole returns [IncludedConfig] when file is a unit or stack autoinclude file, and [ComponentConfig] otherwise.
func fileRole(file *hclparse.File) ConfigRole {
	switch filepath.Base(file.ConfigPath) {
	case DefaultAutoIncludeFile, DefaultAutoIncludeStackFile:
		return IncludedConfig
	}

	return ComponentConfig
}
