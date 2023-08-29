package terraform

import (
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// Prefix to use for terraform variables set with environment variables.
const TFVarPrefix = "TF_VAR"

// ModuleVariables will return all the variables defined in the downloaded terraform modules, taking into
// account all the generated sources. This function will return the required and optional variables separately.
func ModuleVariables(modulePath string) ([]string, []string, error) {
	module, diags := tfconfig.LoadModule(modulePath)
	if diags.HasErrors() {
		return nil, nil, errors.WithStackTrace(diags)
	}

	required := []string{}
	optional := []string{}
	for _, variable := range module.Variables {
		if variable.Required {
			required = append(required, variable.Name)
		} else {
			optional = append(optional, variable.Name)
		}
	}
	return required, optional, nil
}
