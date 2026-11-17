// Command faketofu stands in for tofu in benchmarks. It answers the commands Terragrunt runs and
// does no work, so a benchmark that points --tf-path at it measures only Terragrunt.
package main

import (
	"fmt"
	"os"
)

const (
	// version is the tofu release faketofu reports. Terragrunt parses it to decide which tofu
	// features it may use, so changing it can change what a benchmark measures.
	version = "OpenTofu v1.13.2\non linux_amd64"

	// outputs is what `output -json` prints: the one output that fixture units read from their
	// dependency.
	outputs = `{"id":{"sensitive":false,"type":"string","value":"faketofu"}}`
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		return
	}

	switch args[0] {
	case "version", "-version", "--version":
		write(version)
	case "output":
		write(outputs)
	}
}

func write(text string) {
	if _, err := fmt.Fprintln(os.Stdout, text); err != nil {
		os.Exit(1)
	}
}
