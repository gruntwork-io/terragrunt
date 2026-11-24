package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fmtMaxRunes caps each diff and parse error in a fmt result, so one large
// file cannot balloon the response.
const fmtMaxRunes = 8192

type fmtInput struct {
	WorkingDir string   `json:"working_dir,omitempty" jsonschema:"Directory whose HCL files to format, relative to the server root. Defaults to the server root."`
	File       string   `json:"file,omitempty"        jsonschema:"Path of a single .hcl file to format, relative to working_dir. When set, the directory is not walked and filter must be empty."`
	Filter     []string `json:"filter,omitempty"      jsonschema:"Terragrunt filter queries on file paths (e.g. './stage/**' or '!./modules/**') to narrow which files are formatted."`
	Check      bool     `json:"check,omitempty"       jsonschema:"Report the files that need formatting without changing any."`
	Diff       bool     `json:"diff,omitempty"        jsonschema:"Include a unified diff for each file that needs formatting."`
}

type fmtFile struct {
	Path string `json:"path"`
	Diff string `json:"diff,omitempty"`
}

type fmtOutput struct {
	Changed     []fmtFile `json:"changed,omitempty"`
	ParseErrors []string  `json:"parse_errors,omitempty"`
	Skipped     []string  `json:"skipped,omitempty"`
	FileCount   int       `json:"file_count"`
	Formatted   bool      `json:"formatted"`
}

func registerFmt(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "fmt",
		Description: "Rewrite the .hcl files under a directory into canonical HCL format, as 'terragrunt hcl fmt' " +
			"does, or a single file via the optional 'file' argument. Use after editing .hcl files. Set " +
			"check=true to report which files need formatting without changing any. 'changed' lists the files " +
			"whose formatting differed: rewritten, or left as they were under check=true. diff=true adds a " +
			"unified diff to each. A file that does not parse is left untouched and reported in parse_errors. " +
			"A file found to be a symlink or otherwise non-regular when fmt reaches it is neither read nor " +
			"written, and comes back in skipped. " +
			"formatted=true means every file read was already formatted and parsed.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(false),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, toolHandler(l, "fmt", func(ctx context.Context, input fmtInput) (fmtOutput, error) {
		return runFmt(ctx, l, d, rootVenv, input)
	}))
}

func runFmt(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input fmtInput,
) (fmtOutput, error) {
	if input.File != "" && len(input.Filter) > 0 {
		return fmtOutput{}, errors.New("the 'file' and 'filter' arguments are mutually exclusive")
	}

	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return fmtOutput{}, err
	}

	files, err := fmtSelectFiles(l, rootVenv.FS, d.launchDir, dir, input)
	if err != nil {
		return fmtOutput{}, err
	}

	out := fmtOutput{FileCount: len(files)}

	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return fmtOutput{}, err
		}

		if err := fmtFormatFile(l, rootVenv, dir, path, input, &out); err != nil {
			return fmtOutput{}, err
		}
	}

	out.Formatted = len(out.Changed) == 0 && len(out.ParseErrors) == 0

	return out, nil
}

// fmtSelectFiles returns the files a fmt call examines: the file argument,
// whose directory must resolve inside the server root, or else the files
// [format.Files] finds under dir.
func fmtSelectFiles(
	l log.Logger,
	fsys vfs.FS,
	launchDir, dir string,
	input fmtInput,
) ([]string, error) {
	if input.File == "" {
		filters, err := filter.ParseFilterQueries(l, input.Filter)
		if err != nil {
			return nil, fmt.Errorf("parsing filter queries: %w", err)
		}

		return format.Files(l, fsys, dir, nil, filters)
	}

	path := input.File
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}

	if filepath.Ext(path) != ".hcl" {
		return nil, fmt.Errorf("file %q is not an .hcl file", input.File)
	}

	parent, err := resolveUnderRoot(fsys, launchDir, "parent directory", filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("file %q: %w", input.File, err)
	}

	return []string{filepath.Join(parent, filepath.Base(path))}, nil
}

// fmtFormatFile formats one file and records the outcome on out. A file
// that is a symlink or otherwise not regular is skipped unread, and one that
// does not parse is left as it is.
func fmtFormatFile(
	l log.Logger,
	v *venv.Venv,
	dir, path string,
	input fmtInput,
	out *fmtOutput,
) error {
	rel := discovery.RelPathOrAbs(l, dir, path, "formatted file")

	info, err := vfs.Lstat(v.FS, path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", rel, err)
	}

	if !info.Mode().IsRegular() {
		out.Skipped = append(out.Skipped, rel)

		return nil
	}

	contents, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", rel, err)
	}

	formatted, err := format.Bytes(l, v, contents, rel)
	if err != nil {
		out.ParseErrors = append(out.ParseErrors, truncateRunes(err.Error(), fmtMaxRunes))

		return nil
	}

	if bytes.Equal(formatted, contents) {
		return nil
	}

	changed := fmtFile{Path: rel}
	if input.Diff {
		changed.Diff = truncateRunes(string(format.BytesDiff(contents, formatted, rel)), fmtMaxRunes)
	}

	out.Changed = append(out.Changed, changed)

	if input.Check {
		return nil
	}

	if err := vfs.WriteFileAtomic(v.FS, path, formatted, info.Mode().Perm()); err != nil {
		return fmt.Errorf("writing %s: %w", rel, err)
	}

	return nil
}
