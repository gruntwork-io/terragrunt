package common

import (
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
)

// Option applies configuration to a StackRunner.
type Option interface {
	Apply(stack StackRunner)
}

// optionImpl is a lightweight Option implementation that wraps an apply function
// and optionally carries HCL parser options.
type optionImpl struct {
	apply         func(StackRunner)
	parserOptions []hclparse.Option
}

func (o optionImpl) Apply(stack StackRunner) {
	if o.apply != nil {
		o.apply(stack)
	}
}

// ParseOptionsProvider exposes HCL parser options carried by an Option.
type ParseOptionsProvider interface {
	GetParseOptions() []hclparse.Option
}

// GetParseOptions returns the HCL parser options attached to the option, if any.
func (o optionImpl) GetParseOptions() []hclparse.Option {
	if len(o.parserOptions) > 0 {
		return o.parserOptions
	}

	return nil
}

// WithParseOptions provides custom HCL parser options to both discovery and stack execution.
func WithParseOptions(parserOptions []hclparse.Option) Option {
	return optionImpl{
		// No-op apply for runner; discovery picks up parser options via GetParseOptions
		apply:         func(StackRunner) {},
		parserOptions: parserOptions,
	}
}

// ReportProvider exposes the report attached to an Option.
type ReportProvider interface {
	GetReport() *report.Report
}

// reportOption wraps a report and implements both Option and ReportProvider.
type reportOption struct {
	report *report.Report
}

func (o reportOption) Apply(stack StackRunner) {
	stack.SetReport(o.report)
}

func (o reportOption) GetReport() *report.Report {
	return o.report
}

// WithReport attaches a report collector to the stack, enabling run summaries and metrics.
func WithReport(r *report.Report) Option {
	return reportOption{report: r}
}

// WorktreeOption carries worktrees through the runner pipeline for git filter expressions.
type WorktreeOption struct {
	Worktrees *worktrees.Worktrees
}

// Apply is a no-op for runner (worktrees are used in discovery, not runner execution).
func (o WorktreeOption) Apply(stack StackRunner) {}

// WithWorktrees provides git worktrees to discovery for git filter expressions.
func WithWorktrees(w *worktrees.Worktrees) Option {
	return WorktreeOption{Worktrees: w}
}
