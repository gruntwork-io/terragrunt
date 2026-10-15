package validate

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

const (
	missingDependencySummary = "Missing dependency"
	missingDirDetail         = "This directory does not exist."
	missingFileDetail        = "This file does not exist."

	dependencyBlock     = "dependency"
	dependenciesBlock   = "dependencies"
	configPathAttribute = "config_path"
	pathsAttribute      = "paths"
)

// MissingDependencyError reports a dependency path that has no unit configuration.
type MissingDependencyError struct {
	// Path is the directory of the dependency.
	Path string
	// ConfigFile is the name of the unit configuration file that validation looked for in Path.
	ConfigFile string
	// Dependents are the directories of the units that depend on Path, sorted.
	Dependents []string
	// DirExists reports whether Path is a directory.
	DirExists bool
}

func (err MissingDependencyError) Error() string {
	dependents := strings.Join(err.Dependents, ", ")

	if !err.DirExists {
		return fmt.Sprintf("dependency of %s: %s does not exist", dependents, err.Path)
	}

	return fmt.Sprintf("dependency of %s: %s has no %s", dependents, err.Path, err.ConfigFile)
}

// detail is the detail of a diagnostic that points at the expression declaring the dependency.
func (err MissingDependencyError) detail() string {
	if !err.DirExists {
		return missingDirDetail
	}

	return missingConfigDetail(err.ConfigFile)
}

// asMissingDependency returns an error when c is a dependency of other units and has no configFile.
// Graph filters add such a unit to the discovered set from the dependency path alone.
func asMissingDependency(
	fsys vfs.FS,
	c component.Component,
	configFile string,
) (*MissingDependencyError, error) {
	dependents := c.Dependents()
	if len(dependents) == 0 {
		return nil, nil
	}

	dirExists := vfs.IsDir(fsys, c.Path())
	if !dirExists && vfs.Exists(fsys, c.Path()) {
		return nil, nil
	}

	if exists, err := hasUnitConfig(fsys, c.Path(), configFile); err != nil || exists {
		return nil, err
	}

	paths := dependents.Paths()
	slices.Sort(paths)

	return &MissingDependencyError{
		Path:       c.Path(),
		ConfigFile: configFile,
		Dependents: paths,
		DirExists:  dirExists,
	}, nil
}

// isFileDependency reports whether c is a dependency whose `config_path` names an existing file instead
// of a directory, which discovery records as a unit at the path of that file.
//
// This form of `config_path` is deprecated. It is kept working for backwards compatibility, and the
// dependency-config-path-file strict control rejects it. The dependent checks that the file exists,
// and the path is not a directory that could be validated as a unit.
func isFileDependency(fsys vfs.FS, c component.Component) bool {
	if len(c.Dependents()) == 0 {
		return false
	}

	info, err := fsys.Stat(c.Path())

	return err == nil && !info.IsDir()
}

// reportMissingDependencies points a diagnostic at each expression that declares one of the missing
// dependencies, in every dependent whose own configuration spells the path out. It returns the missing
// dependencies that got no diagnostic and that no dependent reported while it was validated.
func reportMissingDependencies(
	fsys vfs.FS,
	collector *DiagnosticsCollector,
	missing []*MissingDependencyError,
	unitErrs map[string]error,
) []error {
	var (
		errs    []error
		sources = make(dependencySources)
	)

	for _, missingErr := range missing {
		reported, err := collector.pointAtDependents(fsys, sources, missingErr, unitErrs)
		if err != nil {
			errs = append(errs, err)
		}

		if !reported && !isUngeneratedStackUnit(fsys, missingErr.Path) {
			errs = append(errs, missingErr)
		}
	}

	return errs
}

// withoutDecidedDependencies returns err without the errors discovery raises for the configuration it
// looks for at each dependency validation has already decided on: the ones in missing, and the ones in
// fileDependencies, whose `config_path` names a file.
func withoutDecidedDependencies(
	err error,
	missing []*MissingDependencyError,
	fileDependencies []string,
	configFile string,
) error {
	if err == nil {
		return nil
	}

	configPaths := make(map[string]struct{}, len(missing)+len(fileDependencies))

	for _, missingErr := range missing {
		configPaths[filepath.Join(missingErr.Path, missingErr.ConfigFile)] = struct{}{}
	}

	for _, path := range fileDependencies {
		configPaths[filepath.Join(filepath.Dir(path), configFile)] = struct{}{}
		configPaths[filepath.Join(path, configFile)] = struct{}{}
	}

	rest := slices.DeleteFunc(joinedErrors(err), func(leaf error) bool {
		notFound, ok := errors.AsType[config.TerragruntConfigNotFoundError](leaf)
		if !ok {
			return false
		}

		_, decided := configPaths[notFound.Path]

		return decided
	})

	return errors.Join(rest...)
}

