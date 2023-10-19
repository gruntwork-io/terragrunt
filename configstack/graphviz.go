package configstack

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"

	"github.com/gruntwork-io/terragrunt/options"
)

// WriteDot is used to emit a GraphViz compatible definition
// for a directed graph. It can be used to dump a .dot file.
// This is a similar implementation to terraform's digraph https://github.com/hashicorp/terraform/blob/master/digraph/graphviz.go
// adding some styling to modules that are excluded from the execution in *-all commands
func WriteDot(w io.Writer, terragruntOptions *options.TerragruntOptions, modules []*TerraformModule) error {

	_, err := w.Write([]byte("digraph {\n"))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer func(w io.Writer, p []byte) {
		_, err := w.Write(p)
		if err != nil {
			terragruntOptions.Logger.Warnf("Failed to close graphviz output: %v", err)
		}
	}(w, []byte("}\n"))

	// all paths are relative to the TerragruntConfigPath
	prefix := filepath.Dir(terragruntOptions.TerragruntConfigPath) + "/"

	for _, source := range modules {
		// apply a different coloring for excluded nodes
		style := ""
		if source.FlagExcluded {
			style = "[color=red]"
		}

		nodeLine := fmt.Sprintf("\t\"%s\" %s;\n",
			strings.TrimPrefix(source.Path, prefix), style)

		_, err := w.Write([]byte(nodeLine))
		if err != nil {
			return errors.WithStackTrace(err)
		}

		for _, target := range source.Dependencies {
			line := fmt.Sprintf("\t\"%s\" -> \"%s\";\n",
				strings.TrimPrefix(source.Path, prefix),
				strings.TrimPrefix(target.Path, prefix),
			)
			_, err := w.Write([]byte(line))
			if err != nil {
				return errors.WithStackTrace(err)
			}
		}
	}

	return nil
}
