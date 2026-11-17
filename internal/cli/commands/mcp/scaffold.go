package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	bpconfig "github.com/gruntwork-io/boilerplate/config"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// scaffoldMaxEntries caps how many entries the safety walk of a
	// downloaded source and the listing of the generated files visit.
	scaffoldMaxEntries = 50_000

	// scaffoldFilesShown caps how many generated files a result lists.
	scaffoldFilesShown = 200

	scaffoldDirPerms = 0o755
)

type scaffoldInput struct {
	Values       map[string]string `json:"values,omitempty"        jsonschema:"Values to write, keyed by name: a module's input variables, or the values.* a unit or stack references. Each is an HCL expression, so a string is quoted. A name left out is written as a TODO placeholder."`
	Vars         map[string]string `json:"vars,omitempty"          jsonschema:"Boilerplate variables for the template, as the scaffold command's --var sets them. Each value is parsed as YAML. Only a module is rendered from a template."`
	Source       string            `json:"source"                  jsonschema:"The module, unit, or stack to scaffold from, as a source URL such as a catalog lists. A relative local path is resolved against working_dir, and the generated terraform source is written relative to output_folder."`
	WorkingDir   string            `json:"working_dir,omitempty"   jsonschema:"Directory to scaffold from, relative to the server root, as the scaffold command's --working-dir. A relative source, output_folder, or var_files entry is resolved against it, and the root configuration and catalog block are looked up from it. Defaults to the server root."`
	OutputFolder string            `json:"output_folder,omitempty" jsonschema:"Directory to write into, relative to working_dir, as the scaffold command's --output-folder. It must be empty or not exist yet. Defaults to working_dir."`
	Template     string            `json:"template,omitempty"      jsonschema:"Boilerplate template URL to render a module with, in place of the module's own .boilerplate directory, the catalog default_template, or the built-in template. Give a local template as an absolute path."`
	VarFiles     []string          `json:"var_files,omitempty"     jsonschema:"YAML files of boilerplate variables, relative to working_dir, as the scaffold command's --var-file reads them."`
}

type scaffoldOutput struct {
	OutputFolder string   `json:"output_folder"`
	Kind         string   `json:"kind"`
	Files        []string `json:"files"`
	TODO         []string `json:"todo,omitempty"`
	Degraded     []string `json:"degraded,omitempty"`
	FileCount    int      `json:"file_count"`
}

func registerScaffold(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "scaffold",
		Description: "Generate new Terragrunt configuration from a module, unit, or stack source, as " +
			"'terragrunt scaffold' does, without prompting. A module gets a terragrunt.hcl rendered from a " +
			"boilerplate template; a unit or stack is copied with a terragrunt.values.hcl for the values it " +
			"references. Pass what to fill in as values; anything left out is written as a TODO placeholder " +
			"and listed in todo. output_folder, which defaults to working_dir, must be empty or not exist, and " +
			"the call writes nothing anywhere else. A failed call removes what it wrote. Template shell helpers " +
			"and hooks never run. A template with dependencies or templated file names is refused. A remote " +
			"source needs --allow=http, or --allow=exec for git. Calls queue behind other runs.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(false),
			OpenWorldHint:   new(d.reachesNetwork()),
		},
	}, toolHandler(l, "scaffold", func(ctx context.Context, input scaffoldInput) (scaffoldOutput, error) {
		return runScaffold(ctx, l, d.forCall(nil), rootVenv, &input)
	}))
}

func runScaffold(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input *scaffoldInput,
) (scaffoldOutput, error) {
	if err := input.validate(); err != nil {
		return scaffoldOutput{}, err
	}

	workingDir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return scaffoldOutput{}, err
	}

	varFiles := make([]string, 0, len(input.VarFiles))

	for _, file := range input.VarFiles {
		path, err := resolveUnderRoot(
			rootVenv.FS,
			d.launchDir,
			"var_files entry",
			scaffoldPath(workingDir, file),
		)
		if err != nil {
			return scaffoldOutput{}, err
		}

		if !vfs.IsFile(rootVenv.FS, path) {
			return scaffoldOutput{}, fmt.Errorf(
				"var_files entry %q: %w",
				file,
				ErrScaffoldVarFileNotFile,
			)
		}

		varFiles = append(varFiles, path)
	}

	release, err := d.acquireRunSlot(ctx)
	if err != nil {
		return scaffoldOutput{}, err
	}

	defer release()

	outDir, undo, err := scaffoldOutputFolder(
		rootVenv.FS,
		d.launchDir,
		workingDir,
		input.OutputFolder,
	)
	if err != nil {
		return scaffoldOutput{}, err
	}

	out, err := scaffoldInto(ctx, l, d, rootVenv, input, workingDir, outDir, varFiles)
	if err != nil {
		return scaffoldOutput{}, errors.Join(err, undo())
	}

	return out, nil
}