// isUngeneratedStackUnit reports whether path is inside the generated directory of a stack that has a
// stack file, which is where `stack generate` writes the units of that stack.
func isUngeneratedStackUnit(fsys vfs.FS, path string) bool {
	for dir := filepath.Dir(path); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if filepath.Base(dir) != inthclparse.StackDir {
			continue
		}

		if vfs.Exists(fsys, filepath.Join(filepath.Dir(dir), config.DefaultStackFile)) {
			return true
		}
	}

	return false
}

// reportsMissingDependency reports whether err has a missing `dependency` or `dependencies` target in its tree.
func reportsMissingDependency(err error) bool {
	if _, ok := errors.AsType[config.DependencyConfigNotFound](err); ok {
		return true
	}

	_, ok := errors.AsType[config.DependencyDirNotFoundError](err)

	return ok
}

// dependencySource is a configuration file read for the expressions that declare its dependencies.
// Nothing in it is evaluated.
type dependencySource struct {
	// file is the parsed configuration, which diagnostics read their source snippet from.
	file *hcl.File
	// configPaths maps the name of each `dependency` block to its `config_path` expression.
	configPaths map[string]hcl.Expression
	// paths has the elements of the `paths` list of each `dependencies` block.
	paths []hcl.Expression
}

// dependencySources holds each configuration file read for its dependencies, by path.
type dependencySources map[string]*dependencySource

// read returns the configuration file at path, reading it the first time it is asked for.
func (s dependencySources) read(fsys vfs.FS, path string) (*dependencySource, error) {
	if source, ok := s[path]; ok {
		return source, nil
	}

	source, err := readDependencySource(fsys, path)
	if err != nil {
		return nil, err
	}

	s[path] = source

	return source, nil
}

// hasUnitConfig reports whether dir has configFile or one of the default configuration files, which a
// `dependency` block resolves its `config_path` to.
func hasUnitConfig(fsys vfs.FS, dir, configFile string) (bool, error) {
	for _, name := range slices.Concat([]string{configFile}, config.DefaultTerragruntConfigPaths) {
		exists, err := vfs.FileExists(fsys, filepath.Join(dir, name))
		if err != nil || exists {
			return exists, err
		}
	}

	return false, nil
}

// readDependencySource reads the configuration file at path.
func readDependencySource(fsys vfs.FS, path string) (*dependencySource, error) {
	src, err := vfs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}

	parse := hclparse.NewParser().ParseHCL
	if strings.HasSuffix(path, ".json") {
		parse = hclparse.NewParser().ParseJSON
	}

	file, diags := parse(src, path)
	if file == nil {
		return nil, diags
	}

	source := &dependencySource{file: file, configPaths: make(map[string]hcl.Expression)}

	content, _, _ := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: dependencyBlock, LabelNames: []string{"name"}},
			{Type: dependenciesBlock},
		},
	})

	for _, block := range content.Blocks {
		attrs, _, _ := block.Body.PartialContent(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{{Name: configPathAttribute}, {Name: pathsAttribute}},
		})

		if attr, ok := attrs.Attributes[configPathAttribute]; ok && block.Type == dependencyBlock {
			source.configPaths[block.Labels[0]] = attr.Expr
		}

		if attr, ok := attrs.Attributes[pathsAttribute]; ok && block.Type == dependenciesBlock {
			elements, _ := hcl.ExprList(attr.Expr)
			source.paths = append(source.paths, elements...)
		}
	}

	return source, nil
}

// expressions returns every expression in the source that declares a dependency.
func (s *dependencySource) expressions() []hcl.Expression {
	exprs := make([]hcl.Expression, 0, len(s.configPaths)+len(s.paths))

	for _, expr := range s.configPaths {
		exprs = append(exprs, expr)
	}

	exprs = append(exprs, s.paths...)

	slices.SortFunc(exprs, func(a, b hcl.Expression) int {
		return a.Range().Start.Byte - b.Range().Start.Byte
	})

	return exprs
}

// missingConfigDetail is the detail of a diagnostic for a dependency on a directory that exists without
// configFile.
func missingConfigDetail(configFile string) string {
	return "This directory has no " + configFile + "."
}

