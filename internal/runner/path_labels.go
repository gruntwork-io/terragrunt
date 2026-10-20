package runner

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
)

// UnitPathLabels returns the short labels shown by the `basename` path format of `%prefix`
// for the given unit paths, keyed by the cleaned path.
//
// A label starts with the last element of the path, then gets parent directories prepended,
// skipping `.terragrunt-stack` directories, until no other path ends in the same elements.
// If no number of parents makes it unique, the path relative to workingDir is used.
// The working directory itself and paths outside of it get no label.
func UnitPathLabels(workingDir string, paths []string) map[string]string {
	workingDir = filepath.Clean(workingDir)

	elems := make(map[string][]string, len(paths))

	for _, path := range paths {
		path = filepath.Clean(path)

		rel, err := filepath.Rel(workingDir, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}

		elems[path] = slices.DeleteFunc(strings.Split(rel, string(filepath.Separator)), func(elem string) bool {
			return elem == hclparse.StackDir
		})
	}

	// Count how many paths end in each sequence of elements.
	suffixCounts := make(map[string]int)

	for _, pathElems := range elems {
		for i := range pathElems {
			suffixCounts[filepath.Join(pathElems[i:]...)]++
		}
	}

	labels := make(map[string]string, len(elems))

	for path, pathElems := range elems {
		label, err := filepath.Rel(workingDir, path)
		if err != nil {
			continue
		}

		for i := len(pathElems) - 1; i >= 0; i-- {
			if suffix := filepath.Join(pathElems[i:]...); suffixCounts[suffix] == 1 {
				label = suffix
				break
			}
		}

		labels[path] = label
	}

	return labels
}
