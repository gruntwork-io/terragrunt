package tui

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/ignore"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// ComponentDiscovery walks an already-cloned repo and classifies every
// directory as a template, stack, unit, module, or nothing. Precedence runs
// template > stack > unit > module: a .boilerplate/ wins over a
// terragrunt.stack.hcl, which wins over a terragrunt.hcl, which wins over
// .tf files. When a directory classifies as a template, stack, or unit, the
// walker returns fs.SkipDir so nested artifacts aren't surfaced as separate
// components.
type ComponentDiscovery struct {
	extraIgnoreFile  string
	walkWithSymlinks bool
}

// NewComponentDiscovery returns a ComponentDiscovery with default settings:
// no symlink following, no extra ignore file.
func NewComponentDiscovery() *ComponentDiscovery {
	return &ComponentDiscovery{}
}

// WithWalkWithSymlinks enables symlink following during the walk.
func (cd *ComponentDiscovery) WithWalkWithSymlinks() *ComponentDiscovery {
	cd.walkWithSymlinks = true
	return cd
}

// WithExtraIgnoreFile layers an additional ignore file on top of the repo's
// .terragrunt-catalog-ignore. The extra rules are appended and take
// precedence under the "last match wins" rule. An empty path is a no-op.
func (cd *ComponentDiscovery) WithExtraIgnoreFile(i string) *ComponentDiscovery {
	cd.extraIgnoreFile = i
	return cd
}

// Discover runs component discovery against repo, walking fsys for
// component directories and README reads. repo must be non-nil; callers
// obtain it from a successful module.NewRepo and check that constructor's
// error first.
func (cd *ComponentDiscovery) Discover(fsys vfs.FS, repo *module.Repo) (Components, error) {
	repoPath := repo.Path()
	cloneURL := repo.CloneURL()

	if repoPath == "" {
		return nil, ErrEmptyRepoPath
	}

	walkFunc := func(root string, fn fs.WalkDirFunc) error {
		return vfs.WalkDir(fsys, root, fn)
	}

	if cd.walkWithSymlinks {
		walkFunc = func(root string, fn fs.WalkDirFunc) error {
			return vfs.WalkDirWithSymlinks(fsys, root, fn)
		}
	}

	ignoreMatcher, err := ignore.Load(fsys, repoPath)
	if err != nil {
		return nil, err
	}

	if cd.extraIgnoreFile != "" {
		extraMatcher, err := ignore.LoadFile(fsys, cd.extraIgnoreFile)
		if err != nil {
			return nil, err
		}

		ignoreMatcher.Merge(extraMatcher)
	}

	var components Components

	err = walkFunc(repoPath, func(dir string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if !d.IsDir() {
			return nil
		}

		if dir != repoPath && isSkippableDir(d.Name()) {
			return fs.SkipDir
		}

		relDir, err := filepath.Rel(repoPath, dir)
		if err != nil {
			return err
		}

		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}

		if ignoreMatcher.Match(relDir) {
			return fs.SkipDir
		}

		markers, err := component.Inspect(fsys, dir)
		if err != nil {
			return err
		}

		kind, isComponent := markers.Kind()
		if !isComponent {
			return nil
		}

		c, err := newComponent(fsys, repo, repoPath, cloneURL, relDir, kind)
		if err != nil {
			return err
		}

		components = append(components, c)

		// Skip descent for kinds that own their whole subtree so nested
		// artifacts (boilerplate.yml, generated .terragrunt-stack output,
		// nested .tf files inside a unit) don't surface as separate components.
		if kind == component.KindTemplate || kind == component.KindUnit ||
			kind == component.KindStack {
			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return components, nil
}

// isSkippableDir reports whether a directory name should not be descended
// into during component discovery. Skipping all dot-prefixed dirs covers .git,
// .terraform, .terragrunt-cache, .boilerplate, and similar; their contents
// either can't be components or are reached through their parent.
func isSkippableDir(name string) bool {
	return strings.HasPrefix(name, ".")
}

func newComponent(
	fsys vfs.FS,
	repo *module.Repo,
	repoPath, cloneURL, relDir string,
	kind component.Kind,
) (*Component, error) {
	doc, err := FindComponentDoc(fsys, filepath.Join(repoPath, relDir))
	if err != nil {
		return nil, err
	}

	return &Component{
		Doc:      doc,
		Repo:     repo,
		Kind:     kind,
		Dir:      relDir,
		cloneURL: cloneURL,
		repoPath: repoPath,
		url:      repo.ModuleURL(relDir),
	}, nil
}
