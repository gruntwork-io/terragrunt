package config

import (
	"fmt"
	"github.com/hashicorp/hil"
	"github.com/hashicorp/hil/ast"
	"github.com/mitchellh/reflectwalk"
	"github.com/hashicorp/hcl"
)

func (ti *TerragruntInterpolation) ResolveTerragruntConfig(configStr string) (*tfvarsFileWithTerragruntConfig, error) {
	config := &tfvarsFileWithTerragruntConfig{}

	if err := hcl.Decode(&config, configStr); err != nil {
		return config, fmt.Errorf( "Error reading config: %v", err)
    }
	w := &Walker{Callback: ti.EvalNode, Replace: true}
	err := reflectwalk.Walk(config, w)
	return config, err
}

func (ti *TerragruntInterpolation)EvalNode(node ast.Node) (interface{}, error) {
	result, err := hil.Eval(node, ti.EvalConfig())
	if err != nil {
		return "", fmt.Errorf("eval error: %v", err)
	}
	return result.Value, nil
}

func (ti *TerragruntInterpolation)EvalConfig() *hil.EvalConfig {
	return &hil.EvalConfig{
		GlobalScope: &ast.BasicScope{
			FuncMap: ti.Funcs(),
			VarMap:  ti.Variables,
		},
	}
}

