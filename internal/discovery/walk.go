package discovery

import (
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// walkRootUnder spells walkRoot below workingDir, since the pruned walk
// compares paths as text. An empty walkRoot means the whole working directory.
// [NewForStackGenerate] sets walkRoot only when it sits within workingDir, but
// walkRoot has its symlinks resolved and workingDir may not.
func walkRootUnder(fsys vfs.FS, workingDir, walkRoot string) (string, error) {
	if walkRoot == "" {
		return workingDir, nil
	}

	rel, err := filepath.Rel(vfs.ResolveForCompare(fsys, workingDir), vfs.ResolveForCompare(fsys, walkRoot))
	if err != nil {
		return "", err
	}

	return filepath.Join(workingDir, rel), nil
}

// skipDirIfIgnorable determines if a directory should be skipped during traversal.
func (d *Discovery) skipDirIfIgnorable(dir string) error {
	if err := util.SkipDirIfIgnorable(dir); err != nil {
		return err
	}

	if d.noHidden {
		if strings.HasPrefix(dir, ".") && dir != "." && dir != ".." {
			return filepath.SkipDir
		}
	}

	return nil
}

// classifyFile returns the component the config file at path makes, classified without a parse. It is nil when
// path is none of filenames.
func (d *Discovery) classifyFile(path string, filenames []string) *DiscoveryResult {
	c := createComponentFromPath(path, filenames, d.discoveryContext)
	if c == nil {
		return nil
	}

	status, reason, graphIdx := d.classifier.Classify(c, filter.ClassificationContext{})

	return &DiscoveryResult{
		Component:            c,
		Status:               status,
		Reason:               reason,
		GraphExpressionIndex: graphIdx,
	}
}
