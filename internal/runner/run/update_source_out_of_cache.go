package run

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RewriteRelativePathsOutOfCache repairs relative filesystem references in the
// OpenTofu/Terraform files that were copied from unitDir into workingDir.
//
// When a unit has no terraform.source, Terragrunt copies its .tf files several
// directory levels down into .terragrunt-cache before running OpenTofu/Terraform.
// The copy preserves the unit's own directory structure, so references that stay
// inside the unit keep resolving, but any relative path that climbed out of the
// unit (for example a module block whose source is "../modules/foo") now points
// at the wrong place. Prefixing each such reference with the relative path from
// workingDir back to unitDir restores the original target.
//
// Only native HCL files (.tf, .tofu) are rewritten; the .json variants are left
// untouched because the native parser cannot read them.
func RewriteRelativePathsOutOfCache(l log.Logger, fsys vfs.FS, unitDir, workingDir string) error {
	// The copy reproduces the unit's directory tree under workingDir, so the
	// hop from any copied file back to its original location is the same as the
	// hop from workingDir to unitDir, regardless of how deep the file sits.
	escapePrefix, err := filepath.Rel(workingDir, unitDir)
	if err != nil {
		return err
	}

	escapePrefix = filepath.ToSlash(escapePrefix)

	files, err := tfFilesInCache(fsys, workingDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		fileDir, err := filepath.Rel(workingDir, filepath.Dir(file))
		if err != nil {
			return err
		}

		if err := rewriteFileRelativePaths(
			l,
			fsys,
			file,
			filepath.ToSlash(fileDir),
			escapePrefix,
		); err != nil {
			return err
		}
	}

	return nil
}

// tfFilesInCache returns the native HCL files (.tf, .tofu) under root, skipping
// the .json variants the parser cannot read.
func tfFilesInCache(fsys vfs.FS, root string) ([]string, error) {
	var files []string

	err := vfs.WalkDir(fsys, root, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !util.IsTFFile(filePath) || strings.HasSuffix(filePath, ".json") {
			return nil
		}

		files = append(files, filePath)

		return nil
	})

	return files, err
}

// rewriteFileRelativePaths rewrites the escaping relative paths in a single
// file. fileDir is the file's directory relative to the copied unit root (in
// slash form); escapePrefix is the relative path from the copied file back to
// its original location. A file that fails to parse is left unchanged: the real
// OpenTofu/Terraform run reports the syntax error with better context.
func rewriteFileRelativePaths(l log.Logger, fsys vfs.FS, file, fileDir, escapePrefix string) error {
	content, err := vfs.ReadFile(fsys, file)
	if err != nil {
		return err
	}

	parsed, diags := hclsyntax.ParseConfig(content, file, hcl.InitialPos)
	if diags.HasErrors() {
		l.Debugf("Skipping relative path rewrite for %s: %s", file, diags.Error())
		return nil
	}

	body, ok := parsed.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	var edits []pathEdit

	// Walk only accumulates the diagnostics its Enter/Exit callbacks return, and
	// walkFunc returns none from either, so a non-empty result would mean that
	// invariant was broken rather than a real parse failure.
	if diags := hclsyntax.Walk(body, walkFunc(func(node hclsyntax.Node) {
		tmpl, ok := node.(*hclsyntax.TemplateExpr)
		if !ok {
			return
		}

		value, diags := tmpl.Value(nil)
		if diags.HasErrors() || value.Type() != cty.String {
			return
		}

		literal := value.AsString()
		if !escapesUnitDir(fileDir, literal) {
			return
		}

		rng := tmpl.Range()
		edits = append(edits, pathEdit{
			start: rng.Start.Byte,
			end:   rng.End.Byte,
			replacement: hclwrite.TokensForValue(cty.StringVal(escapePrefix + "/" + literal)).
				Bytes(),
		})
	})); diags.HasErrors() {
		panic("static HCL walk returned diagnostics: " + diags.Error())
	}

	if len(edits) == 0 {
		return nil
	}

	info, err := fsys.Stat(file)
	if err != nil {
		return err
	}

	return vfs.WriteFile(fsys, file, applyPathEdits(content, edits), info.Mode().Perm())
}

// escapesUnitDir reports whether the relative path value, resolved against
// fileDir within the copied unit tree, climbs above the unit root. Only paths
// written with an explicit "./" or "../" prefix are considered: those are the
// forms OpenTofu/Terraform accepts for local references, and requiring the
// prefix keeps unrelated strings (remote sources, version constraints, arbitrary
// text) from being mistaken for paths.
func escapesUnitDir(fileDir, value string) bool {
	if !strings.HasPrefix(value, "./") && !strings.HasPrefix(value, "../") {
		return false
	}

	resolved := path.Join(fileDir, value)

	return resolved == ".." || strings.HasPrefix(resolved, "../")
}

// pathEdit is a byte-range replacement within a file's contents.
type pathEdit struct {
	replacement []byte
	start       int
	end         int
}

// applyPathEdits returns a new buffer with every edit applied. Edits come from
// distinct, non-overlapping string literals; sorting them by offset lets the
// buffer be stitched front-to-back with the unedited spans between them. The
// walk that produced them visits HCL attributes in map order, so the input is
// not already sorted.
func applyPathEdits(content []byte, edits []pathEdit) []byte {
	slices.SortFunc(edits, func(a, b pathEdit) int {
		return a.start - b.start
	})

	var out []byte

	cursor := 0

	for _, e := range edits {
		out = append(out, content[cursor:e.start]...)
		out = append(out, e.replacement...)
		cursor = e.end
	}

	return append(out, content[cursor:]...)
}

// walkFunc adapts a plain visitor into an [hclsyntax.Walker]; only node entry
// is of interest here.
type walkFunc func(hclsyntax.Node)

func (f walkFunc) Enter(node hclsyntax.Node) hcl.Diagnostics {
	f(node)
	return nil
}

func (f walkFunc) Exit(hclsyntax.Node) hcl.Diagnostics {
	return nil
}
