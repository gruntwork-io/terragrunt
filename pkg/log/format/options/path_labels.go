package options

import (
	"path/filepath"
	"slices"
)

// PathLabels returns the label the `minimal` path format shows for each of
// the given unit paths, keyed by the unit's absolute path: the last element of
// the path, with as many parent directories as it takes to tell the unit apart
// from the others. Relative paths are resolved against baseDir.
func PathLabels(baseDir string, paths []string) map[string]string {
	// suffixes holds each unit's trailing parts, shortest first.
	suffixes := make(map[string][]string, len(paths))

	for _, path := range paths {
		path = absPath(baseDir, path)
		suffixes[path] = trailingParts(relOrSelf(baseDir, path))
	}

	// counts holds how many units end in each trailing part.
	counts := make(map[string]int)

	for _, parts := range suffixes {
		for _, part := range parts {
			counts[part]++
		}
	}

	labels := make(map[string]string, len(suffixes))

	for path, parts := range suffixes {
		if i := slices.IndexFunc(parts, func(part string) bool { return counts[part] == 1 }); i >= 0 {
			labels[path] = parts[i]

			continue
		}

		// The unit's path ends another unit's path, so no part of it is unique.
		labels[path] = parts[len(parts)-1]
	}

	return labels
}

// absPath returns path cleaned, resolved against baseDir when it's relative.
func absPath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	return filepath.Join(baseDir, path)
}

// trailingParts returns the trailing parts of path, shortest first.
func trailingParts(path string) []string {
	var parts []string

	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == filepath.Separator && i+1 < len(path) {
			parts = append(parts, path[i+1:])
		}
	}

	return append(parts, path)
}

// relOrSelf returns path relative to baseDir, or path itself when it's outside baseDir.
func relOrSelf(baseDir, path string) string {
	if rel, err := filepath.Rel(baseDir, path); err == nil && filepath.IsLocal(rel) {
		return rel
	}

	return path
}
