// Package glob consolidates Terragrunt's glob handling behind a single API so
// callers pick a function that matches their use case rather than picking a
// library.
//
// # Grammar
//
// New code uses [gobwas/glob] semantics. Patterns are '/'-separated, '*'
// matches within a single segment, '**' matches any sequence of characters
// including separators, '?' matches any single non-separator character,
// '[...]' matches a character class, and '{a,b}' matches any of the listed
// alternatives. A backslash escapes the following metacharacter.
//
// "**" collapses the flanking separators only when the adjacent segments are
// literals. With literals on both sides, "a/**/b" matches "a/b" as well as
// "a/x/b". If either neighbor is a wildcard (for example "a/**/*.tf" or
// "*/**/b.tf"), "**" does not collapse and a zero-depth match fails. Use
// brace alternation — for example "{*.tf,**/*.tf}" — to cover both depths
// when the trailing segment contains a wildcard.
//
// # When to use what
//
// Use [Compile] for compile-once-match-many: you hold a single pattern and
// test it against many strings, for example filter expressions evaluated
// against every unit in a discovery pass. [Compile] builds a matcher once and
// matches in constant time.
//
// Use [Expand] for compile-then-walk: you hold a single pattern and want to
// enumerate the matching paths on disk, for example mark_glob_as_read
// resolving "locals/**/*.yaml" at config evaluation. Pass [WithFilesOnly] to
// skip directories.
//
// Use [LegacyExpand] only for call sites that participate in user-facing
// configuration surface (for example, include_in_copy and exclude_from_copy
// expansion). It is backed by [zglob] and retained to avoid a silent behavior
// change for patterns users have written against zglob for years. Prefer
// [Expand] for new code.
//
// [gobwas/glob]: https://pkg.go.dev/github.com/gobwas/glob
// [zglob]: https://pkg.go.dev/github.com/mattn/go-zglob
package glob

import (
	"errors"
	iofs "io/fs"
	"path"
	"path/filepath"
	"strings"

	gobwas "github.com/gobwas/glob"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/mattn/go-zglob"
)

// Matcher tests whether a string matches a compiled glob pattern. Values are
// produced by [Compile] and are safe for concurrent use.
type Matcher interface {
	Match(s string) bool
}

// Compile parses pattern as a '/'-separated glob and returns a [Matcher].
// Intended for testing one pattern against many strings.
func Compile(pattern string) (Matcher, error) {
	return gobwas.Compile(pattern, '/')
}

// ExpandOption configures the behavior of [Expand]. See [WithFilesOnly].
type ExpandOption func(*expandOptions)

type expandOptions struct {
	filesOnly bool
}

// WithFilesOnly causes [Expand] to skip directories, returning only matching
// files.
func WithFilesOnly() ExpandOption {
	return func(o *expandOptions) {
		o.filesOnly = true
	}
}

// Expand returns the absolute paths that match pattern on fs. The pattern
// uses '/' as the separator on all platforms and '\' as the escape character.
// A pattern that matches nothing returns an empty slice and a nil error.
//
// Most callers pass [vfs.NewOSFS] for fs; tests can pass an in-memory
// filesystem from [vfs.NewMemMapFS].
func Expand(fs vfs.FS, pattern string, opts ...ExpandOption) ([]string, error) {
	var o expandOptions
	for _, opt := range opts {
		opt(&o)
	}

	pattern = path.Clean(pattern)

	root, hasMeta := splitRoot(pattern)

	if !hasMeta {
		info, err := fs.Stat(root)
		if err != nil {
			if errors.Is(err, iofs.ErrNotExist) {
				return nil, nil
			}

			return nil, err
		}

		if o.filesOnly && info.IsDir() {
			return nil, nil
		}

		return []string{root}, nil
	}

	matcher, err := Compile(pattern)
	if err != nil {
		return nil, err
	}

	var matches []string

	walkErr := vfs.WalkDir(fs, root, func(entry string, d iofs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return iofs.SkipDir
			}

			return nil
		}

		if o.filesOnly && d.IsDir() {
			return nil
		}

		if !matcher.Match(filepath.ToSlash(entry)) {
			return nil
		}

		matches = append(matches, entry)

		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, iofs.ErrNotExist) {
		return nil, walkErr
	}

	return matches, nil
}

// LegacyExpand returns the paths that match pattern using zglob semantics.
// Prefer [Expand] for new code. LegacyExpand exists only for call sites that
// interpret patterns written by users in configuration surface where a
// behavior change between zglob and gobwas would be a breaking change.
func LegacyExpand(pattern string) ([]string, error) {
	return zglob.Glob(pattern)
}

// splitRoot returns the longest leading directory of pattern that contains no
// glob metacharacters, ready to hand to filepath.WalkDir, and reports whether
// any metacharacters were found. pattern must use '/' as the separator.
func splitRoot(pattern string) (string, bool) {
	metaIdx := strings.IndexAny(pattern, "*?[{\\")
	if metaIdx < 0 {
		return filepath.FromSlash(pattern), false
	}

	prefix := pattern[:metaIdx]

	if i := strings.LastIndex(prefix, "/"); i >= 0 {
		prefix = prefix[:i]
	} else {
		prefix = "."
	}

	if prefix == "" {
		prefix = "/"
	}

	return filepath.FromSlash(prefix), true
}
