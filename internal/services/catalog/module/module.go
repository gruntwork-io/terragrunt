// Package module provides a struct to represent an OpenTofu/Terraform module.
package module

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	defaultDescription   = "(no description found)"
	maxDescriptionLength = 200
)

var ignoreFiles = []string{"terraform-cloud-enterprise-private-module-registry-placeholder.tf"}

// Modules is a list of catalog modules.
type Modules []*Module

// Module is an OpenTofu/Terraform module found in a catalog repository, with its README.
type Module struct {
	*Repo
	*Doc

	cloneURL  string
	repoPath  string
	moduleDir string
	url       string
}

// NewModule returns a module instance if the given `moduleDir` path contains an OpenTofu/Terraform module, otherwise returns nil.
func NewModule(l log.Logger, fsys vfs.FS, repo *Repo, moduleDir string) (*Module, error) {
	module := &Module{
		Repo:      repo,
		cloneURL:  repo.cloneURL,
		repoPath:  repo.path,
		moduleDir: moduleDir,
	}

	if ok, err := module.isValid(fsys); !ok || err != nil {
		return nil, err
	}

	l.Debugf("Found module in directory %q", moduleDir)

	module.url = repo.ModuleURL(moduleDir)

	l.Debugf("Module URL: %s", module.url)

	modulePath := filepath.Join(module.repoPath, module.moduleDir)

	doc, err := FindDoc(fsys, modulePath)
	if err != nil {
		return nil, err
	}

	module.Doc = doc

	return module, nil
}

// FilterValue implements /github.com/charmbracelet/bubbles.list.Item.FilterValue
func (module *Module) FilterValue() string {
	return module.Title()
}

// Title implements /github.com/charmbracelet/bubbles.list.DefaultItem.Title
func (module *Module) Title() string {
	if title := module.Doc.Title(); title != "" {
		return strings.TrimSpace(title)
	}

	return filepath.Base(module.moduleDir)
}

// Description implements /github.com/charmbracelet/bubbles.list.DefaultItem.Description
func (module *Module) Description() string {
	if desc := module.Doc.Description(maxDescriptionLength); desc != "" {
		return desc
	}

	return defaultDescription
}

// URL returns the address to view the module in a browser. It is the local path when the repository has no
// remote, and empty when the catalog does not support the remote's format.
func (module *Module) URL() string {
	return module.url
}

// TerraformSourcePath returns the module source URL in the format expected by go-getter:
// baseURL//moduleDir?query (e.g., git::https://github.com/org/repo.git//modules/foo?ref=v1.0.0)
func (module *Module) TerraformSourcePath() string {
	return SourcePath(module.cloneURL, module.moduleDir)
}

func (module *Module) isValid(fsys vfs.FS) (bool, error) {
	files, err := vfs.ReadDir(fsys, filepath.Join(module.repoPath, module.moduleDir))
	if err != nil {
		return false, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if slices.Contains(ignoreFiles, file.Name()) {
			continue
		}

		if util.IsTFFile(file.Name()) {
			return true, nil
		}
	}

	return false, nil
}

// ModuleDir returns the module's directory relative to the repository root.
func (module *Module) ModuleDir() string {
	return module.moduleDir
}

// NewModuleForTest creates a Module for testing purposes.
func NewModuleForTest(cloneURL, moduleDir string) *Module {
	return &Module{
		cloneURL:  cloneURL,
		moduleDir: moduleDir,
	}
}