// missingConfigPathDetail is the detail of a diagnostic for a `config_path` that resolves to path, where
// there is no configuration. It names the directory as missing when that is why.
func missingConfigPathDetail(fsys vfs.FS, path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(path)

	switch {
	case isConfigFileName(name) && vfs.IsDir(fsys, filepath.Dir(path)):
		return missingConfigDetail(name)
	case isConfigFileName(name):
		return missingDirDetail
	case ext == ".hcl" || ext == ".json":
		return missingFileDetail
	}

	return missingDirDetail
}

// literalPath returns the path expr spells out as a string literal, resolved against dir.
func literalPath(expr hcl.Expression, dir string) (string, bool) {
	val, diags := expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
		return "", false
	}

	path := filepath.FromSlash(val.AsString())
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	return path, true
}

// addMissingDependency collects a diagnostic that points at expr in file, which declares a missing
// dependency of the unit in unitDir.
func (c *DiagnosticsCollector) addMissingDependency(
	unitDir string,
	file *hcl.File,
	expr hcl.Expression,
	detail string,
) error {
	diag, err := diagnostic.NewDiagnostic(file, &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  missingDependencySummary,
		Detail:   detail,
		Subject:  expr.Range().Ptr(),
	})
	if err != nil {
		return err
	}

	from := len(c.diags)

	c.add(diag)
	c.attribute(from, unitDir)

	if c.missingDependencyUnits == nil {
		c.missingDependencyUnits = make(map[string]struct{})
	}

	c.missingDependencyUnits[unitDir] = struct{}{}

	return nil
}

// reportedMissingDependency reports whether a diagnostic points at a missing dependency of the unit in unitDir.
func (c *DiagnosticsCollector) reportedMissingDependency(unitDir string) bool {
	_, ok := c.missingDependencyUnits[unitDir]

	return ok
}

// pointAtDependents collects a diagnostic in each dependent of missingErr whose own configuration spells
// the missing path out. It reports whether any dependent has a diagnostic for a missing dependency, or
// reported one in unitErrs while it was validated.
func (c *DiagnosticsCollector) pointAtDependents(
	fsys vfs.FS,
	sources dependencySources,
	missingErr *MissingDependencyError,
	unitErrs map[string]error,
) (bool, error) {
	var (
		errs     []error
		reported bool
		detail   = missingErr.detail()
	)

	for _, dependent := range missingErr.Dependents {
		pointed, err := c.pointAtDependency(
			fsys,
			sources,
			dependent,
			filepath.Join(dependent, missingErr.ConfigFile),
			missingErr.Path,
			detail,
		)
		if err != nil {
			errs = append(errs, err)
		}

		if pointed || c.reportedMissingDependency(dependent) ||
			reportsMissingDependency(unitErrs[dependent]) {
			reported = true
		}
	}

	return reported, errors.Join(errs...)
}

// pointAtDependency collects a diagnostic for each expression in the configuration at configPath that
// spells out target, a dependency of the unit in unitDir. It reports whether it found one.
func (c *DiagnosticsCollector) pointAtDependency(
	fsys vfs.FS,
	sources dependencySources,
	unitDir, configPath, target, detail string,
) (bool, error) {
	source, err := sources.read(fsys, configPath)
	if err != nil {
		return false, err
	}

	target = vfs.ResolveForCompare(fsys, target)
	found := false

	for _, expr := range source.expressions() {
		path, ok := literalPath(expr, unitDir)
		if !ok {
			continue
		}

		path = vfs.ResolveForCompare(fsys, path)
		if path != target && !isConfigFileIn(path, target) {
			continue
		}

		if err := c.addMissingDependency(unitDir, source.file, expr, detail); err != nil {
			return found, err
		}

		found = true
	}

	return found, nil
}

// isConfigFileIn reports whether path is a unit configuration file directly inside dir.
func isConfigFileIn(path, dir string) bool {
	return filepath.Dir(path) == dir && isConfigFileName(filepath.Base(path))
}

// isConfigFileName reports whether name is one of the default names of a unit configuration file.
func isConfigFileName(name string) bool {
	return name == config.DefaultTerragruntConfigPath ||
		name == config.DefaultTerragruntJSONConfigPath
}

