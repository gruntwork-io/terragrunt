package tui

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/ignore"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// boilerplateDirName and boilerplateConfigName are the two markers that
// classify a directory as a template.
const (
	boilerplateDirName    = ".boilerplate"
	boilerplateConfigName = "boilerplate.yml"

	// placeholderTFFile matches the legacy ignore in module/module.go so
	// repos that contain the Terraform Cloud/Enterprise private registry
	// placeholder aren't misclassified as modules.
	placeholderTFFile = "terraform-cloud-enterprise-private-module-registry-placeholder.tf"
)

// ComponentDiscovery walks an already-cloned repo and classifies every
// directory as a template, stack, unit, module, or nothing. Precedence runs
// template > stack > unit > module: a .boilerplate/ wins over a
// terragrunt.stack.hcl, which wins over a terragrunt.hcl, which wins over
// .tf files. When a directory classifies as a template, stack, or unit, the
// walker returns fs.SkipDir so nested artifacts aren't surfaced as separate
// components.
type ComponentDiscovery struct {
	fsys             vfs.FS
	extraIgnoreFile  string
	walkWithSymlinks bool
}

// NewComponentDiscovery returns a ComponentDiscovery with default settings:
// no symlink following, no extra ignore file, the real OS filesystem.
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

// WithFS sets the filesystem used for the discovery walk and per-component
// README reads. When unset, Discover uses vfs.NewOSFS().
func (cd *ComponentDiscovery) WithFS(fsys vfs.FS) *ComponentDiscovery {
	cd.fsys = fsys
	return cd
}

// Discover runs component discovery against repo. repo must be non-nil;
// callers obtain it from a successful module.NewRepo and check that
// constructor's error first.
func (cd *ComponentDiscovery) Discover(repo *module.Repo) (Components, error) {
	repoPath := repo.Path()
	cloneURL := repo.CloneURL()

	if repoPath == "" {
		return nil, ErrEmptyRepoPath
	}

	fsys := cd.fsys
	if fsys == nil {
		fsys = vfs.NewOSFS()
	}

	walkFunc := func(root string, fn fs.WalkDirFunc) error {
		return vfs.WalkDir(fsys, root, fn)
	}

	if cd.walkWithSymlinks {
		walkFunc = util.WalkDirWithSymlinks
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

		kind, isComponent, err := classifyDir(fsys, dir)
		if err != nil {
			return err
		}

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
		if kind == ComponentKindTemplate || kind == ComponentKindUnit ||
			kind == ComponentKindStack {
			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return components, nil
}

func classifyDir(fsys vfs.FS, dir string) (ComponentKind, bool, error) {
	entries, err := vfs.ReadDirEntries(fsys, dir)
	if err != nil {
		return 0, false, err
	}

	var (
		hasTF       bool
		hasTemplate bool
		hasUnit     bool
		hasStack    bool
	)

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			if name == boilerplateDirName {
				hasTemplate = true
			}

			continue
		}

		switch name {
		case config.DefaultStackFile:
			hasStack = true
		case config.DefaultTerragruntConfigPath:
			hasUnit = true
		case boilerplateConfigName:
			hasTemplate = true
		case placeholderTFFile:
			// Ignore: legacy Terraform Cloud/Enterprise placeholder.
		default:
			if util.IsTFFile(name) {
				hasTF = true
			}
		}
	}

	switch {
	case hasTemplate:
		return ComponentKindTemplate, true, nil
	case hasStack:
		return ComponentKindStack, true, nil
	case hasUnit:
		return ComponentKindUnit, true, nil
	case hasTF:
		return ComponentKindModule, true, nil
	}

	return 0, false, nil
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
	kind ComponentKind,
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
