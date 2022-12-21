package graph

import (
	"bytes"
	goerrors "errors"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"os/exec"
	"regexp"
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
	graph, err := parse(bytes.NewReader(cleanTerraformGraphOutput(terraformGraphOutput)))
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &TerraformGraph{graph: graph}, nil
}

var terraformGraphLabelsRegexp = regexp.MustCompile(`(\s+".+?") \[label = .+]`)

// The 'terraform graph' command includes labels that are not strictly necessary, but which the digraph code interprets
// as nodes. So, we strip those labels so that we can get a clean parse without extraneous nodes.
func cleanTerraformGraphOutput(terraformGraphOutput []byte) []byte {
	return terraformGraphLabelsRegexp.ReplaceAll(terraformGraphOutput, []byte("$1"))
}

func (graph *TerraformGraph) Clone() *TerraformGraph {
	newGraph := map[string]nodeset{}
	for node, originalEdges := range graph.graph {
		newEdges := map[string]bool{}
		for edge, val := range originalEdges {
			newEdges[edge] = val
		}
		newGraph[node] = newEdges
	}
	return &TerraformGraph{graph: newGraph}
}

func (graph *TerraformGraph) RemoveModule(moduleName string) error {
	return graph.removeNodeAndEdges(fmt.Sprintf("module.%s", moduleName))
}

func (graph *TerraformGraph) RemoveOutput(outputName string) error {
	return graph.removeNodeAndEdges(fmt.Sprintf("output.%s", outputName))
}

func (graph *TerraformGraph) RemoveVariable(variableName string) error {
	return graph.removeNodeAndEdges(fmt.Sprintf("var.%s", variableName))
}

func (graph *TerraformGraph) RemoveLocal(localName string) error {
	return graph.removeNodeAndEdges(fmt.Sprintf("local.%s", localName))
}

func (graph *TerraformGraph) RemoveProvider(providerName string) error {
	// TODO: this assumes all providers are from hashicorp... We need a way to support custom providers too!
	return graph.removeNodeAndEdges(fmt.Sprintf(`provider["registry.terraform.io/hashicorp/%s"]`, providerName))
}

func (graph *TerraformGraph) RemoveResource(resourceType string, resourceName string) error {
	return graph.removeNodeAndEdges(fmt.Sprintf(`%s.%s`, resourceType, resourceName))
}

func (graph *TerraformGraph) RemoveDataSource(dataSourceType string, dataSourceName string) error {
	return graph.removeNodeAndEdges(fmt.Sprintf(`%s.%s`, dataSourceType, dataSourceName))
}

func (graph *TerraformGraph) removeNodeAndEdges(nodeName string) error {
	nodeRegex, err := createNodeNameRegex(nodeName)
	if err != nil {
		return err
	}

	for key, edges := range graph.graph {
		if nodeRegex.MatchString(key) {
			delete(graph.graph, key)
		} else {
			for edge, _ := range edges {
				if nodeRegex.MatchString(edge) {
					delete(edges, edge)
				}
			}
		}
	}

	return nil
}

// Returns a regexp that matches the given nodeName anywhere in the graph. For example, if nodeName is module.vpc,
// this will return a regexp that will match any of the following:
//
// module.vpc
// module.vpc.vpc_id
// module.vpc.vpc_id (expand)
// [root] module.vpc.vpc_id (expand)
func createNodeNameRegex(nodeName string) (*regexp.Regexp, error) {
	nodeRegex, err := regexp.Compile(fmt.Sprintf(`^(\[root] )?%s($| |\.)`, nodeName))
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return nodeRegex, nil
}

// DoesModuleDependOnModule returns true if the module fromModule depends on the module toModule, and false otherwise.
func (graph *TerraformGraph) DoesModuleDependOnModule(fromModule string, toModule string) (bool, error) {
	return graph.doesFromDependOnTo(formatModuleNameClose(fromModule), formatModuleNameExpand(toModule))
}

// DoesModuleDependOnVariable returns true if the module fromModule depends on the input variable toVariable, and false
// otherwise.
func (graph *TerraformGraph) DoesModuleDependOnVariable(fromModule string, toVariable string) (bool, error) {
	return graph.doesFromDependOnTo(formatModuleNameClose(fromModule), formatVariableName(toVariable))
}

