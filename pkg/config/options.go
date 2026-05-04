package config

import (
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
)

// Option is a functional option for NewParsingContext.
type Option func(*ParsingContext)

// WithStrictControls sets the strict controls for the parsing context.
func WithStrictControls(controls strict.Controls) Option {
	return func(pctx *ParsingContext) {
		pctx.StrictControls = controls
	}
}

// WithExec installs the vexec.Exec backend that subprocess-spawning HCL
// helpers (currently run_cmd) will use. It is intended for tests that want
// to intercept subprocess execution; production paths can either pass
// vexec.NewOSExec() explicitly to RunCommand, or rely on the OS-backed
// default returned by ParsingContext.Exec when no override is set.
func WithExec(e vexec.Exec) Option {
	return func(pctx *ParsingContext) {
		pctx.exec = e
	}
}