// validate checks the arguments that need no download to judge.
func (input *scaffoldInput) validate() error {
	if input.Source == "" {
		return ErrScaffoldSourceRequired
	}

	for name := range input.Vars {
		if name == "" || strings.Contains(name, "=") {
			return fmt.Errorf("vars has the name %q: %w", name, ErrScaffoldVarName)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(input.Values)) {
		_, diags := hclsyntax.ParseExpression(
			[]byte(input.Values[name]),
			name,
			hcl.Pos{Line: 1, Column: 1},
		)
		if diags.HasErrors() {
			return fmt.Errorf("the value of %q: %w: %w", name, ErrScaffoldValueNotHCL, diags)
		}
	}

	return nil
}

// scaffoldInto scaffolds the source from workingDir into outDir, which
// [scaffoldOutputFolder] has already readied.
func scaffoldInto(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input *scaffoldInput,
	workingDir, outDir string,
	varFiles []string,
) (scaffoldOutput, error) {
	opts, env, err := buildDirOptions(l, d, rootVenv, workingDir, nil)
	if err != nil {
		return scaffoldOutput{}, err
	}

	opts.TerragruntConfigPath = filepath.Join(workingDir, config.DefaultTerragruntConfigPath)
	opts.ScaffoldOutputFolder = outDir
	opts.NoDependencyPrompt = true
	opts.NoShell = true
	opts.NoHooks = true
	opts.ScaffoldVarFiles = varFiles

	for _, name := range slices.Sorted(maps.Keys(input.Vars)) {
		opts.ScaffoldVars = append(opts.ScaffoldVars, name+"="+input.Vars[name])
	}

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	opts.ScaffoldRootFileName = scaffold.GetDefaultRootFileName(ctx, cv.FS, opts)

	plan, err := scaffold.Prepare(ctx, l, cv, opts, input.Source, input.Template)
	if err != nil {
		return scaffoldOutput{}, scaffoldExplainDenials(err, d.rec)
	}

	defer plan.Cleanup(cv.FS)

	if plan.Kind().IsCopyable() &&
		(input.Template != "" || len(input.Vars) > 0 || len(input.VarFiles) > 0) {
		return scaffoldOutput{}, fmt.Errorf(
			"the source is a %s: %w",
			plan.Kind(),
			ErrScaffoldNotAModule,
		)
	}

	if err := scaffoldCheckSource(cv.FS, plan); err != nil {
		return scaffoldOutput{}, err
	}

	todo, err := scaffoldTODO(plan, input.Values)
	if err != nil {
		return scaffoldOutput{}, err
	}

	if err := plan.Generate(ctx, l, cv, opts, input.Values); err != nil {
		return scaffoldOutput{}, fmt.Errorf("generating into %s: %w", outDir, err)
	}

	files, err := scaffoldListFiles(cv.FS, outDir)
	if err != nil {
		return scaffoldOutput{}, err
	}

	out := scaffoldOutput{
		OutputFolder: renderRelPath(d.launchDir, outDir),
		Kind:         plan.Kind().String(),
		Files:        files,
		FileCount:    len(files),
		TODO:         todo,
		Degraded:     d.rec.notes(),
	}

	if len(out.Files) > scaffoldFilesShown {
		out.Files = out.Files[:scaffoldFilesShown]
	}

	return out, nil
}

// scaffoldExplainDenials adds to a failed download the invocations the
// server refused on the way, since a refused git clone surfaces only as a
// failed clone.
func scaffoldExplainDenials(err error, rec *execRecorder) error {
	notes := rec.notes()
	if len(notes) == 0 {
		return err
	}

	return fmt.Errorf("%w (denied: %s)", err, strings.Join(notes, "; "))
}

// scaffoldPath resolves a tool-call path against workingDir, as the scaffold
// command resolves one against --working-dir.
func scaffoldPath(workingDir, requested string) string {
	if filepath.IsAbs(requested) {
		return requested
	}

	return filepath.Join(workingDir, requested)
}

// scaffoldOutputFolder resolves the output_folder argument against
// workingDir, confines it under the server root, and readies it: a missing
// directory is created, and an existing one must be empty. An empty argument
// means workingDir itself. The returned func undoes a failed scaffold. It
// removes the directories created here, or empties the one that already
// existed.
func scaffoldOutputFolder(
	fsys vfs.FS,
	launchDir, workingDir, requested string,
) (string, func() error, error) {
	label := fmt.Sprintf("output_folder %q", requested)
	if requested == "" {
		label = fmt.Sprintf(
			"working_dir %q (the default output_folder)",
			renderRelPath(launchDir, workingDir),
		)
	}

	existing, missing, err := scaffoldSplitExisting(
		fsys,
		filepath.Clean(scaffoldPath(workingDir, requested)),
	)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", label, err)
	}

	existing, err = vfs.EvalSymlinks(fsys, existing)
	if err != nil {
		return "", nil, fmt.Errorf("resolving %s: %w", label, err)
	}

	dir := filepath.Join(append([]string{existing}, missing...)...)

	if !withinRoot(launchDir, dir) {
		return "", nil, fmt.Errorf("%s: %w %s", label, ErrOutsideRoot, launchDir)
	}

	if len(missing) > 0 {
		if err := fsys.MkdirAll(dir, scaffoldDirPerms); err != nil {
			return "", nil, fmt.Errorf("creating %s: %w", label, err)
		}

		created := filepath.Join(existing, missing[0])

		return dir, func() error { return fsys.RemoveAll(created) }, nil
	}

	if !vfs.IsDir(fsys, dir) {
		return "", nil, fmt.Errorf("%s: %w", label, ErrScaffoldOutputFolderNotDir)
	}

	empty, err := vfs.IsDirectoryEmpty(fsys, dir)
	if err != nil {
		return "", nil, fmt.Errorf("reading %s: %w", label, err)
	}

	if !empty {
		return "", nil, fmt.Errorf("%s: %w", label, ErrScaffoldOutputFolderNotEmpty)
	}

	return dir, func() error { return scaffoldEmptyDir(fsys, dir) }, nil
}

