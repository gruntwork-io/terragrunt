// Command vend regenerates the OpenTofu code vendored in ../upstream.
// [copies] and [extractions] list what is copied from upstream.
//
// Run it from the repository root:
//
//	go run ./internal/vendored/opentofu/vend [--tag tag]
//
// Without --tag, vend regenerates the release recorded in ../VENDOR.md. With
// --tag, it vendors that release and, once the vendored packages build and
// pass their tests, records the tag and its commit in ../VENDOR.md.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	upstreamRepo   = "https://github.com/opentofu/opentofu.git"
	upstreamModule = "github.com/opentofu/opentofu"

	upstreamPkg = "github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/upstream"
	upstreamDir = "upstream"
	vendorDoc   = "VENDOR.md"

	fetchTimeout = 5 * time.Minute

	dirPerms  = 0o755
	filePerms = 0o644

	usageExitCode = 2
)

// tagRow and commitRow match the rows of the Source table in VENDOR.md that
// record the vendored release, capturing the recorded value.
var (
	tagRow    = regexp.MustCompile("(?m)^\\| Tag \\| `([^`]+)` \\|$")
	commitRow = regexp.MustCompile("(?m)^\\| Commit \\| `([^`]+)` \\|$")
)

// copies lists the upstream files and directories vendored whole.
var copies = []string{
	"LICENSE",
	"internal/ipaddr",
	"internal/lang/funcs",
	"internal/lang/types",
	"internal/tfdiags/config_traversals.go",
}

// extractions lists the upstream files of which only some declarations are
// vendored.
var extractions = []extraction{
	{
		from:  "internal/lang/functions.go",
		decls: []string{"makeBaseFunctionTable"},
		// The patch package registers replacements for these. The upstream ones
		// bypass the Terragrunt venv or logger.
		commentOut: []string{
			"abspath",
			"base64decode",
			"file",
			"filebase64",
			"filebase64sha256",
			"filebase64sha512",
			"fileexists",
			"filemd5",
			"fileset",
			"filesha1",
			"filesha256",
			"filesha512",
			"pathexpand",
		},
		replacements: []replacement{
			// Terragrunt builds its function table from this, so it has to be
			// reachable from outside the package.
			{old: "makeBaseFunctionTable", with: "MakeBaseFunctionTable"},
			// convert takes a typeexpr.TypeContext, which only OpenTofu's fork of
			// HCL defines.
			{old: "baseDir string, typeCtx *typeexpr.TypeContext)", with: "baseDir string)"},
			{old: "\t\t\"convert\":             makeConvertFunc(typeCtx),\n", with: ""},
			{
				old: `	ret["templatefile"] = funcs.MakeTemplateFileFunc(baseDir, func() map[string]function.Function {
		// The templatefile function prevents recursive calls to itself
		// by copying this map and overwriting the "templatefile" entry.
		return ret
	})
`,
				with: `	// The patch package registers a vfs-backed replacement for this:
	//
	// ret["templatefile"] = funcs.MakeTemplateFileFunc(baseDir, func() map[string]function.Function {
	// 	return ret
	// })
`,
			},
		},
	},
	{
		from: "internal/lang/marks/marks.go",
		// The rest of upstream marks.go uses addrs and tfdiags, which are not
		// vendored.
		decls: []string{"valueMark", "Has", "Contains", "Sensitive", "Ephemeral", "TypeType"},
	},
}

// patches lists the text vend replaces in the copied files.
var patches = []patch{
	{
		// The patch package's templatefile renders through this, so it has to be
		// reachable from outside the funcs package.
		path: "internal/lang/funcs",
		old:  "renderTemplate",
		with: "RenderTemplate",
	},
	{
		// This error reaches Terragrunt through the patch package's
		// templatefile, which logs the template stack through Terragrunt's
		// logger. An Error method has nothing to log through.
		path: "internal/lang/funcs/filesystem.go",
		old: `	log.Printf("[DEBUG] Template Stack (%d): %s", len(err.sources)-1, err.sources[len(err.sources)-1])

`,
		with: "",
	},
	{
		// The line above was the only use of log in this file.
		path: "internal/lang/funcs/filesystem.go",
		old:  "\t\"log\"\n",
		with: "",
	},
}

