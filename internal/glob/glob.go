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
// "**" does not collapse the separators that flank it. A pattern such as
// "a/**/b" requires at least one intermediate segment and will not match
// "a/b". Use brace alternation — for example "{a/b,a/**/b}" — when you want
// zero-or-more behavior.
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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	gobwas "github.com/gobwas/glob"
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

// Expand returns the absolute paths that match pattern on the local
// filesystem. The pattern uses '/' as the separator on all platforms and '\'
// as the escape character. A pattern that matches nothing returns an empty
// slice and a nil error.
func Expand(pattern string, opts ...ExpandOption) ([]string, error) {
	var o expandOptions
	for _, opt := range opts {
		opt(&o)
	}

	pattern = path.Clean(pattern)

	root, hasMeta := splitRoot(pattern)

	if !hasMeta {
		info, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
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

	walkErr := filepath.WalkDir(root, func(entry string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
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
	if walkErr != nil && !os.IsNotExist(walkErr) {
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
