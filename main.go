package main

import (
	"os"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/gruntcreds/gruntcreds/util"
)

func main() {
	// TODO: allow build to plug in the version number
	app := shell.CreateTerragruntCli("0.0.1")
	err := app.Run(os.Args)

	if err != nil {
		util.Logger.Println(err)
		os.Exit(1)
	}
}