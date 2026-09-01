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
// brace alternation like "{*.tf,**/*.tf}" to cover both depths when the
// trailing segment contains a wildcard.
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
	"fmt"
	"io/fs"
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

// ErrOutsideBoundary reports that a pattern's walk root fell outside the
// boundary supplied to [WithBoundary].
var ErrOutsideBoundary = errors.New("glob pattern resolves outside the configured boundary")

// ExpandOption configures the behavior of [Expand]. See [WithFilesOnly] and
// [WithBoundary].
type ExpandOption func(*expandOptions)

type expandOptions struct {
	boundary  string
	filesOnly bool
}

// WithFilesOnly causes [Expand] to skip directories, returning only matching
// files.
func WithFilesOnly() ExpandOption {
	return func(o *expandOptions) {
		o.filesOnly = true
	}
}

// WithBoundary constrains the directory [Expand] is allowed to walk. boundary
// must be an absolute path. [Expand] returns [ErrOutsideBoundary] when the
// pattern's walk root is not boundary or a descendant of it. An empty boundary
// imposes no constraint.
func WithBoundary(boundary string) ExpandOption {
	return func(o *expandOptions) {
		o.boundary = boundary
	}
}

// Expand returns the absolute paths that match pattern on fsys. The pattern
// uses '/' as the separator on all platforms and '\' as the escape character.
// A pattern that matches nothing returns an empty slice and a nil error.
//
// Pass [WithBoundary] to constrain the walk to a directory; a pattern whose
// walk root falls outside it returns [ErrOutsideBoundary].
//
// Most callers pass [vfs.NewOSFS] for fsys; tests can pass an in-memory
// filesystem from [vfs.NewMemMapFS].
func Expand(fsys vfs.FS, pattern string, opts ...ExpandOption) ([]string, error) {
	var o expandOptions
	for _, opt := range opts {
		opt(&o)
	}

	pattern = path.Clean(pattern)

	root, hasMeta := splitRoot(pattern)

	if err := o.checkBoundary(fsys, root); err != nil {
		return nil, err
	}

	if !hasMeta {
		info, err := fsys.Stat(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
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

	walkErr := vfs.WalkDir(fsys, root, func(entry string, d fs.DirEntry, walkErr error) error {
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
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, walkErr
	}

	return matches, nil
}

// LegacyExpand returns the paths on fsys that match pattern using zglob
// semantics. Prefer [Expand] for new code. LegacyExpand exists only for call
// sites that interpret patterns written by users in configuration surface
// where a behavior change between zglob and gobwas would be a breaking change.
//
// zglob offers no way to walk anything but the real filesystem, so the walk is
// reproduced here over fsys. Deciding whether a path matches is still zglob's
// own matcher, built from the pattern by [zglob.New], which keeps the grammar
// identical; fsys supplies only the directory entries and the resolution of a
// symlinked walk root.
// TestLegacyExpandMatchesZglob pins the two against each other over a corpus
// of patterns.
func LegacyExpand(fsys vfs.FS, pattern string) ([]string, error) {
	root, hasMeta := legacyRoot(pattern)

	// A pattern with no metacharacters names one path, and zglob reports a
	// missing one as fs.ErrNotExist rather than as an empty result. Callers
	// distinguish the two, so the distinction is preserved.
	if !hasMeta {
		if _, err := fsys.Stat(pattern); err != nil {
			return nil, fs.ErrNotExist
		}

		return []string{pattern}, nil
	}

	matcher, err := zglob.New(pattern)
	if err != nil {
		return nil, err
	}

	// zglob stats its walk root through symlinks, so a pattern rooted at a
	// symlinked directory expands through the link. Walking the link target
	// while reporting entries under the root's own spelling keeps that
	// behavior (issue #6791). A failed Lstat is deliberately left to the walk
	// below, which probes the same root and surfaces the same error.
	walkRoot := root

	if info, lstatErr := vfs.Lstat(fsys, root); lstatErr == nil && info.Mode()&fs.ModeSymlink != 0 {
		walkRoot, err = vfs.EvalSymlinks(fsys, root)
		if err != nil {
			return nil, err
		}
	}

	matches := []string{}

	// zglob surfaces a walk failure rather than treating it as an empty match,
	// including the common case of a pattern rooted at a directory that does
	// not exist, so the error is passed straight back here too.
	walkErr := vfs.WalkDir(fsys, walkRoot, func(entry string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if walkRoot != root {
			rel, relErr := filepath.Rel(walkRoot, entry)
			if relErr != nil {
				return relErr
			}

			entry = filepath.Join(root, rel)
		}

		if matcher.Match(filepath.ToSlash(entry)) {
			matches = append(matches, entry)
		}

		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return matches, nil
}

// legacyRoot returns the deepest directory of pattern that zglob would start
// its walk from, and reports whether pattern has anything to expand. It
// reproduces zglob's rule, which treats only "*" and "{" as the markers that
// end the literal prefix.
//
// zglob also expands a leading "~" and any whole segment of the form "$NAME"
// from the process environment before choosing its root. That is not
// reproduced here, so such a pattern roots its walk at a literal "$NAME"
// directory and reports it missing, which the caller in internal/util already
// reads as no matches. The matcher zglob builds still reads the environment,
// so the walk root, not the grammar, is what keeps the expansion out.
func legacyRoot(pattern string) (string, bool) {
	var (
		globmask string
		root     string
		found    bool
	)

	for segment := range strings.SplitSeq(filepath.ToSlash(pattern), "/") {
		if !found && strings.ContainsAny(segment, "*{") {
			found = true

			root = globmask
			if root == "" {
				root = "."
			}
		}

		globmask = path.Join(globmask, segment)

		if globmask == "" {
			globmask = "/"
		}
	}

	if !found {
		return "", false
	}

	return filepath.Clean(root), true
}

// splitRoot returns the longest leading directory of pattern that contains no
// glob metacharacters, ready to hand to filepath.WalkDir, and reports whether
// any metacharacters were found. pattern must use '/' as the separator.
func splitRoot(pattern string) (string, bool) {
	metaIdx := strings.IndexAny(pattern, "*?[{\\")
	if metaIdx < 0 {
		return filepath.FromSlash(pattern), false
	}

	prefix, _, ok := strings.CutLast(pattern[:metaIdx], "/")
	if !ok {
		prefix = "."
	}

	if prefix == "" {
		prefix = "/"
	}

	return filepath.FromSlash(prefix), true
}

// checkBoundary reports whether walking from root is permitted. An empty
// boundary imposes no constraint; otherwise root must fall inside it.
func (o expandOptions) checkBoundary(fsys vfs.FS, root string) error {
	if o.boundary == "" {
		return nil
	}

	if !vfs.Within(fsys, o.boundary, root) {
		return fmt.Errorf("%w: %q is outside %q", ErrOutsideBoundary, root, o.boundary)
	}

	return nil
}
