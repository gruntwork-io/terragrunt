package redesign

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// scaffoldCmd is a tea.ExecCommand that scaffolds a unit from the given
// component. It bypasses catalog.CatalogService so the redesign can scaffold
// both module- and template-kind components without depending on the legacy
// service.
type scaffoldCmd struct {
	component *Component
	opts      *options.TerragruntOptions
	logger    log.Logger
}

func newScaffoldCmd(logger log.Logger, opts *options.TerragruntOptions, c *Component) *scaffoldCmd {
	return &scaffoldCmd{component: c, opts: opts, logger: logger}
}

func (c *scaffoldCmd) Run() error {
	c.logger.Debugf("Scaffolding component: %q", c.component.TerraformSourcePath())

	// TODO: thread venv from the CLI entrypoint through the catalog TUI
	// so this leaf participates in the root virtualized environment.
	return scaffold.Run(context.Background(), c.logger, venv.OSVenv(), c.opts, c.component.TerraformSourcePath(), "")
}

func (c *scaffoldCmd) SetStdin(io.Reader)  {}
func (c *scaffoldCmd) SetStdout(io.Writer) {}
func (c *scaffoldCmd) SetStderr(io.Writer) {}
