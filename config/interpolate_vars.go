package config

import (
	"fmt"
	"reflect"

	"github.com/gruntwork-io/terragrunt/util"

//	"github.com/hashicorp/hil"
	hilast "github.com/hashicorp/hil/ast"
	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/mitchellh/reflectwalk"
)

// ResolveTerragruntVariables returns the list of variables
// after reading terragrunt_var_files block from config string
// and parsing all variables found in these files
func (ti *TerragruntInterpolation) ResolveTerragruntVariables(configStr string) (map[string]hilast.Variable, error) {
	files, err := ti.resolveTerragruntVarFiles(configStr)
	if err != nil {
		return map[string]hilast.Variable{}, err
	}
	vars, err := loadVarsFromFiles(files)
	if err != nil {
		return map[string]hilast.Variable{}, err
	}
	return vars, nil
}

func (ti *TerragruntInterpolation) resolveTerragruntVarFiles(configStr string) ([]string, error) {
	retval := []string{}
	t, err := hcl.Parse(configStr)
	if err != nil {
		return retval, err
	}
	list, ok := t.Node.(*ast.ObjectList)
	if !ok {
		return nil, fmt.Errorf("error parsing: file doesn't contain a root object")
	}

	o := list.Filter("terragrunt_var_files")
	if len(o.Items) > 1 {
		return []string{}, fmt.Errorf("error parsing: only 1 terragrunt_var_files block allowed")
	}
	if len(o.Items) == 1 {
		return ti.interpolateVarFiles(o.Items[0])
	}
	return []string{}, nil
}

// interpolateVarFiles returns terragrunt_var_files after all functions inside of the block
// were applied
func (ti *TerragruntInterpolation)interpolateVarFiles(item *ast.ObjectItem) ([]string, error){
	type terragruntVarFileConfig struct {
		TerragruntVarFiles []string `hcl:terragrunt_var_files,omitempty`
	}
	var config terragruntVarFileConfig
	var varFiles []string
	if err := hcl.DecodeObject(&varFiles, item.Val); err != nil {
	   return []string{}, fmt.Errorf("Error reading terraform_var_files: %s", err)
	}
	config.TerragruntVarFiles = varFiles
	// we have uninterpolated config here. Now we walk it
	w := &Walker{Callback: ti.EvalNode, Replace: true}
	err := reflectwalk.Walk(&config, w)
	return config.TerragruntVarFiles, err
}

func loadVarsFromFiles(files []string) (map[string]hilast.Variable, error) {
	retval := map[string]hilast.Variable{}

	for _, f := range files {
		var out map[string]interface{}

		configString, err := util.ReadFileAsString(f)
		if err != nil {
			return nil, err
		}
		if err = hcl.Decode(&out, configString); err != nil {
			return nil, err
		}
		for k, v := range out {
			o, err := NewVariable(v)
			if err == nil {
				varkey := fmt.Sprintf("var.%s", k)
				retval[varkey] = o
			}
		}
	}
	return retval, nil
}

func NewVariable(v interface{}) (result hilast.Variable, err error) {
	switch val := reflect.ValueOf(v); val.Kind() {
	case reflect.String:
		result.Type = hilast.TypeString
	case reflect.Int:
		result.Type = hilast.TypeInt
	case reflect.Map:
		result.Type = hilast.TypeMap
	case reflect.Bool:
		result.Type = hilast.TypeBool
	case reflect.Float32, reflect.Float64:
		result.Type = hilast.TypeFloat
	case reflect.Slice, reflect.Array:
		result.Type = hilast.TypeList
	default:
		err = fmt.Errorf("Uknown type: %s", val.Kind())
	}

	result.Value = v
	return
}
