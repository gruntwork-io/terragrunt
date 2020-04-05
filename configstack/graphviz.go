package configstack

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"io"
	"strings"
)

// WriteDot is used to emit a GraphViz compatible definition
// for a directed graph. It can be used to dump a .dot file.
func WriteDot(w io.Writer, terragruntOptions *options.TerragruntOptions, modules []*TerraformModule) error {

	w.Write([]byte("digraph {\n"))
	defer w.Write([]byte("}\n"))

	for _, source := range modules {
		// apply a different coloring for excluded nodes
		style := ""
		if source.FlagExcluded {
			style = fmt.Sprintf("[fillcolor=red]")
		}

		nodeLine := fmt.Sprintf("\t\"%s\" %s;\n",
			strings.TrimPrefix(source.Path, terragruntOptions.WorkingDir), style)

		w.Write([]byte(nodeLine))
		for _, target := range source.Dependencies {
			line := fmt.Sprintf("\t\"%s\" -> \"%s\";\n",
				strings.TrimPrefix(source.Path, terragruntOptions.WorkingDir),
				strings.TrimPrefix(target.Path, terragruntOptions.WorkingDir),
			)
			w.Write([]byte(line))
		}
	}

	return nil
}
