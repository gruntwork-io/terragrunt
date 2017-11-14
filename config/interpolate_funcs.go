package config

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hil/ast"
	"os"
	"path/filepath"
	"strings"
)

func (ti *TerragruntInterpolation) Funcs() map[string]ast.Function {
	return map[string]ast.Function{
		"find_in_parent_folders":                ti.interpolateFindInParentFolders(),
		"path_relative_to_include":              ti.interpolatePathRelativeToInclude(),
		"path_relative_from_include":            ti.interpolatePathRelativeFromInclude(),
		"get_env":                               ti.interpolateGetEnv(),
		"get_tfvars_dir":                        ti.interpolateGetTfVarsDir(),
		"get_parent_tfvars_dir":                 ti.interpolateGetParentTfVarsDir(),
		"get_aws_account_id":                    ti.interpolateGetAWSAccountID(),
		"prepend_list":                          ti.interpolatePrependList(),
		"import_parent_tree":                    ti.interpolateImportParentTree(),
		"find_all_in_parent_folders":            ti.interpolateFindAllInParentFolders(),
		"get_terraform_commands_that_need_vars": ti.interpolateGetTerraformCommandsThatNeedVars(),
	}
}

func (ti *TerragruntInterpolation) interpolateFindInParentFolders() ast.Function {
	return ast.Function{
		ArgTypes:     []ast.Type{},
		ReturnType:   ast.TypeString,
		Variadic:     true,
		VariadicType: ast.TypeString,
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
func (ti *TerragruntInterpolation) interpolatePathRelativeToInclude() ast.Function {
	return ast.Function{
		ArgTypes:   []ast.Type{},
		ReturnType: ast.TypeString,
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

func (ti *TerragruntInterpolation) interpolateGetTerraformCommandsThatNeedVars() ast.Function {
	return ast.Function{
		ArgTypes:   []ast.Type{},
		ReturnType: ast.TypeList,
		Variadic:   false,
		Callback: func(args []interface{}) (interface{}, error) {
			return stringSliceToVariableValue(TERRAFORM_COMMANDS_NEED_VARS), nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolateImportParentTree() ast.Function {
	return ast.Function{
		ArgTypes:   []ast.Type{ast.TypeString},
		ReturnType: ast.TypeList,
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

			for i := 0; i < ti.Options.MaxFoldersToCheck; i++ {
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

func (ti *TerragruntInterpolation) interpolateFindAllInParentFolders() ast.Function {
	return ast.Function{
		ArgTypes:   []ast.Type{ast.TypeString},
		ReturnType: ast.TypeList,
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

			for i := 0; i < ti.Options.MaxFoldersToCheck; i++ {
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

func (ti *TerragruntInterpolation) interpolatePathRelativeFromInclude() ast.Function {
	return ast.Function{
		ArgTypes:     []ast.Type{},
		ReturnType:   ast.TypeString,
		Variadic:     true,
		VariadicType: ast.TypeString,
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

			return util.GetPathRelativeTo(includePath, currentPath)
		},
	}
}

func (ti *TerragruntInterpolation) interpolateGetEnv() ast.Function {
	return ast.Function{
		ArgTypes:     []ast.Type{ast.TypeString},
		ReturnType:   ast.TypeString,
		Variadic:     true,
		VariadicType: ast.TypeString,
		Callback: func(args []interface{}) (interface{}, error) {
			envParam := args[0].(string)
			result := os.Getenv(envParam)
			if result == "" {
				return "", fmt.Errorf("environment variable %s not set", envParam)
			}
			return envParam, nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolateGetTfVarsDir() ast.Function {
	return ast.Function{
		ArgTypes:     []ast.Type{},
		ReturnType:   ast.TypeString,
		Variadic:     false,
		VariadicType: ast.TypeString,
		Callback: func(args []interface{}) (interface{}, error) {
			if ti.Options == nil {
				return "", fmt.Errorf("terragrunt options not set")
			}
			terragruntConfigFileAbsPath, err := filepath.Abs(ti.Options.TerragruntConfigPath)
			if err != nil {
				return "", errors.WithStackTrace(err)
			}
			return filepath.ToSlash(filepath.Dir(terragruntConfigFileAbsPath)), nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolateGetParentTfVarsDir() ast.Function {
	return ast.Function{
		ArgTypes:   []ast.Type{},
		ReturnType: ast.TypeString,
		Variadic:   false,
		Callback: func(args []interface{}) (interface{}, error) {
			parentPath, err := ti.pathRelativeFromInclude()
			if err != nil {
				return "", errors.WithStackTrace(err)
			}

			currentPath := filepath.Dir(ti.Options.TerragruntConfigPath)
			parentPath, err = filepath.Abs(filepath.Join(currentPath, parentPath))
			if err != nil {
				return "", errors.WithStackTrace(err)
			}
			return filepath.ToSlash(parentPath), nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolateGetAWSAccountID() ast.Function {
	return ast.Function{
		ArgTypes:     []ast.Type{},
		ReturnType:   ast.TypeString,
		Variadic:     true,
		VariadicType: ast.TypeString,
		Callback: func(args []interface{}) (interface{}, error) {
			sess, err := session.NewSession()
			if err != nil {
				return "", errors.WithStackTrace(err)
			}

			identity, err := sts.New(sess).GetCallerIdentity(nil)
			if err != nil {
				return "", errors.WithStackTrace(err)
			}

			return *identity.Account, nil
		},
	}
}

func (ti *TerragruntInterpolation) interpolatePrependList() ast.Function {
	return ast.Function{
		ArgTypes:     []ast.Type{ast.TypeString, ast.TypeList},
		ReturnType:   ast.TypeString,
		Variadic:     true,
		VariadicType: ast.TypeList,
		Callback: func(args []interface{}) (interface{}, error) {
			var retval []string
			prefix := args[0].(string)
			list := args[1].([]string)

			for _, i := range list {
				retval = append(retval, prefix+string(i[1]))
			}
			return stringSliceToVariableValue(retval), nil
		},
	}
}

// stringSliceToVariableValue converts a string slice into the value
// required to be returned from interpolation functions which return
// TypeList. Borrowed from hashicorp/terraform
func stringSliceToVariableValue(values []string) []ast.Variable {
	output := make([]ast.Variable, len(values))
	for index, value := range values {
		output[index] = ast.Variable{
			Type:  ast.TypeString,
			Value: value,
		}
	}
	return output
}
