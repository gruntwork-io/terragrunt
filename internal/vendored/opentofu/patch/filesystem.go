// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2026 Gruntwork, LLC
// SPDX-License-Identifier: MPL-2.0

package patch

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/upstream/lang/funcs"
	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/upstream/lang/marks"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// recursionDepthEnv names the environment variable upstream reads to cap
	// how deep templatefile may call itself.
	recursionDepthEnv = "TF_TEMPLATE_RECURSION_DEPTH"

	// defaultRecursionDepth is the cap upstream applies when
	// [recursionDepthEnv] is unset.
	defaultRecursionDepth = 1024

	// pathParam names the parameter every one of these functions takes the
	// file path in, matching upstream.
	pathParam = "path"
)

// Functions returns the replacements for the upstream functions commented out
// of the vendored function table, each going through the Terragrunt venv or
// logger. funcsCb returns the table templatefile renders a template with,
// which is the table these functions are registered in.
//
// Each of these functions reports every file it reads to onRead, as the path
// the venv's filesystem sees. A path with no file behind it goes unreported,
// including one fileexists finds missing. A template rendered by another
// template reports its own file the same way.
func Functions(
	v *venv.Venv,
	l log.Logger,
	baseDir string,
	funcsCb func() map[string]function.Function,
	onRead func(path string),
) map[string]function.Function {
	return map[string]function.Function{
		"abspath":      AbsPathFunc(v),
		"base64decode": Base64DecodeFunc(l),
		"pathexpand":   PathExpandFunc(v),
		"file":         FileFunc(v, baseDir, false, onRead),
		"filebase64":   FileFunc(v, baseDir, true, onRead),
		"fileexists":   FileExistsFunc(v, baseDir, onRead),
		"fileset":      FileSetFunc(v, baseDir, onRead),
		"filebase64sha256": FileHashFunc(
			v, baseDir, sha256.New, base64.StdEncoding.EncodeToString, onRead,
		),
		"filebase64sha512": FileHashFunc(
			v, baseDir, sha512.New, base64.StdEncoding.EncodeToString, onRead,
		),
		"filemd5":      FileHashFunc(v, baseDir, md5.New, hex.EncodeToString, onRead),
		"filesha1":     FileHashFunc(v, baseDir, sha1.New, hex.EncodeToString, onRead),
		"filesha256":   FileHashFunc(v, baseDir, sha256.New, hex.EncodeToString, onRead),
		"filesha512":   FileHashFunc(v, baseDir, sha512.New, hex.EncodeToString, onRead),
		"templatefile": TemplateFileFunc(v, l, baseDir, funcsCb, onRead),
	}
}

// FileFunc returns a function that reads the file at a path relative to
// baseDir, as a string when encBase64 is false and as base64 when it is true.
func FileFunc(
	v *venv.Venv,
	baseDir string,
	encBase64 bool,
	onRead func(path string),
) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        pathParam,
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		Type:         function.StaticReturnType(cty.String),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			pathArg, pathMarks := args[0].Unmark()
			path := pathArg.AsString()

			src, err := readFileBytes(v, baseDir, path, pathMarks, onRead)
			if err != nil {
				return cty.UnknownVal(cty.String), function.NewArgError(0, err)
			}

			if encBase64 {
				enc := base64.StdEncoding.EncodeToString(src)

				return cty.StringVal(enc).WithMarks(pathMarks), nil
			}

			if !utf8.Valid(src) {
				return cty.UnknownVal(cty.String), fmt.Errorf(
					"contents of %s are not valid UTF-8; use the filebase64 "+
						"function to obtain the Base64 encoded contents or "+
						"the other file functions (e.g. filemd5, filesha256) "+
						"to obtain file hashing results instead",
					redact(path, pathMarks),
				)
			}

			return cty.StringVal(string(src)).WithMarks(pathMarks), nil
		},
	})
}

