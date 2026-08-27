package runner

import "bytes"

// SubstringScanner reports whether every one of its needles has appeared in the
// stream written to it.
//
// The longest needle bounds what it retains, not the length of the stream, so
// watching a unit's stderr for the whole of a `run --all` costs a fixed amount of
// memory. A scanner with no needles reports [SubstringScanner.FoundAll] straight
// away. It is not safe for concurrent use.
type SubstringScanner struct {
	needles [][]byte
	found   []bool
	tail    []byte
	seam    []byte
	overlap int
}

// NewSubstringScanner returns a scanner watching for every needle in needles.
// Empty needles are dropped, since every stream trivially contains them.
func NewSubstringScanner(needles ...string) *SubstringScanner {
	scanner := &SubstringScanner{}

	for _, needle := range needles {
		if needle == "" {
			continue
		}

		scanner.needles = append(scanner.needles, []byte(needle))

		if overlap := len(needle) - 1; overlap > scanner.overlap {
			scanner.overlap = overlap
		}
	}

	scanner.found = make([]bool, len(scanner.needles))

	return scanner
}

// Write implements [io.Writer] and never reports an error.
func (scanner *SubstringScanner) Write(p []byte) (int, error) {
	if scanner.FoundAll() {
		return len(p), nil
	}

	// A needle can straddle two writes, so search the seam between the retained
	// tail and the head of p before searching p itself. The longest needle bounds
	// both regions, which is what keeps memory independent of how much a unit
	// writes.
	if len(scanner.tail) > 0 {
		head := p[:min(len(p), scanner.overlap)]
		scanner.seam = append(append(scanner.seam[:0], scanner.tail...), head...)

		scanner.search(scanner.seam)
	}

	scanner.search(p)
	scanner.retain(p)

	return len(p), nil
}

// FoundAll reports whether every needle has been seen.
func (scanner *SubstringScanner) FoundAll() bool {
	for _, found := range scanner.found {
		if !found {
			return false
		}
	}

	return true
}

func (scanner *SubstringScanner) search(b []byte) {
	for i, needle := range scanner.needles {
		if !scanner.found[i] && bytes.Contains(b, needle) {
			scanner.found[i] = true
		}
	}
}

// retain keeps the trailing bytes a later write might need to complete a needle.
// Writes shorter than a needle accumulate, so a needle arriving a byte at a time
// still matches.
func (scanner *SubstringScanner) retain(p []byte) {
	if len(p) >= scanner.overlap {
		scanner.tail = append(scanner.tail[:0], p[len(p)-scanner.overlap:]...)

		return
	}

	scanner.tail = append(scanner.tail, p...)

	if len(scanner.tail) > scanner.overlap {
		scanner.tail = append(scanner.tail[:0], scanner.tail[len(scanner.tail)-scanner.overlap:]...)
	}
}
