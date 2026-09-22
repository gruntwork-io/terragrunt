package login

import (
	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Options holds the login command's configuration.
type Options struct {
	*options.TerragruntOptions

	BaseURL string
	Force   bool
}

// NewOptions returns Options wrapping the given Terragrunt options, addressed
// at the production portal.
func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		BaseURL:           portal.DefaultBaseURL,
	}
}