// scaffoldSplitExisting splits the clean, absolute path into its longest
// prefix that exists, without following a final symlink, and the names below
// that prefix that do not exist yet.
func scaffoldSplitExisting(fsys vfs.FS, path string) (string, []string, error) {
	var missing []string

	for {
		_, err := vfs.Lstat(fsys, path)
		if err == nil {
			return path, missing, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, err
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", nil, err
		}

		missing = slices.Insert(missing, 0, filepath.Base(path))
		path = parent
	}
}

// scaffoldEmptyDir removes everything in dir and leaves dir itself.
func scaffoldEmptyDir(fsys vfs.FS, dir string) error {
	entries, err := vfs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}

	errs := make([]error, 0, len(entries))

	for _, entry := range entries {
		errs = append(errs, fsys.RemoveAll(filepath.Join(dir, entry.Name())))
	}

	return errors.Join(errs...)
}

// scaffoldCheckSource refuses a downloaded source whose render could read or
// write outside it. Boilerplate works on the OS filesystem directly, where the
// server's own checks do not reach.
//
// Every link in the source must resolve inside it. A module's template must
// sit inside the source too, declare no dependencies, and use no template
// syntax in a file or directory name. Boilerplate downloads a dependency's
// template itself and writes it wherever the template says, and a rendered
// name can climb out of output_folder.
func scaffoldCheckSource(fsys vfs.FS, plan *scaffold.Plan) error {
	root := plan.SourceDir()

	if err := scaffoldCheckLinks(fsys, root); err != nil {
		return err
	}

	if plan.Kind().IsCopyable() {
		return nil
	}

	templateDir := plan.TemplateDir()

	if !withinRoot(root, templateDir) {
		return fmt.Errorf(
			"%w: the template directory is outside the downloaded source",
			ErrScaffoldUnsafeTemplate,
		)
	}

	if err := scaffoldCheckTemplateNames(fsys, templateDir); err != nil {
		return err
	}

	return scaffoldCheckDependencies(fsys, templateDir)
}

