package graph

import (
	"bytes"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"os/exec"
)

type TerraformGraph struct {
	graph graph
}

func GetParsedTerraformGraph(terraformDir string) (*TerraformGraph, error) {
	cmd := exec.Command("terraform", "graph")
	cmd.Dir = terraformDir

	stdout, err := cmd.Output()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return Parse(stdout)
}

func Parse(terraformGraphOutput []byte) (*TerraformGraph, error) {
	graph, err := parse(bytes.NewReader(terraformGraphOutput))
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &TerraformGraph{graph: graph}, nil
}

// DoesModuleDependOnModule returns true if fromModule depends on toModule and false otherwise.
func (graph *TerraformGraph) DoesModuleDependOnModule(fromModule string, toModule string) (bool, error) {
	fromModuleFormatted := formatModuleNameClose(fromModule)
	toModuleFormatted := formatModuleNameExpand(toModule)

	if graph.graph[fromModuleFormatted] == nil {
		return false, fmt.Errorf("The given module name does not show up in the dependency graph: %s", fromModule)
	}

	if graph.graph[toModuleFormatted] == nil {
		return false, fmt.Errorf("The given module name does not show up in the dependency graph: %s", toModuleFormatted)
	}

	roots := make(nodeset)
	roots[toModuleFormatted] = true

	// We need to do a "reverse" check
	graphToCheck := graph.graph.transpose()
	reachableNodes := graphToCheck.reachableFrom(roots)

	for reachableNode, isReachable := range reachableNodes {
		if reachableNode == fromModuleFormatted && isReachable {
			return true, nil
		}
	}

	return false, nil
}

// The 'terraform graph' output formats root module names in a specific way; we must use this exact format to be able to
// navigate the graph it returns.
func formatModuleNameExpand(moduleName string) string {
	return fmt.Sprintf("[root] module.%s (expand)", moduleName)
}

// The 'terraform graph' output formats root module names in a specific way; we must use this exact format to be able to
// navigate the graph it returns.
func formatModuleNameClose(moduleName string) string {
	return fmt.Sprintf("[root] module.%s (close)", moduleName)
}