// DoesModuleDependOnDataSource returns true if the module fromModule depends on the data source toDataSource, of type
// toDataSourceType, and false otherwise.
func (graph *TerraformGraph) DoesModuleDependOnDataSource(fromModule string, toDataSourceType string, toDataSourceName string) (bool, error) {
	return graph.doesFromDependOnTo(formatModuleNameClose(fromModule), formatDataSourceName(toDataSourceType, toDataSourceName))
}

// DoesOutputDependOnModule returns true if the output variable fromOutput depends on the module toModule, and false
// otherwise.
func (graph *TerraformGraph) DoesOutputDependOnModule(fromOutput string, toModule string) (bool, error) {
	return graph.doesFromDependOnTo(formatOutputName(fromOutput), formatModuleNameExpand(toModule))
}

// DoesOutputDependOnVariable returns true if the output variable fromOutput depends on the input variable toVariable,
// and false otherwise.
func (graph *TerraformGraph) DoesOutputDependOnVariable(fromOutput string, toVariable string) (bool, error) {
	return graph.doesFromDependOnTo(formatOutputName(fromOutput), formatVariableName(toVariable))
}

// DoesAnythingDependOnLocal returns true if anything (any module, resource, data source, provider, etc) depends on the
// local variable toLocal.
func (graph *TerraformGraph) DoesAnythingDependOnLocal(toLocal string) (bool, error) {
	return graph.doesAnythingDependOnNode(formatLocalName(toLocal))
}

// DoesAnythingDependOnLocal returns true if anything (any module, resource, data source, provider, etc) depends on the
// input variable toVariable.
func (graph *TerraformGraph) DoesAnythingDependOnVariable(toVariable string) (bool, error) {
	return graph.doesAnythingDependOnNode(formatVariableName(toVariable))
}

// DoesAnythingDependOnProvider returns true if anything (any module, resource, data source, etc) depends on the
// provider toProvider.
func (graph *TerraformGraph) DoesAnythingDependOnProvider(toProvider string) (bool, error) {
	return graph.doesAnythingDependOnNode(formatProviderName(toProvider))
}

// The output of the 'terraform graph' command has "nodes" that we don't really care about such as:
//
// =
// ->
// [root] root
//
// The nodes we do care about look like this:
//
// module.xxx
// var.xxx
// output.xxx
// local.xxx
// provider["xxx"]
// aws_instance.xxx
// data.terraform_remote_state.xxx
//
// So this is a regex that helps us much the patterns we do care about, while ignoring the ones we don't care about.
var nodePatternsWeCareAboutRegexp = regexp.MustCompile(`([0-9a-zA-Z-_]+\.[0-9a-zA-Z-_])|(provider\[".+"])`)

// doesAnythingDependOnNode returns true if anything (any module, resource, data source, provider, etc) depends on the
// given node. This method assumes that formattedNodeName is already formatted to the way the 'terraform graph' command
// names nodes, using one of the formatXXX methods below.
func (graph *TerraformGraph) doesAnythingDependOnNode(formattedNodeName string) (doesDepend bool, recoveredErr error) {
	// The digraph code uses panic, so we try to recover from those panics and return errors as values instead
	defer func() {
		if err := recover(); err != nil {
			recoveredErr = recoveredValueToError(err)
		}
	}()

	if graph.graph[formattedNodeName] == nil {
		return false, errors.WithStackTrace(fmt.Errorf("The given name does not show up in the dependency graph: %s", formattedNodeName))
	}

	roots := make(nodeset)
	roots[formattedNodeName] = true

	// Do a reverse search, as we're looking at which nodes depend on the given one
	graphToScan := graph.graph.transpose()
	reachableNodes := graphToScan.reachableFrom(roots)

	// Every node will be reachable from itself and root... So if this list contains anything other than that, return
	// true.
	for reachableNode, isReachable := range reachableNodes {
		if isReachable && (nodePatternsWeCareAboutRegexp.MatchString(reachableNode) && reachableNode != formattedNodeName) {
			return true, nil
		}
	}

	return false, nil
}

