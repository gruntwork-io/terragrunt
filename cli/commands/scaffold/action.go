package scaffold

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/fixture-download/invalid-path/.terragrunt-cache/JcmikJhv4-73PZ_MObPtv2y-Blk/p_piCTTWVab2Hmnj1OtnAruj8J4/errors"
	"github.com/hashicorp/go-getter"
)

func Run(opts *options.TerragruntOptions) error {
	// download remote repo to local
	moduleUrl := ""
	templateUrl := ""
	if len(opts.TerraformCliArgs) >= 2 {
		moduleUrl = opts.TerraformCliArgs[1]
	}

	if len(opts.TerraformCliArgs) >= 3 {
		templateUrl = opts.TerraformCliArgs[2]
	}

	opts.Logger.Infof("Scaffolding a new Terragrunt module %s %s to %s", moduleUrl, templateUrl, opts.WorkingDir)

	err := getter.GetAny(opts.WorkingDir, moduleUrl)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// run boilerplate

	//boilerplate := cli.CreateBoilerplateCli()

	return nil
}