// pointAtDependencyErrors moves the missing dependencies in err's tree into diagnostics that point at the
// expression declaring each one, and returns the errors it found no expression for. configPaths are the
// files that can declare a dependency of the unit in unitDir, its own configuration first.
func (c *DiagnosticsCollector) pointAtDependencyErrors(
	fsys vfs.FS,
	unitDir string,
	configPaths []string,
	err error,
) error {
	if !reportsMissingDependency(err) {
		return err
	}

	sources := make([]*dependencySource, 0, len(configPaths))

	for _, configPath := range configPaths {
		source, readErr := readDependencySource(fsys, configPath)
		if readErr != nil {
			return errors.Join(err, readErr)
		}

		sources = append(sources, source)
	}

	var rest []error

	for _, leaf := range joinedErrors(err) {
		pointed, pointErr := c.pointAtDependencyError(fsys, unitDir, sources, leaf)
		if pointErr != nil {
			return errors.Join(err, pointErr)
		}

		if !pointed {
			rest = append(rest, leaf)
		}
	}

	return errors.Join(rest...)
}

// pointAtDependencyError collects the diagnostics for leaf when it reports a missing dependency that one
// of sources declares, and reports whether it did.
func (c *DiagnosticsCollector) pointAtDependencyError(
	fsys vfs.FS,
	unitDir string,
	sources []*dependencySource,
	leaf error,
) (bool, error) {
	if blockErr, ok := errors.AsType[config.DependencyBlockError](leaf); ok {
		return c.pointAtDependencyBlock(fsys, unitDir, sources, blockErr)
	}

	if dirErr, ok := errors.AsType[config.DependencyDirNotFoundError](leaf); ok {
		return c.pointAtDependencyDirs(fsys, unitDir, sources[0], dirErr)
	}

	return false, nil
}

// pointAtDependencyBlock collects a diagnostic at the `config_path` of the dependency block blockErr names,
// when blockErr reports a missing configuration and one of sources declares the block.
func (c *DiagnosticsCollector) pointAtDependencyBlock(
	fsys vfs.FS,
	unitDir string,
	sources []*dependencySource,
	blockErr config.DependencyBlockError,
) (bool, error) {
	notFound, ok := errors.AsType[config.DependencyConfigNotFound](blockErr.Err)
	if !ok {
		return false, nil
	}

	for _, source := range sources {
		expr, ok := source.configPaths[blockErr.Name]
		if !ok {
			continue
		}

		err := c.addMissingDependency(
			unitDir,
			source.file,
			expr,
			missingConfigPathDetail(fsys, notFound.Path),
		)

		return err == nil, err
	}

	return false, nil
}

// pointAtDependencyDirs collects a diagnostic at each `dependencies` path in source that is not a directory.
// It collects none unless those paths account for every directory dirErr reports.
func (c *DiagnosticsCollector) pointAtDependencyDirs(
	fsys vfs.FS,
	unitDir string,
	source *dependencySource,
	dirErr config.DependencyDirNotFoundError,
) (bool, error) {
	type missingDir struct {
		expr hcl.Expression
		path string
	}

	var missing []missingDir

	for _, expr := range source.paths {
		path, ok := literalPath(expr, unitDir)
		if ok && !vfs.IsDir(fsys, path) {
			missing = append(missing, missingDir{expr: expr, path: path})
		}
	}

	if len(missing) != len(dirErr.Dir) {
		return false, nil
	}

	for _, dir := range missing {
		if err := c.addMissingDependency(
			unitDir,
			source.file,
			dir.expr,
			missingDirDetail,
		); err != nil {
			return false, err
		}
	}

	return true, nil
}

// joinedErrors returns the errors that the joins in err's tree combine. An error that is not a join is
// returned as it is, with whatever it wraps.
func joinedErrors(err error) []error {
	var (
		leaves  []error
		pending = []error{err}
	)

	for len(pending) > 0 {
		next := pending[0]
		pending = pending[1:]

		joined, ok := next.(interface{ Unwrap() []error })
		if !ok {
			leaves = append(leaves, next)

			continue
		}

		pending = append(joined.Unwrap(), pending...)
	}

	return leaves
}

// dependencyConfigPaths returns the files that can declare a dependency of the unit configured at
// configPath: the configuration itself, then the files it includes.
func dependencyConfigPaths(configPath string, cfg *config.TerragruntConfig) []string {
	includes := make([]string, 0, len(cfg.ProcessedIncludes))

	for _, include := range cfg.ProcessedIncludes {
		path := include.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(configPath), path)
		}

		includes = append(includes, path)
	}

	slices.Sort(includes)

	return append([]string{configPath}, includes...)
}
