package tui

import (
	"errors"
	"io"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Sentinel errors returned by CopyCmd. Match with errors.Is.
var (
	ErrNilComponent    = errors.New("nil component or repo")
	ErrEmptyRepoPath   = errors.New("empty repo path")
	ErrEmptyWorkingDir = errors.New("empty working directory")
)

// CopyCmd is a tea.ExecCommand that scaffolds a unit or stack component into
// the user's working directory. It resolves the component's paths out of the
// cloned repository and hands the work to [component.Scaffold], which the
// `scaffold` command drives too.
type CopyCmd struct {
	component *Component
	opts      *options.TerragruntOptions
	logger    log.Logger
	fsys      vfs.FS
	values    map[string]string
	result    component.Result
}

// NewCopyCmd builds a CopyCmd that materializes a unit or stack component's
// files into opts.WorkingDir when Run is invoked.
func NewCopyCmd(l log.Logger, opts *options.TerragruntOptions, c *Component) *CopyCmd {
	return &CopyCmd{component: c, opts: opts, logger: l}
}

// WithFS sets the filesystem used for source reads and destination writes.
// Required: Run panics on a nil filesystem.
func (c *CopyCmd) WithFS(fsys vfs.FS) *CopyCmd {
	c.fsys = fsys
	return c
}

// WithValues threads user-supplied HCL fragments into the generated
// terragrunt.values.hcl. The map is keyed by the bare variable name (e.g.,
// `name`, not `values.name`); the lookup in component.WriteValuesFile uses the
// reference's bare name. Entries not in the map fall back to the same
// `"TODO"` / try-fallback behavior as the placeholder flow.
func (c *CopyCmd) WithValues(values map[string]string) *CopyCmd {
	c.values = values
	return c
}

// Run performs the copy, optionally writing a terragrunt.values.hcl stub
// when the source has values.* references.
func (c *CopyCmd) Run() error {
	fsys := c.fsys
	if fsys == nil {
		panic("tui.CopyCmd: nil filesystem; wire the venv FS at construction")
	}

	paths, err := c.resolvePaths()
	if err != nil {
		return err
	}

	c.logger.Debugf("Copying component %q to %q", paths.Src, paths.Dst)

	result, err := component.Scaffold(fsys, c.component.Kind, paths, c.values)
	if err != nil {
		return err
	}

	c.result = result

	return nil
}

// Result exposes the outcome of the last Run call. Intended for the TUI
// update loop to format an exit message; tests may use it too.
func (c *CopyCmd) Result() component.Result {
	return c.result
}

// SetStdin is a no-op; CopyCmd does not interact with stdio and only
// implements this method to satisfy the tea.ExecCommand interface.
func (c *CopyCmd) SetStdin(io.Reader) {}

// SetStdout is a no-op; see SetStdin.
func (c *CopyCmd) SetStdout(io.Writer) {}

// SetStderr is a no-op; see SetStdin.
func (c *CopyCmd) SetStderr(io.Writer) {}

// resolvePaths locates this copy: the cloned repository, the component within
// it, and the user's working directory. Files from the component are
// materialized directly into the working directory so the action mirrors how
// scaffold emits its output.
func (c *CopyCmd) resolvePaths() (component.Paths, error) {
	if c.component == nil || c.component.Repo == nil {
		return component.Paths{}, ErrNilComponent
	}

	repoPath := c.component.Repo.Path()
	if repoPath == "" {
		return component.Paths{}, ErrEmptyRepoPath
	}

	src := repoPath
	if c.component.Dir != "" {
		src = filepath.Join(repoPath, filepath.FromSlash(c.component.Dir))
	}

	workingDir := c.opts.WorkingDir
	if workingDir == "" {
		return component.Paths{}, ErrEmptyWorkingDir
	}

	return component.Paths{Root: repoPath, Src: src, Dst: workingDir}, nil
}
