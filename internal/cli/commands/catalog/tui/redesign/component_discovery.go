package redesign

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/ignore"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/util"
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
// directory as either a module, a template, or neither. Templates win over
// modules when a directory qualifies as both. When a directory is classified
// as a template, the walker returns fs.SkipDir so that boilerplate.yml files
// inside the .boilerplate subtree are not double-counted.
//
// Unlike the legacy module.Repo.FindModules walker (which only scans the
// `modules/` convention), this walks the entire repo — templates may live
// anywhere, and the redesign treats module/template discovery uniformly.
//
// Construct one via NewComponentDiscovery, customize it with the With*
// methods, then call Discover on a repo.
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

// Discover runs component discovery against repo.
func (cd *ComponentDiscovery) Discover(repo *module.Repo) (Components, error) {
	if repo == nil {
		return nil, errors.New("ComponentDiscovery.Discover: nil repo")
	}

	repoPath := repo.Path()
	cloneURL := repo.CloneURL()

	if repoPath == "" {
		return nil, errors.New("ComponentDiscovery.Discover: empty repo path")
	}

	walkFunc := filepath.WalkDir
	if cd.walkWithSymlinks {
		walkFunc = util.WalkDirWithSymlinks
	}

	ignoreMatcher, err := ignore.Load(repoPath)
	if err != nil {
		return nil, err
	}

	if cd.extraIgnoreFile != "" {
		extraMatcher, err := ignore.LoadFile(cd.extraIgnoreFile)
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
			return errors.New(err)
		}

		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}

		if ignoreMatcher.Match(relDir) {
			return fs.SkipDir
		}

		kind, isComponent, err := classifyDir(dir)
		if err != nil {
			return err
		}

		if !isComponent {
			return nil
		}

		c, err := newComponent(repo, repoPath, cloneURL, relDir, kind)
		if err != nil {
			return err
		}

		components = append(components, c)

		// Templates: skip descent so we don't re-enter the .boilerplate
		// subtree and classify its inner boilerplate.yml as a second
		// component.
		if kind == ComponentKindTemplate {
			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return components, nil
}

// classifyDir inspects a single directory and returns its ComponentKind.
// Template classification wins over module classification.
func classifyDir(dir string) (ComponentKind, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false, errors.New(err)
	}

	var hasTF bool

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			if name == boilerplateDirName {
				return ComponentKindTemplate, true, nil
			}

			continue
		}

		if name == boilerplateConfigName {
			return ComponentKindTemplate, true, nil
		}

		if name == placeholderTFFile {
			continue
		}

		if util.IsTFFile(name) {
			hasTF = true
		}
	}

	if hasTF {
		return ComponentKindModule, true, nil
	}

	return 0, false, nil
}

// isSkippableDir reports whether a directory name should not be descended
// into during component discovery. We skip all dot-prefixed dirs (.git,
// .terraform, .terragrunt-cache, .boilerplate, etc.) because their contents
// either can't be components or should only be discovered via their parent.
func isSkippableDir(name string) bool {
	return strings.HasPrefix(name, ".")
}

// newComponent constructs a *Component for a directory that has been
// classified. It populates the doc and URL fields the same way the legacy
// module.NewModule does, but into the redesign-owned Component type.
func newComponent(repo *module.Repo, repoPath, cloneURL, relDir string, kind ComponentKind) (*Component, error) {
	doc, err := FindComponentDoc(filepath.Join(repoPath, relDir))
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
