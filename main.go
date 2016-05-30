package main

import (
	"os"
	"github.com/gruntwork-io/gruntcreds/gruntcreds/util"
	"github.com/gruntwork-io/terragrunt/cli"
)

func main() {
	// TODO: allow build to plug in the version number
	app := cli.CreateTerragruntCli("0.0.1")
	err := app.Run(os.Args)

	if err != nil {
		util.Logger.Println(err)
		os.Exit(1)
	}
}