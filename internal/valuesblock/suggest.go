package valuesblock

import (
	"fmt"
	"strings"

	"github.com/agext/levenshtein"
)

// maxSuggestionDistance is the largest edit distance at which a name reads as a misspelling of another, matching the
// suggestions HCL and OpenTofu make.
const maxSuggestionDistance = 2

// Hint returns a paragraph, after a blank line, for a reference to name, which d does not declare. It suggests
// the declared name closest to name, or lists the declared names when none is close.
func (d *Declarations) Hint(name string) string {
	names := d.Names()

	if suggestion := NameSuggestion(name, names); suggestion != "" {
		return fmt.Sprintf("\n\nDid you mean %q?", suggestion)
	}

	if len(names) == 0 {
		return "\n\nThe values block declares no values."
	}

	return "\n\nDeclared values: " + strings.Join(names, ", ") + "."
}

// NameSuggestion returns the name in names closest to given, or the empty string when none is within two edits. Of
// names equally close, it returns the first.
func NameSuggestion(given string, names []string) string {
	best := ""
	bestDistance := maxSuggestionDistance + 1

	for _, name := range names {
		distance := levenshtein.Distance(given, name, nil)
		if distance < bestDistance {
			best = name
			bestDistance = distance
		}
	}

	return best
}
