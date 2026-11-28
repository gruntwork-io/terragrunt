package glob_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlternatives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "no braces",
			pattern: "a/*/b",
			want:    []string{"a/*/b"},
		},
		{
			name:    "one group",
			pattern: "{a,b}/c",
			want:    []string{"a/c", "b/c"},
		},
		{
			name:    "option holding a separator",
			pattern: "a/{b,c/d}/**",
			want:    []string{"a/b/**", "a/c/d/**"},
		},
		{
			name:    "nested group",
			pattern: "{a,{b,c}}/x",
			want:    []string{"a/x", "b/x", "c/x"},
		},
		{
			name:    "two groups",
			pattern: "{a,b}/{c,d}",
			want:    []string{"a/c", "a/d", "b/c", "b/d"},
		},
		{
			name:    "empty option",
			pattern: "a{,b}",
			want:    []string{"a", "ab"},
		},
		{
			name:    "escaped braces are literal",
			pattern: `\{a,b\}`,
			want:    []string{`\{a,b\}`},
		},
		{
			name:    "braces inside a class are literal",
			pattern: "[{]a",
			want:    []string{"[{]a"},
		},
		{
			name:    "stray closing brace is literal",
			pattern: "a}/b",
			want:    []string{"a}/b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := glob.Alternatives(tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAlternativesRejectsUnbalancedBraces(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"{a,b", "a/[bc"} {
		_, err := glob.Alternatives(pattern)
		require.ErrorIs(t, err, glob.ErrUnbalancedBraces, pattern)
	}
}

func TestAlternativesCapsExpansion(t *testing.T) {
	t.Parallel()

	wide := strings.Repeat("{a,b}", 9)

	_, err := glob.Alternatives(wide)
	require.ErrorIs(t, err, glob.ErrTooManyAlternatives)

	deep := strings.Repeat("{a,", glob.MaxBraceDepth+2) + strings.Repeat("}", glob.MaxBraceDepth+2)

	_, err = glob.Alternatives(deep)
	require.ErrorIs(t, err, glob.ErrTooManyAlternatives)
}

func TestRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    []glob.Root
	}{
		{
			name:    "literal path",
			pattern: "path/to/unit",
			want:    []glob.Root{{Dir: "path/to/unit"}},
		},
		{
			name:    "single-segment wildcard",
			pattern: "path/to/*",
			want:    []glob.Root{{Dir: "path/to/*"}},
		},
		{
			name:    "globstar after a literal prefix",
			pattern: "path/to/**",
			want:    []glob.Root{{Dir: "path/to", Recursive: true}},
		},
		{
			name:    "wildcard before a globstar",
			pattern: "a/*/b/**",
			want:    []glob.Root{{Dir: "a/*/b", Recursive: true}},
		},
		{
			name:    "globstar inside a segment",
			pattern: "a/b**/c",
			want:    []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "leading globstar",
			pattern: "**/unit",
			want:    []glob.Root{{Dir: ".", Recursive: true}},
		},
		{
			name:    "absolute leading globstar",
			pattern: "/**/unit",
			want:    []glob.Root{{Dir: "/", Recursive: true}},
		},
		{
			name:    "brace alternation yields a root per option",
			pattern: "{a,b}/**",
			want: []glob.Root{
				{Dir: "a", Recursive: true},
				{Dir: "b", Recursive: true},
			},
		},
		{
			name:    "duplicate roots collapse",
			pattern: "a/{x,y}**",
			want:    []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "character class walks from before it",
			pattern: "a/[!x]/b",
			want:    []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "escape walks from before it",
			pattern: `a/\*/b`,
			want:    []glob.Root{{Dir: "a", Recursive: true}},
		},
		{
			name:    "unbalanced brace walks from the literal prefix",
			pattern: "a/b/{c,d",
			want:    []glob.Root{{Dir: "a/b", Recursive: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, glob.Roots(tt.pattern))
		})
	}
}

func TestRootsCoverEveryMatch(t *testing.T) {
	t.Parallel()

	patterns := []string{
		"a/b", "a/*", "a/**", "a/**/b", "*/b", "a/*/b/**", "{a,b}/c", "{a,{b,c/d}}/**",
		"a/b**", "a?/b", "a/[!x]/b", "a/[a-z]", `a/\*`, "{,a}/x", "a/{b,**}/c", "**", "{a,b",
		"/a/**", "/**",
	}
	paths := []string{
		"a", "a/b", "a/c", "a/b/c", "a/x/b", "b", "b/c", "c/d/x", "c/d", "ab/b", "a/bc/d",
		"a/*", "/x", "a/x", "a/x/y/c", "/a/b", "{a,b",
	}

	for _, pattern := range patterns {
		matcher, err := glob.Compile(pattern)
		require.NoError(t, err, pattern)

		roots := glob.Roots(pattern)

		for _, p := range paths {
			if matcher.Match(p) {
				assert.Truef(t, covered(t, roots, p), "%q matches %q but no root in %v covers it", pattern, p, roots)
			}
		}
	}
}

func FuzzRoots(f *testing.F) {
	f.Add("a/{b,c}/**", "a/c/d")
	f.Add("{a,{b,c}}/x", "c/x")
	f.Add("a/[!x]b", "a//b")
	f.Add("/**/x", "/y/x")

	f.Fuzz(func(t *testing.T, pattern, candidate string) {
		if !gobwasHandles(pattern, candidate) || !oracleMatch(pattern, candidate) {
			return
		}

		roots := glob.Roots(pattern)
		require.Truef(t, covered(t, roots, candidate), "%q matches %q but no root in %v covers it", pattern, candidate, roots)
	})
}

// gobwasHandles reports whether gobwas/glob can serve as an oracle for
// matching pattern against candidate. It matches NUL bytes and multibyte runes
// differently with and without braces, so "?0" misses "ߣ0" while "{?,}0"
// matches it, and it backtracks exponentially on long runs of "*" inside
// unclosed braces.
func gobwasHandles(pattern, candidate string) bool {
	return printableASCII(pattern+candidate) && len(pattern) <= 24 && len(candidate) <= 64
}

// printableASCII reports whether s holds only printable ASCII characters.
func printableASCII(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < ' ' || r > '~' })
}

// oracleMatch reports whether pattern compiles and matches candidate.
// gobwas/glob compiles some malformed patterns, such as an unclosed "{" or an
// empty "{}", and then panics in Match, so a panic counts as no match.
func oracleMatch(pattern, candidate string) (matched bool) {
	defer func() {
		if recover() != nil {
			matched = false
		}
	}()

	matcher, err := glob.Compile(pattern)

	return err == nil && matcher.Match(candidate)
}

// covered reports whether one of roots covers p: a non-recursive root whose
// Dir matches p, or a recursive root whose Dir matches p or one of its
// ancestors. A recursive "." covers every path and a recursive "/" every
// absolute one.
func covered(t *testing.T, roots []glob.Root, p string) bool {
	t.Helper()

	for _, root := range roots {
		if !root.Recursive {
			matcher, err := glob.Compile(root.Dir)
			require.NoError(t, err, root.Dir)

			if matcher.Match(p) {
				return true
			}

			continue
		}

		if root.Dir == "." {
			return true
		}

		if root.Dir == "/" && strings.HasPrefix(p, "/") {
			return true
		}

		matcher, err := glob.Compile(root.Dir)
		require.NoError(t, err, root.Dir)

		if matcher.Match(p) {
			return true
		}

		for i := range len(p) {
			if p[i] == '/' && matcher.Match(p[:i]) {
				return true
			}
		}
	}

	return false
}