// FileExistsFunc returns a function that reports whether a regular file exists
// at a path relative to baseDir.
func FileExistsFunc(v *venv.Venv, baseDir string, onRead func(path string)) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        pathParam,
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		Type:         function.StaticReturnType(cty.Bool),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			pathArg, pathMarks := args[0].Unmark()

			path, err := resolvePath(v, baseDir, pathArg.AsString())
			if err != nil {
				return cty.UnknownVal(cty.Bool), err
			}

			fi, err := v.FS.Stat(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return cty.False.WithMarks(pathMarks), nil
				}

				statErr := fmt.Errorf("failed to stat %s", redact(path, pathMarks))

				return cty.UnknownVal(cty.Bool), statErr
			}

			if fi.Mode().IsRegular() {
				onRead(path)

				return cty.True.WithMarks(pathMarks), nil
			}

			return cty.False, irregularFileError(fi.Mode().Type(), redact(path, pathMarks))
		},
	})
}

// FileSetFunc returns a function that enumerates the files under a path
// relative to baseDir whose paths match a glob pattern.
func FileSetFunc(v *venv.Venv, baseDir string, onRead func(path string)) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        pathParam,
				Type:        cty.String,
				AllowMarked: true,
			},
			{
				Name:        "pattern",
				Type:        cty.String,
				AllowMarked: true,
			},
		},
		Type:         function.StaticReturnType(cty.Set(cty.String)),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			pathArg, pathMarks := args[0].Unmark()
			patternArg, patternMarks := args[1].Unmark()
			pattern := patternArg.AsString()
			valueMarks := []cty.ValueMarks{pathMarks, patternMarks}

			path := pathArg.AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}

			matches, err := matchFiles(v.FS, path, pattern)
			if err != nil {
				return cty.UnknownVal(cty.Set(cty.String)), fmt.Errorf(
					"failed to glob pattern %s: %w",
					redact(filepath.Join(path, pattern), valueMarks...),
					err,
				)
			}

			for _, match := range matches {
				onRead(filepath.Join(path, match.AsString()))
			}

			if len(matches) == 0 {
				return cty.SetValEmpty(cty.String).WithMarks(valueMarks...), nil
			}

			return cty.SetVal(matches).WithMarks(valueMarks...), nil
		},
	})
}

// AbsPathFunc returns a function that turns a path into an absolute one,
// resolving a relative path against the working directory the venv reports.
func AbsPathFunc(v *venv.Venv) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: pathParam,
				Type: cty.String,
			},
		},
		Type:         function.StaticReturnType(cty.String),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			path := args[0].AsString()

			if !filepath.IsAbs(path) {
				wd, err := v.Platform.Getwd()
				if err != nil {
					return cty.UnknownVal(cty.String), err
				}

				path = filepath.Join(wd, path)
			}

			return cty.StringVal(filepath.ToSlash(filepath.Clean(path))), nil
		},
	})
}

// PathExpandFunc returns a function that replaces a leading ~ with the home
// directory of the user the venv reports.
func PathExpandFunc(v *venv.Venv) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: pathParam,
				Type: cty.String,
			},
		},
		Type:         function.StaticReturnType(cty.String),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			path, err := expandHome(v, args[0].AsString())
			if err != nil {
				return cty.UnknownVal(cty.String), err
			}

			return cty.StringVal(path), nil
		},
	})
}

// FileHashFunc returns a function that hashes the file at a path relative to
// baseDir with hf and encodes the sum with enc.
func FileHashFunc(
	v *venv.Venv,
	baseDir string,
	hf func() hash.Hash,
	enc func([]byte) string,
	onRead func(path string),
) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name: pathParam,
				Type: cty.String,
			},
		},
		Type:         function.StaticReturnType(cty.String),
		RefineResult: refineNotNull,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			sum, err := hashFile(v, baseDir, args[0].AsString(), hf, onRead)
			if err != nil {
				return cty.UnknownVal(cty.String), err
			}

			return cty.StringVal(enc(sum)), nil
		},
	})
}

