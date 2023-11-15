package scaffold

import (
	"fmt"

	boilerplate_options "github.com/gruntwork-io/boilerplate/options"
	"github.com/gruntwork-io/boilerplate/templates"
	"github.com/gruntwork-io/boilerplate/variables"
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
	opts.Logger.Infof("Running boilerplate in %s", opts.WorkingDir)

	bopts := &boilerplate_options.BoilerplateOptions{
		TemplateUrl:  fmt.Sprintf("%s//boilerplate", opts.WorkingDir),
		OutputFolder: opts.WorkingDir,
	}

	emptyDep := variables.Dependency{}
	err = templates.ProcessTemplate(bopts, bopts, emptyDep)
	if err != nil {
		return err
	}

	return nil
}
