package git

import (
	"bufio"
	"bytes"
	"strings"
)

// Diffs represents the diffs between two Git references.
type Diffs struct {
	Added   []string
	Removed []string
	Changed []string
}

// ParseDiff parses the stdout of a `git diff --name-status --no-renames` into a Diffs object.
func ParseDiff(output []byte) (*Diffs, error) {
	maxCount := bytes.Count(output, []byte("\n")) + 1

	added := make([]string, 0, maxCount)
	removed := make([]string, 0, maxCount)
	changed := make([]string, 0, maxCount)

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		status, path, found := strings.Cut(line, "\t")
		if !found || path == "" {
			return nil, &WrappedError{
				Op:      "parse_diff",
				Context: "invalid diff line",
				Err:     ErrParseDiff,
			}
		}

		switch status {
		case "A":
			added = append(added, path)
		case "D":
			removed = append(removed, path)
		case "M":
			changed = append(changed, path)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, &WrappedError{
			Op:      "parse_diff",
			Context: "failed to read diff output",
			Err:     err,
		}
	}

	return &Diffs{
		Added:   added,
		Removed: removed,
		Changed: changed,
	}, nil
}
