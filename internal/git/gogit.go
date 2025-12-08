package git

import (
	"path/filepath"

	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// GoOpenGitDir opens a Git repository using the `go-git` library, but chroots to the `.git` directory if present.
//
// Use this for operations that don't need access to the rest of the repository for read-only access, etc.
//
// Opening a Git repository leaves the storage open, so it's the responsibility of the caller to
// close the storage with `GoCloseStorage` when it is no longer needed.
func (g *GitRunner) GoOpenGitDir() error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	baseDir := g.WorkDir

	fs := osfs.New(baseDir)
	if _, err := fs.Stat(git.GitDirName); err == nil {
		fs, err = fs.Chroot(git.GitDirName)
		if err != nil {
			return err
		}
	}

	s := filesystem.NewStorageWithOptions(fs, cache.NewObjectLRUDefault(), filesystem.Options{KeepDescriptors: true})

	repo, err := git.Open(s, fs)
	if err != nil {
		return err
	}

	g.goRepo = repo
	g.goStorage = s

	return nil
}

// GoOpenRepo opens a Git repository using the `go-git` library.
func (g *GitRunner) GoOpenRepo() error {
	if err := g.RequiresWorkDir(); err != nil {
		return err
	}

	baseDir := g.WorkDir

	wt := osfs.New(baseDir)

	dotGitDir := osfs.New(baseDir)
	if _, err := dotGitDir.Stat(git.GitDirName); err == nil {
		dotGitDir, err = dotGitDir.Chroot(git.GitDirName)
		if err != nil {
			return err
		}
	}

	s := filesystem.NewStorageWithOptions(dotGitDir, cache.NewObjectLRUDefault(), filesystem.Options{KeepDescriptors: true})

	repo, err := git.Open(s, wt)
	if err != nil {
		return err
	}

	g.goRepo = repo
	g.goStorage = s

	return nil
}

// GoCloseStorage closes the storage for a Git repository.
func (g *GitRunner) GoCloseStorage() error {
	if g.goStorage == nil {
		return nil
	}

	if err := g.goStorage.Close(); err != nil {
		return err
	}

	g.goRepo = nil
	g.goStorage = nil

	return nil
}

// GoLsTreeRecursive uses the `go-git` library to recursively list the contents of a git tree.
//
// In testing, this is significantly slower than LsTreeRecursive, so we don't use it right now.
// We'll keep it here and benchmark it again later if we can optimize it.
func (g *GitRunner) GoLsTreeRecursive(ref string) ([]TreeEntry, error) {
	if err := g.RequiresGoRepo(); err != nil {
		return nil, err
	}

	h, err := g.goRepo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, err
	}

	c, err := g.goRepo.CommitObject(*h)
	if err != nil {
		return nil, err
	}

	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}

	entries, err := g.goLsTreeOnTree(tree, "")
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// GoAdd adds a file to the Git repository.
func (g *GitRunner) GoAdd(paths ...string) error {
	if err := g.RequiresGoRepo(); err != nil {
		return err
	}

	w, err := g.goRepo.Worktree()
	if err != nil {
		return err
	}

	for _, path := range paths {
		_, err := w.Add(path)
		if err != nil {
			return err
		}
	}

	return nil
}

// GoStatus gets the status of the Git repository.
func (g *GitRunner) GoStatus() (git.Status, error) {
	if err := g.RequiresGoRepo(); err != nil {
		return nil, err
	}

	w, err := g.goRepo.Worktree()
	if err != nil {
		return nil, err
	}

	return w.Status()
}

// GoCommit commits changes to the Git repository.
func (g *GitRunner) GoCommit(message string, opts *git.CommitOptions) error {
	if err := g.RequiresGoRepo(); err != nil {
		return err
	}

	if opts == nil {
		return errors.New("commit options are required for go commits")
	}

	w, err := g.goRepo.Worktree()
	if err != nil {
		return err
	}

	_, err = w.Commit(message, opts)
	if err != nil {
		return err
	}

	return nil
}

// GoOpenRepoHead gets the head of the Git repository.
func (g *GitRunner) GoOpenRepoHead() (*plumbing.Reference, error) {
	if err := g.RequiresGoRepo(); err != nil {
		return nil, err
	}

	return g.goRepo.Head()
}

// GoOpenRepoCommitObject gets a commit object from the Git repository.
func (g *GitRunner) GoOpenRepoCommitObject(hash plumbing.Hash) (*object.Commit, error) {
	if err := g.RequiresGoRepo(); err != nil {
		return nil, err
	}

	return g.goRepo.CommitObject(hash)
}

// goLsTreeOnTree uses the `go-git` library to recursively list the contents of a git tree.
func (g *GitRunner) goLsTreeOnTree(tree *object.Tree, path string) ([]TreeEntry, error) {
	entries := make([]TreeEntry, 0, len(tree.Entries))

	for _, entry := range tree.Entries {
		var entryPath string
		if path == "" {
			entryPath = entry.Name
		} else {
			entryPath = filepath.Join(path, entry.Name)
		}

		if entry.Mode.IsFile() {
			entries = append(entries, TreeEntry{
				Mode: entry.Mode.String(),
				Type: "blob",
				Hash: entry.Hash.String(),
				Path: entryPath,
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

				subTreeEntries, err := g.goLsTreeOnTree(subTree, entryPath)
				if err != nil {
					return nil, err
				}

				entries = append(entries, subTreeEntries...)
			}
		}
	}

	return entries, nil
}