// TemplateFileFunc returns a function that renders the file at a path relative
// to baseDir as an HCL template, with the variables given as its second
// argument. funcsCb returns the functions the template may call.
func TemplateFileFunc(
	v *venv.Venv,
	l log.Logger,
	baseDir string,
	funcsCb func() map[string]function.Function,
	onRead func(path string),
) function.Function {
	return templateFileFunc(v, l, baseDir, funcsCb, onRead, 0)
}

// templateFileFunc builds the templatefile function that a template nested
// depth levels deep may call, so that a template including itself stops at the
// recursion cap rather than running until the process dies.
func templateFileFunc(
	v *venv.Venv,
	l log.Logger,
	baseDir string,
	funcsCb func() map[string]function.Function,
	onRead func(path string),
	depth int,
) function.Function {
	loadTmpl := func(path string, valueMarks cty.ValueMarks) (hcl.Expression, error) {
		maxDepth, err := maxRecursionDepth(v)
		if err != nil {
			return nil, err
		}

		if depth > maxDepth {
			l.Debugf(
				"template stack reached depth %d at %s",
				depth,
				redact(path, valueMarks),
			)

			// The sources unwind up the stack as the error is returned.
			return nil, funcs.ErrorTemplateRecursionLimit{}
		}

		src, err := readFileBytes(v, baseDir, path, valueMarks, onRead)
		if err != nil {
			return nil, err
		}

		if !utf8.Valid(src) {
			return nil, fmt.Errorf("contents of %s are not valid UTF-8", redact(path, valueMarks))
		}

		expr, diags := hclsyntax.ParseTemplate(src, path, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, diags
		}

		return expr, nil
	}

	// The nested table is built on each call because the table funcsCb returns
	// is filled in after this function is registered in it.
	nested := func() map[string]function.Function {
		given := funcsCb()
		nested := make(map[string]function.Function, len(given))

		for name, fn := range given {
			if name == "templatefile" {
				fn = templateFileFunc(v, l, baseDir, funcsCb, onRead, depth+1)
			}

			nested[name] = fn
		}

		return nested
	}

	return function.New(&function.Spec{
		Params: []function.Parameter{
			{
				Name:        pathParam,
				Type:        cty.String,
				AllowMarked: true,
			},
			{
				Name: "vars",
				Type: cty.DynamicPseudoType,
			},
		},
		Type: func(args []cty.Value) (cty.Type, error) {
			if !args[0].IsKnown() || !args[1].IsKnown() {
				return cty.DynamicPseudoType, nil
			}

			pathArg, pathMarks := args[0].Unmark()

			expr, err := loadTmpl(pathArg.AsString(), pathMarks)
			if err != nil {
				return cty.DynamicPseudoType, err
			}

			val, err := funcs.RenderTemplate(expr, args[1], nested())

			return val.Type(), err
		},
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			pathArg, pathMarks := args[0].Unmark()

			expr, err := loadTmpl(pathArg.AsString(), pathMarks)
			if err != nil {
				return cty.DynamicVal, err
			}

			result, err := funcs.RenderTemplate(expr, args[1], nested())

			return result.WithMarks(pathMarks), err
		},
	})
}

// maxRecursionDepth returns how deep templatefile may call itself.
func maxRecursionDepth(v *venv.Venv) (int, error) {
	val := v.Env[recursionDepthEnv]
	if val == "" {
		return defaultRecursionDepth, nil
	}

	depth, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %w", recursionDepthEnv, err)
	}

	return depth, nil
}

// hashFile returns the hf sum of the file at a path relative to baseDir.
func hashFile(
	v *venv.Venv,
	baseDir, path string,
	hf func() hash.Hash,
	onRead func(path string),
) (_ []byte, err error) {
	name, err := resolvePath(v, baseDir, path)
	if err != nil {
		return nil, err
	}

	f, err := v.FS.Open(name)
	if err != nil {
		return nil, err
	}

	onRead(name)

	defer func() {
		err = errors.Join(err, f.Close())
	}()

	h := hf()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

// matchFiles returns the regular files under dir whose path relative to dir
// matches pattern, in the slash-separated form the pattern is written in.
func matchFiles(fsys vfs.FS, dir, pattern string) ([]cty.Value, error) {
	if !doublestar.ValidatePattern(pattern) {
		return nil, doublestar.ErrBadPattern
	}

	var matches []cty.Value

	err := vfs.WalkDirWithSymlinks(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)

		ok, err := doublestar.Match(pattern, rel)
		if err != nil {
			return err
		}

		if !ok || !isRegularFile(fsys, path, d) {
			return nil
		}

		matches = append(matches, cty.StringVal(rel))

		return nil
	})
	if err != nil {
		return nil, err
	}

	return matches, nil
}

