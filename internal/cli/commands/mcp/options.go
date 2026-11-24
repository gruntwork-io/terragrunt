package mcp

import (
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Options are the options for the mcp command.
type Options struct {
	*options.TerragruntOptions

	Allow              []string
	AllowCmds          []string
	DocsStartupTimeout time.Duration
	DocsCallTimeout    time.Duration
	AllowApply         bool
}

// NewOptions returns a new [Options] with the given TerragruntOptions.
func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions:  opts,
		DocsStartupTimeout: defaultDocsStartupTimeout,
		DocsCallTimeout:    defaultDocsCallTimeout,
	}
}
