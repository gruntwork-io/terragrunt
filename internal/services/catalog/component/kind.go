// Package component classifies the directories a catalog offers and scaffolds
// the ones that are scaffolded by copying rather than by generating a
// configuration. Both the catalog user interface and the `scaffold` command
// drive it, so a unit or a stack is scaffolded the same way whichever one the
// user reaches for.
package component

import (
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

const (
	// boilerplateDirName and boilerplateConfigName are the two markers that
	// identify a boilerplate template.
	boilerplateDirName    = ".boilerplate"
	boilerplateConfigName = "boilerplate.yml"

	// placeholderTFFile matches the legacy ignore in module/module.go so a
	// directory holding nothing else is not mistaken for a module.
	placeholderTFFile = "terraform-cloud-enterprise-private-module-registry-placeholder.tf"
)

// Kind classifies a component directory. A module or a template is scaffolded
// by generating a configuration from it; a unit or a stack is already a
// Terragrunt configuration, and is scaffolded by copying its files.
type Kind int

const (
	// KindModule is a directory containing .tf files.
	KindModule Kind = iota
	// KindTemplate is a directory containing a `.boilerplate/` subdirectory
	// or a top-level `boilerplate.yml`.
	KindTemplate
	// KindUnit is a directory containing a `terragrunt.hcl` file.
	KindUnit
	// KindStack is a directory containing a `terragrunt.stack.hcl` file.
	KindStack
)

// String returns the user-visible kind label.
func (k Kind) String() string {
	switch k {
	case KindTemplate:
		return "template"
	case KindUnit:
		return "unit"
	case KindStack:
		return "stack"
	case KindModule:
		return "module"
	}

	return "module"
}

// IsCopyable reports whether a component of this kind is scaffolded by copying
// its directory tree into the working directory rather than by generating a
// configuration from it.
func (k Kind) IsCopyable() bool {
	return k == KindUnit || k == KindStack
}

// ConfigFile returns the Terragrunt configuration file a component of this
// kind is built around, or an empty string for kinds that have none. It names
// the file whose `values.*` references [Scaffold] resolves.
func (k Kind) ConfigFile() string {
	switch k {
	case KindUnit:
		return config.DefaultTerragruntConfigPath
	case KindStack:
		return config.DefaultStackFile
	case KindModule, KindTemplate:
		return ""
	}

	return ""
}

// Markers records which of the files that identify a component a directory
// carries. A directory can carry several at once, and what to make of that
// depends on the caller: the catalog presents such a directory as the kind
// its author most likely meant, while `scaffold` will only copy a directory
// that has no module to generate from. Reading the markers once lets each
// apply its own rule to the same facts.
type Markers struct {
	TF       bool
	Template bool
	Unit     bool
	Stack    bool
}

// Inspect reads dir and reports the component markers it carries.
func Inspect(fsys vfs.FS, dir string) (Markers, error) {
	entries, err := vfs.ReadDirEntries(fsys, dir)
	if err != nil {
		return Markers{}, err
	}

	var markers Markers

	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			if name == boilerplateDirName {
				markers.Template = true
			}

			continue
		}

		switch name {
		case config.DefaultStackFile:
			markers.Stack = true
		case config.DefaultTerragruntConfigPath:
			markers.Unit = true
		case boilerplateConfigName:
			markers.Template = true
		case placeholderTFFile:
			// Ignore: legacy Terraform Cloud/Enterprise placeholder.
		default:
			if util.IsTFFile(name) {
				markers.TF = true
			}
		}
	}

	return markers, nil
}

// Kind reports what kind of component the markers describe, and whether they
// describe one at all. A directory carrying more than one marker takes the
// precedence template > stack > unit > module, so a template built around a
// unit is offered as the template its author wrote. This is the rule the
// catalog browses by.
func (m Markers) Kind() (Kind, bool) {
	switch {
	case m.Template:
		return KindTemplate, true
	case m.Stack:
		return KindStack, true
	case m.Unit:
		return KindUnit, true
	case m.TF:
		return KindModule, true
	}

	return 0, false
}

// CopyKind reports the kind of component that is scaffolded by copying these
// markers' directory, and whether they describe one. This is the rule
// `scaffold` acts on, and it parts from [Markers.Kind] over a directory
// holding both: Terraform files mean there is a module to generate from, and
// module directories have carried a terragrunt.hcl of their own since long
// before units existed.
func (m Markers) CopyKind() (Kind, bool) {
	if m.TF {
		return 0, false
	}

	kind, ok := m.Kind()
	if !ok || !kind.IsCopyable() {
		return 0, false
	}

	return kind, true
}