// doesFromDependOnTo returns true if the formattedFromName node depends on the formattedToName node. This method
// assumes that formattedFromName and formattedToName are already formatted to the way the 'terraform graph' command
// names nodes, using one of the formatXXX methods below.
func (graph *TerraformGraph) doesFromDependOnTo(formattedFromName string, formattedToName string) (doesDepend bool, recoveredErr error) {
	// The digraph code uses panic, so we try to recover from those panics and return errors as values instead
	defer func() {
		if err := recover(); err != nil {
			recoveredErr = recoveredValueToError(err)
		}
	}()

	if graph.graph[formattedFromName] == nil {
		return false, errors.WithStackTrace(fmt.Errorf("The given name does not show up in the dependency graph: %s", formattedFromName))
	}

	if graph.graph[formattedToName] == nil {
		return false, errors.WithStackTrace(fmt.Errorf("The given name does not show up in the dependency graph: %s", formattedToName))
	}

	roots := make(nodeset)
	roots[formattedFromName] = true

	reachableNodes := graph.graph.reachableFrom(roots)

	for reachableNode, isReachable := range reachableNodes {
		if reachableNode == formattedToName && isReachable {
			return true, nil
		}
	}

	return false, nil
}

// formatModuleNameExpand formats a root module name the way the 'terraform graph' command formats it. Note that
// modules show up in three ways in the graph, one with "(expand)", one with "(close)", and one with no suffix. This
// method returns the "(expand)" version, which seems to be what you want to pass into the digraph code when calling
// 'reverse'.
func formatModuleNameExpand(moduleName string) string {
	return fmt.Sprintf("[root] module.%s (expand)", moduleName)
}

// formatModuleNameClose formats a root module name the way the 'terraform graph' command formats it. Note that
// modules show up in three ways in the graph, one with "(expand)", one with "(close)", and one with no suffix. This
// method returns the "(close)" version, which seems to be the value to look for in the output of the 'digraph reverse'
// command.
func formatModuleNameClose(moduleName string) string {
	return fmt.Sprintf("[root] module.%s (close)", moduleName)
}

// formatModuleNameNoSuffix formats a root module name the way the 'terraform graph' command formats it. Note that
// modules show up in three ways in the graph, one with "(expand)", one with "(close)", and one with no suffix. This
// method returns the no suffix version. This is mostly useful when cleaning up / removing nodes.
func formatModuleNameNoSuffix(moduleName string) string {
	return fmt.Sprintf("[root] module.%s", moduleName)
}

// formatVariableName formats a root input variable name the way the 'terraform graph' command formats it.
func formatVariableName(variableName string) string {
	return fmt.Sprintf("[root] var.%s", variableName)
}

// formatVariableName formats a root output variable name the way the 'terraform graph' command formats it.
func formatOutputName(variableName string) string {
	return fmt.Sprintf("[root] output.%s", variableName)
}

// formatLocalName formats a root local variable name the way the 'terraform graph' command formats it.
func formatLocalName(localName string) string {
	return fmt.Sprintf("[root] local.%s (expand)", localName)
}

// formatDataSourceName formats a root data source name the way the 'terraform graph' command formats it.
func formatDataSourceName(dataSourceType string, dataSourceName string) string {
	return fmt.Sprintf("[root] data.%s.%s (expand)", dataSourceType, dataSourceName)
}

// formatProviderName formats a root provider name the way the 'terraform graph' command formats it.
func formatProviderName(providerName string) string {
	// TODO: this assumes all providers are from hashicorp... We need a way to support custom providers too!
	return fmt.Sprintf(`[root] provider["registry.terraform.io/hashicorp/%s"]`, providerName)
}

func recoveredValueToError(value any) error {
	switch valueWithType := value.(type) {
	case string:
		return errors.WithStackTrace(goerrors.New(valueWithType))
	case error:
		return errors.WithStackTrace(valueWithType)
	default:
		return errors.WithStackTrace(fmt.Errorf("Unknown error occurred: %v", valueWithType))
	}
}