// patch replaces every occurrence of one piece of text in a copied upstream
// file, or in the Go files of a copied upstream directory.
type patch struct {
	path string
	old  string
	with string
}

// extraction vendors some top-level declarations of an upstream file.
type extraction struct {
	from         string
	decls        []string
	replacements []replacement
	commentOut   []string
}

// replacement replaces every occurrence of one exact piece of text.
type replacement struct {
	old  string
	with string
}

// release identifies an OpenTofu release by its tag and the commit the tag
// resolves to.
type release struct {
	tag    string
	commit string
}

func main() {
	ref := flag.String(
		"tag",
		"",
		"OpenTofu release `tag` to vendor and record (default: the tag VENDOR.md records)",
	)

	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "vend: unexpected arguments:", strings.Join(flag.Args(), " "))
		flag.Usage()
		os.Exit(usageExitCode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, *ref)

	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, "vend:", err)
		os.Exit(1)
	}
}

// run vendors the OpenTofu release at ref, or the release VENDOR.md records
// when ref is empty, and records a newly vendored release in VENDOR.md.
func run(ctx context.Context, ref string) (err error) {
	dir, err := vendoredDir()
	if err != nil {
		return err
	}

	docPath := filepath.Join(dir, vendorDoc)

	doc, err := os.ReadFile(docPath)
	if err != nil {
		return err
	}

	recorded, err := readRelease(doc)
	if err != nil {
		return err
	}

	if ref == "" {
		ref = recorded.tag
	}

	checkout, err := os.MkdirTemp("", "opentofu-vendor-")
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, os.RemoveAll(checkout))
	}()

	vendored, err := regenerate(ctx, dir, checkout, ref, recorded)
	if err != nil {
		return err
	}

	if vendored == recorded {
		return nil
	}

	if err := writeRelease(docPath, doc, vendored); err != nil {
		return err
	}

	fmt.Fprintf(
		os.Stderr,
		"vend: recorded OpenTofu %s (%s) in %s\n",
		vendored.tag,
		vendored.commit,
		vendorDoc,
	)

	return nil
}

// vendoredDir returns the directory vend's source directory is in, which holds
// VENDOR.md and the upstream directory.
func vendoredDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot find the source file of vend")
	}

	if !filepath.IsAbs(file) {
		return "", fmt.Errorf(
			"source file %s is not an absolute path; run vend without -trimpath",
			file,
		)
	}

	return filepath.Dir(filepath.Dir(file)), nil
}

// readRelease returns the release recorded in the Source table of the
// VENDOR.md content doc, failing unless the table has exactly one Tag row and
// one Commit row.
func readRelease(doc []byte) (release, error) {
	tags := tagRow.FindAllSubmatch(doc, -1)
	commits := commitRow.FindAllSubmatch(doc, -1)

	if len(tags) != 1 || len(commits) != 1 {
		return release{}, fmt.Errorf(
			"found %d Tag and %d Commit rows in the Source table of %s, want one of each",
			len(tags),
			len(commits),
			vendorDoc,
		)
	}

	return release{tag: string(tags[0][1]), commit: string(commits[0][1])}, nil
}

// regenerate rebuilds and verifies the upstream directory in dir from the
// OpenTofu release at ref, cloned into checkout, and returns that release. It
// fails when ref is the recorded tag but no longer resolves to the recorded
// commit.
func regenerate(
	ctx context.Context,
	dir, checkout, ref string,
	recorded release,
) (_ release, err error) {
	fmt.Fprintf(os.Stderr, "vend: fetching OpenTofu %s\n", ref)

	resolved, err := fetch(ctx, checkout, ref)
	if err != nil {
		return release{}, err
	}

	if ref == recorded.tag && resolved != recorded.commit {
		return release{}, fmt.Errorf(
			"tag %s resolves to %s upstream, but %s records %s",
			ref,
			resolved,
			vendorDoc,
			recorded.commit,
		)
	}

	upstream, err := os.OpenRoot(checkout)
	if err != nil {
		return release{}, err
	}

	defer func() {
		err = errors.Join(err, upstream.Close())
	}()

	dst := filepath.Join(dir, upstreamDir)

	if err := os.RemoveAll(dst); err != nil {
		return release{}, err
	}

	src := upstream.FS()

	if err := copyUpstream(src, dst); err != nil {
		return release{}, err
	}

	for i := range extractions {
		if err := writeExtraction(src, dst, &extractions[i]); err != nil {
			return release{}, err
		}
	}

	if err := applyPatches(dst); err != nil {
		return release{}, err
	}

	rewrite := func(data []byte) ([]byte, error) { return rewriteImports(dst, data) }
	if err := transformGoFiles(dst, rewrite); err != nil {
		return release{}, err
	}

	if err := verify(ctx, dst); err != nil {
		return release{}, err
	}

	return release{tag: ref, commit: resolved}, nil
}

