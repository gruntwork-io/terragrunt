package controls

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/exp/slices"
)

const (
	GlobalEnvVarsCategoryName     = "Global env vars"
	CommandEnvVarsCategoryNameFmt = "`%s` command env vars"
)

var _ = strict.Control(new(DeprecatedEnvVar))

type DeprecatedEnvVar struct {
	*Control
	ErrorFmt   string
	WarningFmt string

	depreacedFlag cli.Flag
	newFlag       cli.Flag
}

func NewDeprecatedEnvVar(depreacedFlag, newFlag cli.Flag, newValue string) *DeprecatedEnvVar {
	var (
		depreacedName = util.FirstElement(util.RemoveEmptyElements(depreacedFlag.GetEnvVars()))
		newName       = util.FirstElement(util.RemoveEmptyElements(newFlag.GetEnvVars()))
	)

	if newValue != "" {
		newName += "=" + newValue
	}

	return &DeprecatedEnvVar{
		Control: &Control{
			Name:        depreacedName,
			Description: "replaced with: " + newName,
		},
		ErrorFmt:   "`%s` env var is no longer supported. Use `%s` instead.",
		WarningFmt: "`%s` env var is deprecated and will be removed in a future version. Use `%s` instead.",

		depreacedFlag: depreacedFlag,
		newFlag:       newFlag,
	}
}

func (ctrl *DeprecatedEnvVar) Evaluate(ctx context.Context) error {
	var (
		valueName = ctrl.depreacedFlag.Value().GetName()
		envName   string
	)

	if valueName == "" || !ctrl.depreacedFlag.Value().IsEnvSet() || slices.Contains(ctrl.newFlag.GetEnvVars(), valueName) {
		return nil
	}

	if names := ctrl.newFlag.GetEnvVars(); len(names) > 0 {
		envName = names[0]

		value := ctrl.newFlag.Value().String()

		if value == "" {
			value = ctrl.depreacedFlag.Value().String()
		}

		envName += "=" + value
	}

	if ctrl.Enabled {
		if ctrl.Status != strict.ActiveStatus || ctrl.ErrorFmt == "" {
			return nil
		}

		return errors.Errorf(ctrl.ErrorFmt, valueName, envName)
	}

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.WarningFmt != "" {
		ctrl.OnceWarn.Do(func() {
			logger.Warnf(ctrl.WarningFmt, valueName, envName)
		})
	}

	return nil
}
