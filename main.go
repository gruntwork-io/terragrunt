package main

import (
	"github.com/gruntwork-io/gruntwork-cli/entrypoint"
	"github.com/gruntwork-io/terragrunt/cli"
	"os"
)

// This variable is set at build time using -ldflags parameters. For more info, see:
// http://stackoverflow.com/a/11355611/483528
var VERSION string

// The main entrypoint for Terragrunt
func main() {
	app := cli.CreateTerragruntCli(VERSION, os.Stdout, os.Stderr)
	entrypoint.RunApp(app)
}
