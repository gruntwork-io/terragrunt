package glob

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Pruner reports which directories can hold a match of a set of [Root]s, so a
// walk can skip every directory that cannot. Values are produced by
// [NewPruner] and are safe for concurrent use.
type Pruner struct {
	roots []prunerRoot
}

// prunerRoot is a [Root] with its Dir split into per-segment matchers.
type prunerRoot struct {
	segments  []func(string) bool
	recursive bool
}

// NewPruner compiles roots into a [Pruner]. Each Dir must be '/'-separated and
// relative, with "." naming the top of the walk.
//
// Panics when a Dir segment does not compile. [Roots] never yields a segment
// holding a character class, brace, or escape, so a segment holds only literal
// text, "*", and "?", which always compile.
func NewPruner(roots []Root) *Pruner {
	p := &Pruner{roots: make([]prunerRoot, 0, len(roots))}

	for _, root := range roots {
		compiled := prunerRoot{recursive: root.Recursive}

		if root.Dir != "." {
			for segment := range strings.SplitSeq(root.Dir, "/") {
				compiled.segments = append(compiled.segments, segmentMatcher(segment))
			}
		}

		p.roots = append(p.roots, compiled)
	}

	return p
}

// Admits reports whether a match can sit at or below dir, a '/'-separated
// path relative to the top of the walk, with "." naming the top itself.
//
// A directory is admitted when it matches the leading segments of some root's
// Dir, so a walk still has to pass through it, or when it sits at or below a
// match of a recursive root's Dir.
func (p *Pruner) Admits(dir string) bool {
	var segments []string
	if dir != "." {
		segments = strings.Split(dir, "/")
	}

	for _, root := range p.roots {
		if root.admits(segments) {
			return true
		}
	}

	return false
}

// AdmitsBelow reports whether a match can sit strictly below dir, spelled as
// for [Pruner.Admits]. A directory [Pruner.Admits] accepts and AdmitsBelow
// rejects can hold a match only at dir itself.
func (p *Pruner) AdmitsBelow(dir string) bool {
	var segments []string
	if dir != "." {
		segments = strings.Split(dir, "/")
	}

	for _, root := range p.roots {
		if root.admitsBelow(segments) {
			return true
		}
	}

	return false
}

// admitsBelow reports whether a match of r can sit strictly below the
// directory spelled by segments.
func (r prunerRoot) admitsBelow(segments []string) bool {
	return r.admits(segments) && (r.recursive || len(segments) < len(r.segments))
}

// admits reports whether the directory spelled by segments can hold a match
// of r.
func (r prunerRoot) admits(segments []string) bool {
	if len(segments) > len(r.segments) && !r.recursive {
		return false
	}

	n := min(len(segments), len(r.segments))

	return slices.EqualFunc(segments[:n], r.segments[:n], func(name string, match func(string) bool) bool {
		return match(name)
	})
}

// segmentMatcher returns a match for one segment of a [Root]'s Dir. A literal
// segment matches its own spelling only.
//
// A wildcard segment also accepts every name that is not plain ASCII.
// gobwas/glob matches multibyte runes differently depending on the rest of the
// pattern, so "?0" misses "ߣ0" while "{?,}0" matches it. Admitting more costs a
// walk a directory and never a match.
//
// Panics when segment does not compile, as [NewPruner] documents.
func segmentMatcher(segment string) func(string) bool {
	if !strings.ContainsAny(segment, "*?") {
		return func(name string) bool { return name == segment }
	}

	matcher, err := Compile(segment)
	if err != nil {
		panic(fmt.Sprintf("glob: root segment %q does not compile: %v", segment, err))
	}

	return func(name string) bool {
		return !isASCII(name) || matcher.Match(name)
	}
}

// isASCII reports whether s holds only ASCII characters.
func isASCII(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r > unicode.MaxASCII })
}
