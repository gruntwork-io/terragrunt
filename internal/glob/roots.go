package glob

import (
	"errors"
	"slices"
	"strings"
)

const (
	// MaxAlternatives is the most patterns [Alternatives] expands one pattern
	// into before it returns [ErrTooManyAlternatives].
	MaxAlternatives = 256

	// MaxBraceDepth is the deepest brace nesting [Alternatives] expands before
	// it returns [ErrTooManyAlternatives].
	MaxBraceDepth = 16
)

var (
	// ErrTooManyAlternatives reports that a pattern expands into more than
	// [MaxAlternatives] patterns or nests braces deeper than [MaxBraceDepth].
	ErrTooManyAlternatives = errors.New("glob pattern expands into too many alternatives")

	// ErrUnbalancedBraces reports a "{" with no matching "}", or a "[" with no
	// matching "]".
	ErrUnbalancedBraces = errors.New("glob pattern has unbalanced brackets")
)

// Root is a directory that every match of a pattern sits at or below.
type Root struct {
	// Dir is a '/'-separated pattern naming the directories. Its segments hold
	// no "**", braces, character classes, or escapes, so each segment matches
	// within one directory level and a literal segment names one entry.
	Dir string
	// Recursive reports whether matches can sit anywhere below Dir. When it is
	// false, every match is a path Dir itself matches.
	Recursive bool
}

// Roots returns the directories that every match of pattern sits at or below,
// one per brace alternative, without duplicates. pattern must use '/' as the
// separator.
//
// Some patterns cannot be split exactly: an unclosed brace group, braces that
// expand past the limits of [Alternatives], braces in a pattern that also
// holds a "?", and a character class or escape before the first "**". Each
// yields a recursive root at the longest literal directory before the
// construct, which still covers every match.
func Roots(pattern string) []Root {
	if mixesBracesAndQuestionMarks(pattern) {
		return []Root{{Dir: literalDir(pattern), Recursive: true}}
	}

	alternatives, err := Alternatives(pattern)
	if err != nil {
		return []Root{{Dir: literalDir(pattern), Recursive: true}}
	}

	roots := make([]Root, 0, len(alternatives))

	for _, alternative := range alternatives {
		root := rootOf(alternative)
		if !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}

	return roots
}

// Alternatives expands the brace groups in pattern into the patterns they
// stand for, so "{a,b}/c" yields "a/c" and "b/c". Nested groups expand too.
// Escaped braces and braces inside a character class are literal.
//
// Returns [ErrUnbalancedBraces] when a group or class does not close, and
// [ErrTooManyAlternatives] when the expansion exceeds [MaxAlternatives]
// patterns or [MaxBraceDepth] levels of nesting.
func Alternatives(pattern string) ([]string, error) {
	return expandBraces(pattern, 0)
}

// braceGroup is the first top-level brace group of a pattern, split around
// its options.
type braceGroup struct {
	prefix  string
	suffix  string
	options []string
}

// expandBraces returns the patterns pattern stands for once its first brace
// group is expanded, recursing into each for the groups after it.
func expandBraces(pattern string, depth int) ([]string, error) {
	if depth > MaxBraceDepth {
		return nil, ErrTooManyAlternatives
	}

	group, found, err := firstBraceGroup(pattern)
	if err != nil {
		return nil, err
	}

	if !found {
		return []string{pattern}, nil
	}

	var out []string

	for _, option := range group.options {
		expanded, err := expandBraces(group.prefix+option+group.suffix, depth+1)
		if err != nil {
			return nil, err
		}

		out = append(out, expanded...)
		if len(out) > MaxAlternatives {
			return nil, ErrTooManyAlternatives
		}
	}

	return out, nil
}

// firstBraceGroup splits pattern around its first top-level brace group. It
// reports false when pattern has no brace group.
func firstBraceGroup(pattern string) (braceGroup, bool, error) {
	var group braceGroup

	depth := 0
	optionStart := 0

	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				return braceGroup{}, false, ErrUnbalancedBraces
			}

			i += end + 1
		case '{':
			depth++
			if depth == 1 {
				group.prefix = pattern[:i]
				optionStart = i + 1
			}
		case ',':
			if depth == 1 {
				group.options = append(group.options, pattern[optionStart:i])
				optionStart = i + 1
			}
		case '}':
			if depth == 0 {
				continue
			}

			depth--
			if depth == 0 {
				group.options = append(group.options, pattern[optionStart:i])
				group.suffix = pattern[i+1:]

				return group, true, nil
			}
		}
	}

	if depth > 0 {
		return braceGroup{}, false, ErrUnbalancedBraces
	}

	return braceGroup{}, false, nil
}

// rootOf returns the root of a pattern with no brace groups.
func rootOf(pattern string) Root {
	offset := 0

	for segment := range strings.SplitSeq(pattern, "/") {
		if spansDirectories(segment) {
			return Root{Dir: leadingDir(pattern, max(offset-1, 0)), Recursive: true}
		}

		offset += len(segment) + 1
	}

	return Root{Dir: pattern}
}

// mixesBracesAndQuestionMarks reports whether pattern holds both a brace and a
// "?". gobwas/glob miscounts "?" inside brace alternation, so "{?,00}0" matches
// "0", and such a pattern's matches cannot be predicted from its alternatives.
func mixesBracesAndQuestionMarks(pattern string) bool {
	return strings.Contains(pattern, "{") && strings.Contains(pattern, "?")
}

// spansDirectories reports whether segment can match across a '/' or cannot be
// matched one directory at a time: it holds "**", a character class, which
// matches '/' when negated as in "[!x]", or an escape.
func spansDirectories(segment string) bool {
	return strings.Contains(segment, "**") || strings.ContainsAny(segment, `[\`)
}

// literalDir returns the longest leading directory of pattern that holds no
// metacharacters, in '/'-separated form.
func literalDir(pattern string) string {
	metaIdx := strings.IndexAny(pattern, `*?[{\`)
	if metaIdx < 0 {
		return pattern
	}

	cut := strings.LastIndexByte(pattern[:metaIdx], '/')
	if cut < 0 {
		return "."
	}

	return leadingDir(pattern, cut)
}

// leadingDir returns pattern[:n] as a directory, spelling an empty prefix as
// "/" for an absolute pattern and "." otherwise.
func leadingDir(pattern string, n int) string {
	if n > 0 {
		return pattern[:n]
	}

	if strings.HasPrefix(pattern, "/") {
		return "/"
	}

	return "."
}
