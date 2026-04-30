// Package ignore implements .terragrunt-catalog-ignore parsing and matching
// for catalog discovery. The file lives at the root of a catalog repo and
// lets authors exclude directories (e.g. examples/, test/) from module and
// template discovery using simple globs with .gitignore-style negation.
package ignore

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	tgerrors "github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/glob"
)

// FileName is the fixed name looked up at the repo root.
const FileName = ".terragrunt-catalog-ignore"

type rule struct {
	glob   glob.Matcher
	raw    string
	negate bool
}

// Matcher evaluates repo-relative, forward-slash paths against a list of
// ordered ignore rules. The last matching rule wins, matching .gitignore
// semantics.
type Matcher struct {
	rules []rule
}

// Load reads <repoPath>/.terragrunt-catalog-ignore. A missing file is not an
// error: an empty Matcher is returned. Callers pointing at an explicit,
// caller-validated path should use LoadFile instead.
func Load(repoPath string) (*Matcher, error) {
	f, err := os.Open(filepath.Join(repoPath, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Matcher{}, nil
		}

		return nil, tgerrors.New(err)
	}

	return parseAndClose(f)
}

// LoadFile reads an ignore file at the given path. The caller is responsible
// for validating that the path is non-empty and exists; this function simply
// reads and parses.
func LoadFile(path string) (*Matcher, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, tgerrors.New(err)
	}

	return parseAndClose(f)
}

// Merge appends other's rules onto m. The rules from other take precedence
// under the "last match wins" rule.
func (m *Matcher) Merge(other *Matcher) {
	if m == nil || other == nil {
		return
	}

	m.rules = append(m.rules, other.rules...)
}

func parseAndClose(f *os.File) (m *Matcher, err error) {
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = tgerrors.New(cerr)
		}
	}()

	return Parse(f)
}

// Parse consumes an ignore file from r and returns a compiled Matcher.
func Parse(r io.Reader) (*Matcher, error) {
	m := &Matcher{}

	scanner := bufio.NewScanner(r)

	lineNo := 0
	for scanner.Scan() {
		lineNo++

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}

		line = strings.TrimSuffix(line, "/")
		if line == "" {
			continue
		}

		g, err := glob.Compile(line)
		if err != nil {
			return nil, tgerrors.Errorf("%s line %d: invalid pattern %q: %w", FileName, lineNo, line, err)
		}

		m.rules = append(m.rules, rule{glob: g, negate: negate, raw: line})
	}

	if err := scanner.Err(); err != nil {
		return nil, tgerrors.New(err)
	}

	return m, nil
}

// Match reports whether relPath (repo-relative, forward-slash, never leading
// "/") is ignored. The repo root (empty string) is never ignored.
func (m *Matcher) Match(relPath string) bool {
	if m == nil || relPath == "" {
		return false
	}

	ignored := false

	for _, r := range m.rules {
		if !r.glob.Match(relPath) {
			continue
		}

		ignored = !r.negate
	}

	return ignored
}

// Empty reports whether the matcher has no rules.
func (m *Matcher) Empty() bool {
	return m == nil || len(m.rules) == 0
}