// fetch checks out the OpenTofu release at ref into dir and returns the commit
// ref resolves to.
func fetch(ctx context.Context, dir, ref string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	if err := stream(command(ctx, "", "git", "init", "--quiet", "--template=", dir)); err != nil {
		return "", err
	}

	fetchTag := command(
		ctx,
		dir,
		"git",
		"fetch",
		"--quiet",
		"--depth=1",
		"--no-tags",
		upstreamRepo,
		"refs/tags/"+ref,
	)
	if err := stream(fetchTag); err != nil {
		return "", err
	}

	resolved, err := output(command(ctx, dir, "git", "rev-parse", "FETCH_HEAD^{commit}"))
	if err != nil {
		return "", err
	}

	checkout := command(ctx, dir, "git", "checkout", "--quiet", "--detach", resolved)
	if err := stream(checkout); err != nil {
		return "", err
	}

	return resolved, nil
}

// copyUpstream copies each of [copies] from src into dst.
func copyUpstream(src fs.FS, dst string) error {
	for _, from := range copies {
		to := filepath.Join(dst, filepath.FromSlash(vendoredRel(from)))

		info, err := fs.Stat(src, from)
		if err != nil {
			return err
		}

		if info.IsDir() {
			sub, err := fs.Sub(src, from)
			if err != nil {
				return err
			}

			if err := os.CopyFS(to, sub); err != nil {
				return err
			}

			continue
		}

		data, err := fs.ReadFile(src, from)
		if err != nil {
			return err
		}

		if err := writeFile(to, data); err != nil {
			return err
		}
	}

	return nil
}

// writeExtraction writes the file described by e into dst.
func writeExtraction(src fs.FS, dst string, e *extraction) error {
	data, err := fs.ReadFile(src, e.from)
	if err != nil {
		return err
	}

	out, err := extract(e, data)
	if err != nil {
		return fmt.Errorf("%s: %w", e.from, err)
	}

	return writeFile(filepath.Join(dst, filepath.FromSlash(vendoredRel(e.from))), out)
}

// extract builds the vendored file for e from the license header and package
// clause of the upstream file, the declarations e names along with the methods
// on any type among them, and the imports those declarations use once e's
// replacements are applied.
func extract(e *extraction, data []byte) ([]byte, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, e.from, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	offset := func(pos token.Pos) int { return fset.Position(pos).Offset }

	var body bytes.Buffer

	found := map[string]struct{}{}

	for _, decl := range file.Decls {
		name, doc := declInfo(decl)
		if !slices.Contains(e.decls, name) {
			continue
		}

		found[name] = struct{}{}

		start := decl.Pos()
		if doc != nil {
			start = doc.Pos()
		}

		body.Write(data[offset(start):offset(decl.End())])
		body.WriteString("\n\n")
	}

	for _, name := range e.decls {
		if _, ok := found[name]; !ok {
			return nil, fmt.Errorf("declaration %s not found", name)
		}
	}

	decls := body.String()

	for _, r := range e.replacements {
		if !strings.Contains(decls, r.old) {
			return nil, fmt.Errorf("replaced text %q not found", r.old)
		}

		decls = strings.ReplaceAll(decls, r.old, r.with)
	}

	decls, err = commentOutEntries(decls, e.commentOut)
	if err != nil {
		return nil, err
	}

	used, err := qualifiers(decls)
	if err != nil {
		return nil, err
	}

	imports, err := importBlock(fset, file, data, used)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer

	out.Write(data[:offset(file.Name.End())])
	out.WriteString("\n\n")
	out.WriteString(imports)
	out.WriteString(decls)

	return format.Source(out.Bytes())
}

