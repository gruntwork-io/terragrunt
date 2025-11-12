package git

import (
	"path/filepath"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GoLsTreeRecursive uses the `go-git` library to recursively list the contents of a git tree.
//
// In testing, this is significantly slower than LsTreeRecursive, so we don't use it right now.
// We'll keep it here and benchmark it again later if we can optimize it.
func (g *GitRunner) GoLsTreeRecursive(l log.Logger, ref, path string) ([]TreeEntry, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	baseDir := g.WorkDir

	fs := osfs.New(baseDir)
	if _, err := fs.Stat(git.GitDirName); err == nil {
		fs, err = fs.Chroot(git.GitDirName)
		if err != nil {
			return nil, err
		}
	}

	s := filesystem.NewStorageWithOptions(fs, cache.NewObjectLRUDefault(), filesystem.Options{KeepDescriptors: true})

	repo, err := git.Open(s, fs)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := s.Close(); closeErr != nil {
			l.Errorf("failed to close git storage: %s", closeErr.Error())
		}
	}()

	h, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, err
	}

	c, err := repo.CommitObject(*h)
	if err != nil {
		return nil, err
	}

	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	entries, err := g.goLsTreeOnTree(tree, path)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// goLsTreeOnTree uses the `go-git` library to recursively list the contents of a git tree.
func (g *GitRunner) goLsTreeOnTree(tree *object.Tree, path string) ([]TreeEntry, error) {
	entries := make([]TreeEntry, 0, len(tree.Entries))

	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			entries = append(entries, TreeEntry{
				Mode: entry.Mode.String(),
				Type: "blob",
				Hash: entry.Hash.String(),
				Path: filepath.Join(path, entry.Name),
			})
		} else {
			mode, err := entry.Mode.ToOSFileMode()
			if err != nil {
				return nil, err
			}

			if mode.IsDir() {
				subTree, err := tree.Tree(entry.Name)
				if err != nil {
					return nil, err
				}

				subTreePath := filepath.Join(path, entry.Name)

				subTreeEntries, err := g.goLsTreeOnTree(subTree, subTreePath)
				if err != nil {
					return nil, err
				}

				entries = append(entries, subTreeEntries...)
			}
		}
	}

	return entries, nil
}