// isRegularFile reports whether the entry d at path is a regular file, or a
// symbolic link that resolves to one.
func isRegularFile(fsys vfs.FS, path string, d fs.DirEntry) bool {
	if d.Type()&fs.ModeSymlink == 0 {
		return d.Type().IsRegular()
	}

	fi, err := fsys.Stat(path)

	return err == nil && fi.Mode().IsRegular()
}

// readFileBytes returns the contents of the file at a path relative to
// baseDir.
func readFileBytes(
	v *venv.Venv,
	baseDir, path string,
	valueMarks cty.ValueMarks,
	onRead func(path string),
) ([]byte, error) {
	name, err := resolvePath(v, baseDir, path)
	if err != nil {
		return nil, err
	}

	src, err := vfs.ReadFile(v.FS, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf(
				"no file exists at %s; this function works only with files "+
					"that are distributed as part of the configuration source code, "+
					"so if this file will be created by a resource in this configuration "+
					"you must instead obtain this result from an attribute of that resource",
				redact(path, valueMarks),
			)
		}

		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	onRead(name)

	return src, nil
}

// resolvePath returns path as the venv's filesystem sees it: a leading ~ is
// the invoking user's home directory, and a relative path is relative to
// baseDir.
func resolvePath(v *venv.Venv, baseDir, path string) (string, error) {
	path, err := expandHome(v, path)
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	return filepath.Clean(path), nil
}

// expandHome replaces a leading ~ in path with the home directory of the user
// the venv reports, leaving every other path as it is.
func expandHome(v *venv.Venv, path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~")
	if !ok || (rest != "" && rest[0] != '/') {
		return path, nil
	}

	home, err := v.Platform.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to expand ~: %w", err)
	}

	return filepath.Join(home, filepath.FromSlash(rest)), nil
}

// irregularFileError reports what kind of thing sits at the path of a file
// that is not a regular file.
func irregularFileError(fileType fs.FileMode, filename string) error {
	switch {
	case fileType&fs.ModeDir != 0:
		return function.NewArgErrorf(1, "%s is a directory, not a file", filename)
	case fileType&fs.ModeDevice != 0:
		return function.NewArgErrorf(1, "%s is a device node, not a regular file", filename)
	case fileType&fs.ModeNamedPipe != 0:
		return function.NewArgErrorf(1, "%s is a named pipe, not a regular file", filename)
	case fileType&fs.ModeSocket != 0:
		return function.NewArgErrorf(1, "%s is a unix domain socket, not a regular file", filename)
	default:
		return function.NewArgErrorf(1, "%s is not a regular file", filename)
	}
}

// redact returns value as it may be shown in an error, standing in for one
// carrying a sensitive or ephemeral mark.
func redact(value any, valueMarks ...cty.ValueMarks) string {
	marked := cty.DynamicVal.WithMarks(valueMarks...)
	isEphemeral := marks.Has(marked, marks.Ephemeral)
	isSensitive := marks.Has(marked, marks.Sensitive)

	switch {
	case isEphemeral && isSensitive:
		return "(ephemeral sensitive value)"
	case isEphemeral:
		return "(ephemeral value)"
	case isSensitive:
		return "(sensitive value)"
	}

	if s, ok := value.(string); ok {
		return fmt.Sprintf("%q", s)
	}

	return fmt.Sprintf("%v", value)
}

// refineNotNull marks a result as never null, matching upstream's refinement
// on these functions.
func refineNotNull(b *cty.RefinementBuilder) *cty.RefinementBuilder {
	return b.NotNull()
}
