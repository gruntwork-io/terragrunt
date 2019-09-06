package config

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/hashicorp/hcl2/hcl"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)
import "github.com/hashicorp/terraform/dag"

const locals = "locals"
const local = "local"
const globals = "globals"
const global = "global"
const include = "include"
const path = "path"

type RootVertex struct {
}

type VariableVertex struct {
	Type string
	Name string
	Expr hcl.Expression
}

// basicEdge is a basic implementation of Edge that has the source and
// target vertex.
type BasicEdge struct {
	S, T dag.Vertex
}

func (e *BasicEdge) Hashcode() interface{} {
	return fmt.Sprintf("%p-%p", e.S, e.T)
}

func (e *BasicEdge) Source() dag.Vertex {
	return e.S
}

func (e *BasicEdge) Target() dag.Vertex {
	return e.T
}

func getValuesFromHclFile(file *hcl.File) map[string]map[string]cty.Value {
	localsBlock, globalsBlock, includeBlock, _ := getBlocks(file)
	graph := dag.AcyclicGraph{}
	root := RootVertex{}

	verticies := map[string]map[string]VariableVertex{
		local: {},
		global: {},
		include: {},
	}

	// Add root vertex
	graph.Add(root)

	// TODO: diagnostics
	_ = addVerticies(graph, verticies[local], local, localsBlock)
	_ = addVerticies(graph, verticies[global], global, globalsBlock)
	_ = addVerticies(graph, verticies[include], include, includeBlock)

	// TODO validate include
	_ = addEdges(graph, root, verticies, local, localsBlock)
	_ = addEdges(graph, root, verticies, global, globalsBlock)
	_ = addEdges(graph, root, verticies, include, includeBlock)

	// TODO: diagnostics


	// TODO: validate that includes only depend on locals

	values := map[string]map[string]cty.Value{
		local: {},
		global: {},
		include: {},
	}

	err := graph.Validate()
	if err != nil {
		panic(err)
	}

	graph.TransitiveReduction()

	diags := hcl.Diagnostics{}
	walkBreadthFirst(&graph, root, func(v dag.Vertex) (shouldContinue bool) {

		if _, isRoot := v.(RootVertex); isRoot {
			return true
		}

		vertex, ok := v.(VariableVertex)
		if !ok {
			// TODO: diags
			return false
		}

		valuesCty, err := convertValuesToVariables(values)
		if err != nil {
			// TODO
			return false
		}

		ctx := hcl.EvalContext{
			Variables: *valuesCty,
		}

		value, currentDiags := vertex.Expr.Value(&ctx)
		if currentDiags != nil && currentDiags.HasErrors() {
			diags = diags.Extend(currentDiags)
			return false
		}

		switch vertex.Type {
		case local, global:
			values[vertex.Type][vertex.Name] = value
		case include:
			values[include] = map[string]cty.Value{
				"relative": cty.StringVal("relative/path"),
			}
		default:
			// TODO
			return false
		}

		return true
	})

	return values
}

func getBlocks(file *hcl.File) (hcl.Body, hcl.Body, hcl.Body, hcl.Diagnostics) {
	blocksSchema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "locals"},
			{Type: "globals"},
			{Type: "include"},
		},
	}

	// TODO: err, diags
	parsedBlocks, _, diags := file.Body.PartialContent(blocksSchema)
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

	return blocksByType[locals][0].Body, blocksByType[globals][0].Body, blocksByType[include][0].Body, diags
}

func addVerticies(graph dag.AcyclicGraph, verticies map[string]VariableVertex, typ string, block hcl.Body) hcl.Diagnostics {
	attrs, diags := block.JustAttributes()
	if diags != nil && diags.HasErrors() {
		return diags
	}

	for name, attr := range attrs {
		vertex := VariableVertex{
			Type: typ,
			Name: name,
			Expr: attr.Expr,
		}
		verticies[name] = vertex
		graph.Add(vertex)
	}

	return nil
}

func addEdges(graph dag.AcyclicGraph, root RootVertex, verticies map[string]map[string]VariableVertex, typ string, block hcl.Body) hcl.Diagnostics {
	attrs, _ := block.JustAttributes()
	for targetName := range attrs {
		target := verticies[typ][targetName]
		variables := target.Expr.Variables()

		if variables == nil || len(variables) <= 0 {
			graph.Connect(&BasicEdge{
				S: root,
				T: target,
			})
			continue
		}

		for _, variable := range variables {
			// TODO: validation
			sourceType := variable.RootName()
			switch sourceType {
			case local, global:
				sourceName := variable[1].(hcl.TraverseAttr).Name
				source, exists := verticies[sourceType][sourceName]
				if !exists {
					// TODO
					return hcl.Diagnostics{}
				}

				graph.Connect(&BasicEdge{
					S: source,
					T: target,
				})
			case include:
				// TODO validate options
				graph.Connect(&BasicEdge{
					S: verticies[include][path],
					T: target,
				})
			default:
				// TODO
				return hcl.Diagnostics{}
			}
		}
	}

	return nil
}

func walkBreadthFirst(g *dag.AcyclicGraph, root dag.Vertex, cb func(vertex dag.Vertex) (shouldContinue bool)) {
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

func convertValuesToVariables(values map[string]map[string]cty.Value) (*map[string]cty.Value, error) {
	variables := map[string]cty.Value{}
	for k, v := range values {
		variable, err := gocty.ToCtyValue(v, generateTypeFromMap(v))
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}

		variables[k] = variable
	}

	return &variables, nil
}

func generateTypeFromMap(value map[string]cty.Value) cty.Type {
	typeMap := map[string]cty.Type{}
	for k, v := range value {
		typeMap[k] = v.Type()
	}
	return cty.Object(typeMap)
}
