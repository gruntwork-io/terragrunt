package config

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	"path/filepath"
)
import "github.com/hashicorp/terraform/dag"

const local = "local"
const global = "global"
const include = "include"

type rootVertex struct {
}

type variableVertex struct {
	Evaluator *configEvaluator
	Type   string
	Name   string
	Expr   hcl.Expression
	Evaluated bool
}

// basicEdge is a basic implementation of Edge that has the source and
// target vertex.
type basicEdge struct {
	S, T dag.Vertex
}

func (e *basicEdge) Hashcode() interface{} {
	return fmt.Sprintf("%p-%p", e.S, e.T)
}

func (e *basicEdge) Source() dag.Vertex {
	return e.S
}

func (e *basicEdge) Target() dag.Vertex {
	return e.T
}

type evaluatorGlobals struct {
	options  *options.TerragruntOptions
	parser   *hclparse.Parser
	graph    dag.AcyclicGraph
	root     rootVertex
	vertices map[string]variableVertex
	values   map[string]cty.Value
}

type configEvaluator struct {
	globals evaluatorGlobals

	configPath   string
	configFile   *hcl.File

	localVertices  map[string]variableVertex
	localValues    map[string]cty.Value
	includeVertex  *variableVertex
	includePath    *string
	includeValue   *map[string]cty.Value
}

type EvaluationResult struct {
	ConfigFile *hcl.File
	Variables  map[string]cty.Value
}

func newConfigEvaluator(configPath string, globals evaluatorGlobals, includeValue *map[string]cty.Value) *configEvaluator {
	eval := configEvaluator{}
	eval.globals = globals
	eval.configPath = configPath

	eval.localVertices = map[string]variableVertex{}
	eval.localValues = map[string]cty.Value{}

	eval.includeVertex = nil
	eval.includeValue = includeValue

	return &eval
}

// Evaluation Steps:
// 1. Parse child HCL, extract locals, globals, and include
// 2. Add vertices for child locals, globals, and include
// 3. Add edges for child variables based on interpolations used
//     a. When encountering globals that aren't defined in this config, create a vertex for them with an empty expression
// 4. Verify DAG and reduce graph
//     a. Verify no globals are used in path to include (if exists)
// 5. Evaluate everything except globals
// 6. If include exists, find parent HCL, parse, and extract locals and globals
// 7. Add vertices for parent locals
// 8. Add vertices for parent globals that don't already exist, or add expressions to empty globals
// 9. Verify and reduce graph
//     a. Verify that there are no globals that are empty.
// 10. Evaluate everything, skipping things that were evaluated in (5)
func ParseConfigVariables(filename string, terragruntOptions *options.TerragruntOptions) (*EvaluationResult, *EvaluationResult, error) {
	globals := evaluatorGlobals{
		options:  terragruntOptions,
		parser:   hclparse.NewParser(),
		graph:    dag.AcyclicGraph{},
		root:     rootVertex{},
		vertices: map[string]variableVertex{},
		values:   map[string]cty.Value{},
	}

	// Add root of graph
	globals.graph.Add(globals.root)

	child := *newConfigEvaluator(filename, globals, nil)
	var childResult *EvaluationResult = nil

	// 1, 2, 3, 4
	err := child.parseConfig()
	if err != nil {
		return nil, nil, err
	}

	// 5
	diags := globals.evaluateVariables(false)
	if diags != nil {
		return nil, nil, diags
	}

	var parent *configEvaluator = nil
	var parentResult *EvaluationResult = nil
	if child.includePath != nil {
		// 6, 7, 8, 9
		parent = newConfigEvaluator(*child.includePath, globals, child.includeValue)
		err = (*parent).parseConfig()
		if err != nil {
			return nil, nil, err
		}
	}

	// 10
	diags = globals.evaluateVariables(true)
	if diags != nil {
		return nil, nil, diags
	}

	childResult, err = child.toResult()
	if err != nil {
		return nil, nil, err
	}
	if parent != nil {
		parentResult, err = parent.toResult()
		if err != nil {
			return nil, nil, err
		}
	}

	return childResult, parentResult, nil
}