// scaffoldCheckLinks refuses a symlink under root whose stored target, or
// the target its chain resolves to, is outside root.
func scaffoldCheckLinks(fsys vfs.FS, root string) error {
	return scaffoldWalk(fsys, root, func(path string, d fs.DirEntry) error {
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}

		target, err := vfs.Readlink(fsys, path)
		if err != nil {
			return err
		}

		if vfs.ValidateSymlinkTarget(root, path, target) == nil &&
			!errors.Is(vfs.ValidateResolvedSymlinkTarget(fsys, root, path), vfs.ErrSymlinkEscapes) {
			return nil
		}

		return fmt.Errorf(
			"%w: %s links outside the source",
			ErrScaffoldUnsafeTemplate,
			tui.SanitizeLabel(renderRelPath(root, path)),
		)
	})
}

// scaffoldCheckTemplateNames refuses a template whose file or directory
// names hold template syntax. Boilerplate renders each name, URL-decoded
// first, into the path it writes.
func scaffoldCheckTemplateNames(fsys vfs.FS, templateDir string) error {
	return scaffoldWalk(fsys, templateDir, func(path string, d fs.DirEntry) error {
		name := d.Name()

		decoded, err := url.QueryUnescape(name)
		if err != nil {
			decoded = name
		}

		if strings.Contains(name, "{{") || strings.Contains(decoded, "{{") {
			return fmt.Errorf(
				"%w: the template path %s contains template syntax, which could render to a path outside output_folder",
				ErrScaffoldUnsafeTemplate,
				tui.SanitizeLabel(renderRelPath(templateDir, path)),
			)
		}

		return nil
	})
}

// scaffoldCheckDependencies refuses a template whose boilerplate.yml
// declares dependencies.
func scaffoldCheckDependencies(fsys vfs.FS, templateDir string) error {
	path := bpconfig.BoilerplateConfigPath(templateDir)

	contents, err := vfs.ReadFile(fsys, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	cfg, err := bpconfig.ParseBoilerplateConfig(contents)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", bpconfig.BoilerplateConfigFile, err)
	}

	if len(cfg.Dependencies) > 0 {
		return fmt.Errorf(
			"%w: the template declares dependencies, which boilerplate downloads and writes past the server's checks",
			ErrScaffoldUnsafeTemplate,
		)
	}

	return nil
}

// scaffoldTODO returns the required names values leaves unset, which the
// generated configuration gets as placeholders.
//
// Returns an error when values sets a name the source does not take, which
// generating would otherwise drop.
func scaffoldTODO(plan *scaffold.Plan, values map[string]string) ([]string, error) {
	fields := plan.FormFields()
	known := make(map[string]struct{}, len(fields))

	var todo []string

	for i := range fields {
		name := fields[i].Name
		known[name] = struct{}{}

		if _, set := values[name]; fields[i].Required && !set {
			todo = append(todo, name)
		}
	}

	var unknown []string

	for name := range values {
		if _, ok := known[name]; !ok {
			unknown = append(unknown, name)
		}
	}

	if len(unknown) == 0 {
		return todo, nil
	}

	slices.Sort(unknown)

	names := slices.Sorted(maps.Keys(known))
	if len(names) == 0 {
		names = []string{"none"}
	}

	return nil, fmt.Errorf(
		"%w: %s; the %s takes %s",
		ErrScaffoldUnknownValue,
		strings.Join(unknown, ", "),
		plan.Kind(),
		strings.Join(names, ", "),
	)
}

// scaffoldListFiles returns the regular files under dir, relative to it and
// sorted.
func scaffoldListFiles(fsys vfs.FS, dir string) ([]string, error) {
	var files []string

	err := scaffoldWalk(fsys, dir, func(path string, d fs.DirEntry) error {
		if d.Type().IsRegular() {
			files = append(files, renderRelPath(dir, path))
		}

		return nil
	})

	slices.Sort(files)

	return files, err
}

// scaffoldWalk calls fn for every entry under root, root itself excluded,
// without following links. It stops with [ErrScaffoldTooManyEntries] past
// scaffoldMaxEntries.
func scaffoldWalk(fsys vfs.FS, root string, fn func(path string, d fs.DirEntry) error) error {
	seen := 0

	return vfs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == root {
			return nil
		}

		seen++
		if seen > scaffoldMaxEntries {
			return ErrScaffoldTooManyEntries
		}

		return fn(path, d)
	})
}
