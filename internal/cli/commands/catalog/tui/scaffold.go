package tui

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// scaffoldCmd is a tea.ExecCommand that scaffolds a unit from the given
// component. It scaffolds both module- and template-kind components directly
// via the scaffold package. When plan is non-nil the command takes the
// interactive path: the source has already been downloaded and parsed during
// form discovery, so Run only needs to render with the user-supplied values
// and clean up.
type scaffoldCmd struct {
	component *Component
	opts      *options.TerragruntOptions
	logger    log.Logger
	plan      *scaffold.Plan
	values    map[string]string
	venv      *venv.Venv
	files     []string
}

func newScaffoldCmd(
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	c *Component,
) *scaffoldCmd {
	return &scaffoldCmd{component: c, opts: opts, logger: l, venv: v}
}

// WithPlan attaches a prepared scaffold.Plan and the user-supplied HCL
// values collected by the interactive form. Run takes ownership of the
// plan and calls Cleanup on it before returning.
func (c *scaffoldCmd) WithPlan(plan *scaffold.Plan, values map[string]string) *scaffoldCmd {
	c.plan = plan
	c.values = values

	return c
}

func (c *scaffoldCmd) Run() error {
	if c.plan != nil {
		defer c.plan.Cleanup(c.venv.FS)

		c.logger.Debugf("Generating scaffolded component: %q", c.component.TerraformSourcePath())

		if err := c.plan.Generate(context.Background(), c.logger, c.venv, c.opts, c.values); err != nil {
			return err
		}

		c.files = c.plan.GeneratedFiles()

		return nil
	}

	c.logger.Debugf("Scaffolding component: %q", c.component.TerraformSourcePath())

	plan, err := scaffold.Prepare(
		context.Background(),
		c.logger,
		c.venv,
		c.opts,
		c.component.TerraformSourcePath(),
		"",
	)
	if err != nil {
		return err
	}

	defer plan.Cleanup(c.venv.FS)

	if err := plan.Generate(context.Background(), c.logger, c.venv, c.opts, nil); err != nil {
		return err
	}

	c.files = plan.GeneratedFiles()

	return nil
}

// Files returns the paths of the files written by the last Run call, relative
// to the output directory. It is empty until Run completes successfully.
func (c *scaffoldCmd) Files() []string {
	return c.files
}

func (c *scaffoldCmd) SetStdin(io.Reader)  {}
func (c *scaffoldCmd) SetStdout(io.Writer) {}
func (c *scaffoldCmd) SetStderr(io.Writer) {}