func (eval *configEvaluator) parseConfig() error {
	configString, err := util.ReadFileAsString(eval.configPath)
	if err != nil {
		return err
	}

	configFile, err := parseHcl(eval.globals.parser, configString, eval.configPath)
	if err != nil {
		return err
	}

	eval.configFile = configFile

	localsBlock, globalsBlock, includeBlock, diags := eval.getBlocks(configFile)
	if diags != nil && diags.HasErrors() {
		return diags
	}

	var addedVertices []variableVertex
	err = eval.addVertices(local, localsBlock, func(vertex variableVertex) error {
		eval.localVertices[vertex.Name] = vertex
		addedVertices = append(addedVertices, vertex)
		return nil
	})
	if err != nil {
		return err
	}
	if includeBlock != nil {
		err = eval.addVertices(include, includeBlock, func(vertex variableVertex) error {
			// TODO: validate include name and ensure only setting this once
			eval.includeVertex = &vertex
			addedVertices = append(addedVertices, vertex)
			return nil
		})
	}
	if err != nil {
		return err
	}
	err = eval.addVertices(global, globalsBlock, func(vertex variableVertex) error {
		eval.globals.vertices[vertex.Name] = vertex
		addedVertices = append(addedVertices, vertex)
		return nil
	})
	if err != nil {
		return err
	}

	// TODO validate include

	err = eval.addAllEdges(eval.localVertices)
	if err != nil {
		return err
	}
	if eval.includeVertex != nil {
		err = eval.addEdges(*eval.includeVertex)
		if err != nil {
			return err
		}
	}
	err = eval.addAllEdges(eval.globals.vertices)
	if err != nil {
		return err
	}

	// TODO: validate that includes only depend on locals

	err = eval.globals.graph.Validate()
	if err != nil {
		return err
	}

	eval.globals.graph.TransitiveReduction()

	return nil
}

func (eval *configEvaluator) evaluateVariable(vertex variableVertex, diags hcl.Diagnostics, evaluateGlobals bool) bool {
	if vertex.Type == global && !evaluateGlobals {
		return false
	}

	if vertex.Evaluated {
		return true
	}

	valuesCty, err := eval.convertValuesToVariables()
	if err != nil {
		// TODO: diags.Extend(??)
		return false
	}

	ctx := hcl.EvalContext{
		Variables: valuesCty,
	}

	value, currentDiags := vertex.Expr.Value(&ctx)
	if currentDiags != nil && currentDiags.HasErrors() {
		_ = diags.Extend(currentDiags)
		return false
	}

	vertex.Evaluated = true

	switch vertex.Type {
	case global:
		eval.globals.values[vertex.Name] = value

	case local:
		eval.localValues[vertex.Name] = value
	case include:
		includePath, includeValue, err := eval.evaluateInclude(value)
		if err != nil {
			// TODO: diags.Extend(??)
			return false
		}

		eval.includePath = &includePath
		eval.includeValue = &includeValue
	default:
		// TODO: diags.Extend(??)
		return false
	}

	return true
}

func (eval *configEvaluator) evaluateInclude(value cty.Value) (string, map[string]cty.Value, error) {
	// TODO: validate this is a string?
	includePath := value.AsString()
	configPath := eval.configPath

	if includePath == "" {
		return "", nil, IncludedConfigMissingPath(configPath)
	}

	childConfigPathAbs, err := filepath.Abs(configPath)
	if err != nil {
		return "", nil, err
	}

	if !filepath.IsAbs(includePath) {
		includePath = util.JoinPath(filepath.Dir(childConfigPathAbs), includePath)
	}

	relative, err := util.GetPathRelativeTo(filepath.Dir(configPath), filepath.Dir(includePath))
	if err != nil {
		return "", nil, err
	}

	includeValue := map[string]cty.Value{
		"parent": cty.StringVal(filepath.ToSlash(filepath.Dir(includePath))),
		"relative": cty.StringVal(relative),
	}

	return includePath, includeValue, nil
}

func (eval *configEvaluator) getBlocks(file *hcl.File) (hcl.Body, hcl.Body, hcl.Body, hcl.Diagnostics) {
	const locals = "locals"
	const globals = "globals"

	blocksSchema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: locals},
			{Type: globals},
			{Type: include},
		},
	}

	parsedBlocks, _, diags := file.Body.PartialContent(blocksSchema)
	if diags != nil && diags.HasErrors() {
		return nil, nil, nil, diags
	}
	blocksByType := map[string][]*hcl.Block{}

	for _, block := range parsedBlocks.Blocks {
		if block.Type == locals || block.Type == globals || block.Type == include {
			blocks := blocksByType[block.Type]
			if blocks == nil {
				blocks = []*hcl.Block{}
			}

			blocksByType[block.Type] = append(blocks, block)
		}
	}

	// TODO: validate blocks


	if _, exists := blocksByType[include]; exists {
		return blocksByType[locals][0].Body, blocksByType[globals][0].Body, blocksByType[include][0].Body, diags
	} else {
		return blocksByType[locals][0].Body, blocksByType[globals][0].Body, nil, diags
	}
}

