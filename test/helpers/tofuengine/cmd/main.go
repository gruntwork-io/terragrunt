// Command tofuengine serves the test engine from package tofuengine.
package main

import "github.com/gruntwork-io/terragrunt/test/helpers/tofuengine"

func main() {
	tofuengine.Serve()
}
