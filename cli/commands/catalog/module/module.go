// Package module provides a struct to represent an OpenTofu/Terraform module.
package module

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	defaultDescription   = "(no description found)"
	maxDescriptionLenght = 200
)

var (
	terraformFileExts = []string{".tf"}
	ignoreFiles       = []string{"terraform-cloud-enterprise-private-module-registry-placeholder.tf"}
)

type Modules []*Module

type Module struct {
	*Repo
	*Doc

	cloneURL  string
	repoPath  string
	moduleDir string
	url       string
}

// NewModule returns a module instance if the given `moduleDir` path contains a Terraform module, otherwise returns nil.
func NewModule(repo *Repo, moduleDir string) (*Module, error) {
	module := &Module{
		Repo:      repo,
		cloneURL:  repo.cloneURL,
		repoPath:  repo.path,
		moduleDir: moduleDir,
	}

	if ok, err := module.isValid(); !ok || err != nil {
		return nil, err
	}

	repo.logger.Debugf("Found module in directory %q", moduleDir)

	moduleURL, err := repo.ModuleURL(moduleDir)
	if err != nil {
		return nil, err
	}

	module.url = moduleURL

	modulePath := filepath.Join(module.repoPath, module.moduleDir)

	doc, err := FindDoc(modulePath)
	if err != nil {
		return nil, err
	}

	module.Doc = doc

	return module, nil
}

func (module *Module) Logger() log.Logger {
	return module.logger
}

// FilterValue implements /github.com/charmbracelet/bubbles.list.Item.FilterValue
func (module *Module) FilterValue() string {
	return module.Title()
}

// Title implements /github.com/charmbracelet/bubbles.list.DefaultItem.Title
func (module *Module) Title() string {
	if title := module.Doc.Title(); title != "" {
		return title
	}

	return filepath.Base(module.moduleDir)
}

// Description implements /github.com/charmbracelet/bubbles.list.DefaultItem.Description
func (module *Module) Description() string {
	if desc := module.Doc.Description(maxDescriptionLenght); desc != "" {
		return desc
	}

	return defaultDescription
}

func (module *Module) URL() string {
	return module.url
}

func (module *Module) TerraformSourcePath() string {
	return module.cloneURL + "//" + module.moduleDir
}

func (module *Module) isValid() (bool, error) {
	files, err := os.ReadDir(filepath.Join(module.repoPath, module.moduleDir))
	if err != nil {
		return false, errors.New(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if collections.ListContainsElement(ignoreFiles, file.Name()) {
			continue
		}

		ext := filepath.Ext(file.Name())
		if collections.ListContainsElement(terraformFileExts, ext) {
			return true, nil
		}
	}

	return false, nil
}

func (module *Module) ModuleDir() string {
	return module.moduleDir
}