func (eval *configEvaluator) addVertices(vertexType string, block hcl.Body, consumer func(vertex variableVertex) error) error {
	attrs, diags := block.JustAttributes()
	if diags != nil && diags.HasErrors() {
		return diags
	}

	for name, attr := range attrs {
		var vertex *variableVertex = nil

		if vertexType == global {
			globalVertex, exists := eval.globals.vertices[name]
			if exists && globalVertex.Expr == nil {
				// This was referenced by a child but not overridden there
				vertex = &globalVertex
				globalVertex.Evaluator = eval
				globalVertex.Expr = attr.Expr
			}
		}

		if vertex == nil {
			vertex = &variableVertex{
				Evaluator: eval,
				Type: vertexType,
				Name: name,
				Expr: attr.Expr,
				Evaluated: false,
			}
		}

		eval.globals.graph.Add(*vertex)
		err := consumer(*vertex)
		if err != nil {
			return err
		}
	}

	return nil
}

func (eval *configEvaluator) addAllEdges(vertices map[string]variableVertex) error {
	for _, vertex := range vertices {
		err := eval.addEdges(vertex)
		if err != nil {
			return err
		}
	}

	return nil
}

func (eval *configEvaluator) addEdges(target variableVertex) error {
	if target.Expr == nil {
		return nil
	}

	variables := target.Expr.Variables()

	if variables == nil || len(variables) <= 0 {
		eval.globals.graph.Connect(&basicEdge{
			S: eval.globals.root,
			T: target,
		})
		return nil
	}

	for _, variable := range variables {
		sourceType, sourceName, err := getVariableRootAndName(variable)
		if err != nil {
			return err
		}

		switch sourceType {
		case global:
			source, exists := eval.globals.vertices[sourceName]
			if !exists {
				// Could come from parent context, add empty node for now.
				source = variableVertex{
					Evaluator: nil,
					Type: global,
					Name: sourceName,
					Expr: nil,
					Evaluated: false,
				}
			}
			eval.globals.graph.Connect(&basicEdge{
				S: source,
				T: target,
			})
		case local:
			source, exists := eval.localVertices[sourceName]
			if !exists {
				// TODO: error
				return nil
			}

			eval.globals.graph.Connect(&basicEdge{
				S: source,
				T: target,
			})
		case include:
			// TODO validate options
			eval.globals.graph.Connect(&basicEdge{
				S: eval.includeVertex,
				T: target,
			})
		default:
			// TODO: error
			return nil
		}
	}

	return nil
}

func getVariableRootAndName(variable hcl.Traversal) (string, string, error) {
	// TODO: validation
	sourceType := variable.RootName()
	sourceName := variable[1].(hcl.TraverseAttr).Name
	return sourceType, sourceName, nil
}

func (eval *configEvaluator) convertValuesToVariables() (map[string]cty.Value, error) {
	values := map[string]map[string]cty.Value{
		local: eval.localValues,
		global: eval.globals.values,
	}

	if eval.includeValue != nil {
		values[include] = *eval.includeValue
	}

	variables := map[string]cty.Value{}
	for k, v := range values {
		variable, err := gocty.ToCtyValue(v, generateTypeFromMap(v))
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}

		variables[k] = variable
	}

	return variables, nil
}

func (eval *configEvaluator) toResult() (*EvaluationResult, error) {
	variables, err := eval.convertValuesToVariables()
	if err != nil {
		return nil, err
	}

	return &EvaluationResult{
		ConfigFile: eval.configFile,
		Variables:  variables,
	}, nil
}

func (globals *evaluatorGlobals) evaluateVariables(evaluateGlobals bool) hcl.Diagnostics {
	diags := hcl.Diagnostics{}

	walkBreadthFirst(globals.graph, globals.root, func(v dag.Vertex) (shouldContinue bool) {
		if _, isRoot := v.(rootVertex); isRoot {
			return true
		}

		vertex, ok := v.(variableVertex)
		if !ok {
			// TODO: diags.Extend(??)
			return false
		}

		return vertex.Evaluator.evaluateVariable(vertex, diags, evaluateGlobals)
	})

	if diags.HasErrors() {
		return diags
	}

	return nil
}

func walkBreadthFirst(g dag.AcyclicGraph, root dag.Vertex, cb func(vertex dag.Vertex) (shouldContinue bool)) {
	visited := map[dag.Vertex]struct{}{}
	queue := []dag.Vertex{root}

	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:] // pop

		if _, contained := visited[v]; !contained {
			visited[v] = struct{}{}
			shouldContinue := cb(v)

			if shouldContinue {
				for _, child := range g.DownEdges(v).List() {
					queue = append(queue, child)
				}
			}
		}
	}
}

func generateTypeFromMap(value map[string]cty.Value) cty.Type {
	typeMap := map[string]cty.Type{}
	for k, v := range value {
		typeMap[k] = v.Type()
	}
	return cty.Object(typeMap)
}
