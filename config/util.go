package config

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// CopyLockFile ensures that the Terraform lock file from sourceFolder is
// replicated in destinationFolder. The file is only written when the contents
// differ, matching Terraform's behaviour of updating the lock file only when it
// changes.
func CopyLockFile(l log.Logger, opts *options.TerragruntOptions, sourceFolder, destinationFolder string) error {
	return util.CopyLockFile(sourceFolder, destinationFolder, l)
}