// declInfo returns the name a top-level declaration is selected by and its doc
// comment. A method is selected by its receiver's type name. Imports and
// grouped declarations have no name.
func declInfo(decl ast.Decl) (string, *ast.CommentGroup) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			return d.Name.Name, d.Doc
		}

		recv := d.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}

		if id, ok := recv.(*ast.Ident); ok {
			return id.Name, d.Doc
		}
	case *ast.GenDecl:
		if len(d.Specs) != 1 {
			return "", nil
		}

		switch s := d.Specs[0].(type) {
		case *ast.TypeSpec:
			return s.Name.Name, d.Doc
		case *ast.ValueSpec:
			return s.Names[0].Name, d.Doc
		}
	}

	return "", nil
}

// commentOutEntries comments out the function table entry of each of keys,
// found by its name so that the padding gofmt gives the table does not matter.
// Each commented-out entry ends with a note naming its replacement in
// patch.Functions.
func commentOutEntries(decls string, keys []string) (string, error) {
	for _, key := range keys {
		row := regexp.MustCompile(`(?m)^(\t\t)("` + regexp.QuoteMeta(key) + `":.*)$`)

		if n := len(row.FindAllStringIndex(decls, -1)); n != 1 {
			return "", fmt.Errorf("%d entries named %q to comment out, want 1", n, key)
		}

		decls = row.ReplaceAllString(decls, "${1}// ${2} // Replaced by patch.Functions")
	}

	return decls, nil
}

// qualifiers parses Go top-level declarations and returns the identifiers on
// the left of their selector expressions, which include the names of the
// imports the declarations use.
func qualifiers(decls string) (map[string]struct{}, error) {
	file, err := parser.ParseFile(
		token.NewFileSet(),
		"",
		"package p\n\n"+decls,
		parser.SkipObjectResolution,
	)
	if err != nil {
		return nil, err
	}

	used := map[string]struct{}{}

	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				used[id.Name] = struct{}{}
			}
		}

		return true
	})

	return used, nil
}

// importBlock returns an import declaration with the imports of file whose
// names are in used, grouped as upstream groups them.
func importBlock(
	fset *token.FileSet,
	file *ast.File,
	data []byte,
	used map[string]struct{},
) (string, error) {
	var b strings.Builder

	b.WriteString("import (\n")

	group, prevLine, keptGroup := 0, 0, -1

	for _, spec := range file.Imports {
		line := fset.Position(spec.Pos()).Line
		if prevLine != 0 && line > prevLine+1 {
			group++
		}

		prevLine = line

		name, err := importName(spec)
		if err != nil {
			return "", err
		}

		if _, ok := used[name]; !ok {
			continue
		}

		if keptGroup >= 0 && keptGroup != group {
			b.WriteString("\n")
		}

		keptGroup = group

		b.WriteString("\t")
		b.Write(data[fset.Position(spec.Pos()).Offset:fset.Position(spec.End()).Offset])
		b.WriteString("\n")
	}

	b.WriteString(")\n\n")

	return b.String(), nil
}

// importName returns the name an import is referred to by in the importing
// file.
func importName(spec *ast.ImportSpec) (string, error) {
	if spec.Name != nil {
		return spec.Name.Name, nil
	}

	importPath, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return "", err
	}

	name := path.Base(importPath)

	if version, ok := strings.CutPrefix(name, "v"); ok {
		if _, err := strconv.Atoi(version); err == nil {
			name = path.Base(path.Dir(importPath))
		}
	}

	return name, nil
}

// applyPatches applies each of [patches] to the copied files under dst,
// failing when one matches nothing.
func applyPatches(dst string) error {
	for _, p := range patches {
		found := 0

		replace := func(data []byte) ([]byte, error) {
			found += bytes.Count(data, []byte(p.old))

			return bytes.ReplaceAll(data, []byte(p.old), []byte(p.with)), nil
		}

		target := filepath.Join(dst, filepath.FromSlash(vendoredRel(p.path)))

		info, err := os.Stat(target)
		if err != nil {
			return err
		}

		if info.IsDir() {
			err = transformGoFiles(target, replace)
		} else {
			err = transformFile(target, replace)
		}

		if err != nil {
			return err
		}

		if found == 0 {
			return fmt.Errorf("%s: nothing to patch matches %q", p.path, p.old)
		}
	}

	return nil
}

