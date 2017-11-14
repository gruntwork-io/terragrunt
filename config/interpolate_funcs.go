package config

import (
	"path/filepath"
	"strings"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	hilast "github.com/hashicorp/hil/ast"
)

func (ti *TerragruntInterpolation) Funcs() map[string]hilast.Function {
	return map[string]hilast.Function{
		"find_in_parent_folders":   ti.interpolateFindInParentFolders(),
		"path_relative_to_include": ti.interpolatePathRelativeToInclude(),
		/*	"path_relative_from_include": pathRelativeFromInclude(),
			"get_env":                    getEnv(),
			"get_tfvars_dir":             getTfVarsDir(),
			"get_parent_tfvars_dir":      getParentTfVarsDir(),
			"get_aws_account_id":         getAWSAccountID(),
			"import_parent_tree":         importParentTree(),
			"prepend":                    prepend(),*/
		"import_parent_tree":                    ti.interpolateImportParentTree(),
		"find_all_in_parent_folders": ti.interpolateFindAllInParentFolders(),
		"get_terraform_commands_that_need_vars": ti.interpolateGetTerraformCommandsThatNeedVars(),
	}
}

func (ti *TerragruntInterpolation) interpolateFindInParentFolders() hilast.Function {
	return hilast.Function{
		ArgTypes:     []hilast.Type{},
		ReturnType:   hilast.TypeString,
		Variadic:     true,
		VariadicType: hilast.TypeString,
		Callback: func(args []interface{}) (interface{}, error) {
			fileToFindParam := DefaultTerragruntConfigPath
			fallbackParam := ""

			if len(args) > 0 {
				fileToFindParam = args[0].(string)
				if fileToFindParam == "" {
					return "", errors.WithStackTrace(EmptyStringNotAllowed("parameter to the find_in_parent_folders_function"))
				}
			}
			if len(args) == 2 {
				fallbackParam = args[1].(string)
			}

			currentPath, _ := filepath.Abs(filepath.Dir(ti.Options.TerragruntConfigPath))
			pathParts := strings.Split(currentPath, string(filepath.Separator))

			length := len(pathParts) - 1 // start from ".."

			for length > 0 {
				dir := filepath.Join(pathParts[:length]...)
				fileToFind := filepath.Join(string(filepath.Separator), dir, fileToFindParam)
				if util.FileExists(fileToFind) {
					return util.GetPathRelativeTo(fileToFind, filepath.Dir(ti.Options.TerragruntConfigPath))
				}
				length = length - 1
			}

			if fallbackParam != "" {
				return fallbackParam, nil
			}

			return "", errors.WithStackTrace(ParentFileNotFound{Path: ti.Options.TerragruntConfigPath, File: fileToFindParam})
		},
	}
}

// Return the relative path between the included Terragrunt configuration file and the current Terragrunt configuration
// file
func (ti *TerragruntInterpolation) interpolatePathRelativeToInclude() hilast.Function {
	return hilast.Function{
		ArgTypes:   []hilast.Type{},
		ReturnType: hilast.TypeString,
		Variadic:   false,
		Callback: func(args []interface{}) (interface{}, error) {

			if ti.include == nil {
				return ".", nil
			}

			includedConfigPath, err := ResolveTerragruntConfigString(ti.include.Path, ti.include, ti.Options)
			if err != nil {
				return "", errors.WithStackTrace(err)
			}

			includePath := filepath.Dir(includedConfigPath)
			currentPath := filepath.Dir(ti.Options.TerragruntConfigPath)

			if !filepath.IsAbs(includePath) {
				includePath = util.JoinPath(currentPath, includePath)
			}

			return util.GetPathRelativeTo(currentPath, includePath)
		},
	}
}

func (ti *TerragruntInterpolation) interpolateGetTerraformCommandsThatNeedVars() hilast.Function {
	return hilast.Function{
		ArgTypes:   []hilast.Type{},
		ReturnType: hilast.TypeList,
		Variadic:   false,
		Callback: func(args []interface{}) (interface{}, error) {
			return stringSliceToVariableValue(TERRAFORM_COMMANDS_NEED_VARS), nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolateImportParentTree() hilast.Function {
	return hilast.Function{
		ArgTypes:   []hilast.Type{hilast.TypeString},
		ReturnType: hilast.TypeList,
		Variadic:   false,
		Callback: func(args []interface{}) (interface{}, error) {
			fileglob := args[0].(string)
			retval := []string{}

			if fileglob == "" {
				return stringSliceToVariableValue(retval), errors.WithStackTrace(EmptyStringNotAllowed("import_parent_tree"))
			}

			if ti.Options == nil {
				return stringSliceToVariableValue(retval), errors.WithStackTrace(EmptyStringNotAllowed("import_parent_tree"))
			}

			previousDir, err := filepath.Abs(filepath.Dir(ti.Options.TerragruntConfigPath))
			previousDir = filepath.ToSlash(previousDir)

			if err != nil {
				return stringSliceToVariableValue(retval), errors.WithStackTrace(err)
			}

			for i := 0; i < MAX_PARENT_FOLDERS_TO_CHECK; i++ {
				currentDir := filepath.ToSlash(filepath.Dir(previousDir))
				if currentDir == previousDir {
					return stringSliceToVariableValue(retval), nil
				}
				pathglob := filepath.Join(currentDir, fileglob)
				matches, _ := filepath.Glob(pathglob)

				if len(matches) > 0 {
					prefixed := util.PrefixListItems("-var-file=", matches)
					// Variables imported from higher level directories have lower precedence
					retval = append(prefixed, retval...)
				}
				previousDir = currentDir
			}
			return stringSliceToVariableValue(retval), nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolateFindAllInParentFolders() hilast.Function {
	return hilast.Function{
		ArgTypes:   []hilast.Type{hilast.TypeString},
		ReturnType: hilast.TypeList,
		Variadic:   false,
		Callback: func(args []interface{}) (interface{}, error) {
			fileglob := args[0].(string)
			retval := []string{}

			if fileglob == "" {
				return stringSliceToVariableValue(retval), errors.WithStackTrace(EmptyStringNotAllowed("import_parent_tree"))
			}

			if ti.Options == nil {
				return stringSliceToVariableValue(retval), errors.WithStackTrace(EmptyStringNotAllowed("import_parent_tree"))
			}

			previousDir, err := filepath.Abs(filepath.Dir(ti.Options.TerragruntConfigPath))
			previousDir = filepath.ToSlash(previousDir)

			if err != nil {
				return stringSliceToVariableValue(retval), errors.WithStackTrace(err)
			}

			for i := 0; i < MAX_PARENT_FOLDERS_TO_CHECK; i++ {
				currentDir := filepath.ToSlash(filepath.Dir(previousDir))
				if currentDir == previousDir {
					return stringSliceToVariableValue(retval), nil
				}
				pathglob := filepath.Join(currentDir, fileglob)
				matches, _ := filepath.Glob(pathglob)

				if len(matches) > 0 {
					retval = append(matches, retval...)
				}
				previousDir = currentDir
			}
			return stringSliceToVariableValue(retval), nil
		},
	}
}

// stringSliceToVariableValue converts a string slice into the value
// required to be returned from interpolation functions which return
// TypeList. Borrowed from hashicorp/terraform
func stringSliceToVariableValue(values []string) []hilast.Variable {
	output := make([]hilast.Variable, len(values))
	for index, value := range values {
		output[index] = hilast.Variable{
			Type:  hilast.TypeString,
			Value: value,
		}
	}
	return output
}
