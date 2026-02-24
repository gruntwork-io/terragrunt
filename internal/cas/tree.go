package cas

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"golang.org/x/sync/errgroup"
)

// LinkTree writes the tree to a target directory
func LinkTree(ctx context.Context, store *Store, t *git.Tree, targetDir string) error {
	content := NewContent(store)

	dirsToCreate := make(map[string]struct{}, len(t.Entries()))

	type workItem struct {
		itemType string
		entry    git.TreeEntry
		path     string
		dirPath  string
	}

	workItems := make([]workItem, 0, len(t.Entries()))

	for _, entry := range t.Entries() {
		entryPath := filepath.Join(targetDir, entry.Path)
		dirPath := filepath.Dir(entryPath)

		dirsToCreate[dirPath] = struct{}{}

		// If the parent directory is in dirsToCreate,
		// we can remove it, since it will be created
		// when creating the subtree anyways.
		parentDirPath := filepath.Dir(dirPath)
		delete(dirsToCreate, parentDirPath)

		// Create work items based on entry type
		switch entry.Type {
		case "blob":
			workItems = append(workItems, workItem{
				itemType: "link",
				entry:    entry,
				path:     entryPath,
				dirPath:  dirPath,
			})
		case "tree":
			workItems = append(workItems, workItem{
				itemType: "subtree",
				entry:    entry,
				path:     entryPath,
				dirPath:  dirPath,
			})
		}
	}

	for dirPath := range dirsToCreate {
		if err := os.MkdirAll(dirPath, DefaultDirPerms); err != nil {
			return wrapError("mkdir_all", dirPath, err)
		}
	}

	// Use errgroup for concurrent processing
	g, ctx := errgroup.WithContext(ctx)

	// Set concurrency limit
	scalingFactor := 2
	maxWorkers := max(1, runtime.NumCPU()/scalingFactor)
	g.SetLimit(maxWorkers)

	// Process work items concurrently
	for _, work := range workItems {
		g.Go(func() error {
			switch work.itemType {
			case "link":
				err := content.Link(ctx, work.entry.Hash, work.path)
				if err != nil {
					return wrapError("link_blob", work.path, err)
				}
			case "subtree":
				treeData, err := content.Read(work.entry.Hash)
				if err != nil {
					return wrapError("read_tree", work.entry.Hash, err)
				}

				subTree, err := git.ParseTree(treeData, work.path)
				if err != nil {
					return wrapError("parse_tree", work.entry.Hash, err)
				}

				err = LinkTree(ctx, store, subTree, work.path)
				if err != nil {
					return wrapError("link_subtree", work.path, err)
				}
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and return first error if any
	return g.Wait()
}