// rewriteImports points every import of an upstream package in a Go file at
// its vendored copy under dst, failing on an upstream package that is not
// vendored there.
func rewriteImports(dst string, data []byte) ([]byte, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "", data, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	out := data

	for _, spec := range slices.Backward(file.Imports) {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}

		upstreamRel, ok := strings.CutPrefix(importPath, upstreamModule+"/")
		if !ok {
			continue
		}

		rel := vendoredRel(upstreamRel)

		info, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel)))

		switch {
		case errors.Is(err, fs.ErrNotExist), err == nil && !info.IsDir():
			return nil, fmt.Errorf("imports %s, which is not vendored", importPath)
		case err != nil:
			return nil, err
		}

		start := fset.Position(spec.Path.Pos()).Offset
		end := fset.Position(spec.Path.End()).Offset
		out = slices.Concat(out[:start], []byte(strconv.Quote(upstreamPkg+"/"+rel)), out[end:])
	}

	return out, nil
}

// verify tidies the module containing dst, applies go fix and gofmt to the
// vendored code in dst, then builds and tests it.
func verify(ctx context.Context, dst string) error {
	if err := stream(command(ctx, dst, "go", "mod", "tidy")); err != nil {
		return err
	}

	if err := stream(command(ctx, dst, "go", "fix", "./...")); err != nil {
		return err
	}

	if err := transformGoFiles(dst, format.Source); err != nil {
		return err
	}

	if err := stream(command(ctx, dst, "go", "build", "./...")); err != nil {
		return err
	}

	return stream(command(ctx, dst, "go", "test", "./..."))
}

// transformGoFiles replaces the content of every Go file under dir with the
// result of fn.
func transformGoFiles(dir string, fn func([]byte) ([]byte, error)) error {
	var files []string

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(p) == ".go" {
			files = append(files, p)
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := transformFile(file, fn); err != nil {
			return err
		}
	}

	return nil
}

// transformFile replaces the content of the file at name with the result of
// fn.
func transformFile(name string, fn func([]byte) ([]byte, error)) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}

	out, err := fn(data)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	if bytes.Equal(out, data) {
		return nil
	}

	return os.WriteFile(name, out, filePerms)
}

// writeRelease writes the VENDOR.md content doc to name with its Source table
// recording r.
func writeRelease(name string, doc []byte, r release) error {
	doc = replaceValue(doc, tagRow, r.tag)
	doc = replaceValue(doc, commitRow, r.commit)

	return os.WriteFile(name, doc, filePerms)
}

// replaceValue returns a copy of doc with the value row captures replaced by
// value. It panics when row does not match doc; [readRelease] rejects a doc
// without exactly one match for each row before run reaches writeRelease.
func replaceValue(doc []byte, row *regexp.Regexp, value string) []byte {
	loc := row.FindSubmatchIndex(doc)
	if loc == nil {
		panic(fmt.Sprintf("%s does not match the Source table", row))
	}

	return slices.Concat(doc[:loc[2]], []byte(value), doc[loc[3]:])
}

// vendoredRel returns the slash-separated path, relative to the upstream
// directory, that the upstream path p is vendored at.
func vendoredRel(p string) string {
	return strings.TrimPrefix(p, "internal/")
}

// writeFile writes data to name, creating its parent directories.
func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), dirPerms); err != nil {
		return err
	}

	return os.WriteFile(name, data, filePerms)
}

// command returns a command that runs in dir, or the working directory when
// dir is empty, and writes its standard error to the process's.
func command(ctx context.Context, dir, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr

	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	return cmd
}

// stream runs cmd with its standard output sent to standard error.
func stream(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", cmd, err)
	}

	return nil
}

// output runs cmd and returns its standard output without surrounding
// whitespace.
func output(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", cmd, err)
	}

	return strings.TrimSpace(string(out)), nil
}
