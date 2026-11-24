package config

import (
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

const (
	commandsAttrAcceptVars        = "accept_vars"
	commandsAttrAcceptLocking     = "accept_locking"
	commandsAttrAcceptInput       = "accept_input"
	commandsAttrAcceptParallelism = "accept_parallelism"

	commandsExperimentHint = " The `commands` variable needs the " + experiment.CommandsVariable +
		" experiment. Enable it with --experiment " + experiment.CommandsVariable +
		" or TG_EXPERIMENT=" + experiment.CommandsVariable + "."
)

var commandsExperimentHintOption = hclparse.WithDiagnosticsHandler(commandsExperimentDiagnostics)

var commandsVariable = sync.OnceValue(func() cty.Value {
	return cty.ObjectVal(map[string]cty.Value{
		commandsAttrAcceptVars:        stringListVal(TerraformCommandsNeedVars),
		commandsAttrAcceptLocking:     stringListVal(TerraformCommandsNeedLocking),
		commandsAttrAcceptInput:       stringListVal(TerraformCommandsNeedInput),
		commandsAttrAcceptParallelism: stringListVal(TerraformCommandsNeedParallelism),
	})
})

func commandsExperimentDiagnostics(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
	if !slices.ContainsFunc(diags, needsCommandsExperimentHint) {
		return diags, nil
	}

	hinted := make(hcl.Diagnostics, 0, len(diags))

	for _, diag := range diags {
		if !needsCommandsExperimentHint(diag) {
			hinted = append(hinted, diag)

			continue
		}

		withHint := *diag
		withHint.Detail += commandsExperimentHint
		hinted = append(hinted, &withHint)
	}

	return hinted, nil
}

func withCommandsExperimentHint(experiments experiment.Experiments, err error) error {
	var diags hcl.Diagnostics

	if experiments.Evaluate(experiment.CommandsVariable) || !errors.As(err, &diags) {
		return err
	}

	for _, diag := range diags {
		if needsCommandsExperimentHint(diag) {
			diag.Detail += commandsExperimentHint
		}
	}

	return err
}

func needsCommandsExperimentHint(diag *hcl.Diagnostic) bool {
	return referencesUnknownCommands(diag) && !strings.HasSuffix(diag.Detail, commandsExperimentHint)
}

func referencesUnknownCommands(diag *hcl.Diagnostic) bool {
	if diag.Expression == nil || diag.EvalContext == nil {
		return false
	}

	traversal, diags := hcl.AbsTraversalForExpr(diag.Expression)
	if diags.HasErrors() || traversal.RootName() != MetadataCommands {
		return false
	}

	for scope := diag.EvalContext; scope != nil; scope = scope.Parent() {
		if _, defined := scope.Variables[MetadataCommands]; defined {
			return false
		}
	}

	return true
}
