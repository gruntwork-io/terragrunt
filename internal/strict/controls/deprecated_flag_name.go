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
	GlobalFlagsCategoryName     = "Global flags"
	CommandFlagsCategoryNameFmt = "`%s` command flags"
)

var _ = strict.Control(new(DeprecatedFlagName))

type DeprecatedFlagName struct {
	*Control
	ErrorFmt   string
	WarningFmt string

	depreacedFlag cli.Flag
	newFlag       cli.Flag
}

func NewDeprecatedFlagName(depreacedFlag, newFlag cli.Flag, newValue string) *DeprecatedFlagName {
	var (
		depreacedName = util.FirstElement(util.RemoveEmptyElements(depreacedFlag.Names()))
		newName       = util.FirstElement(util.RemoveEmptyElements(newFlag.Names()))
	)

	if newValue != "" {
		newName += "=" + newValue
	}

	return &DeprecatedFlagName{
		Control: &Control{
			Name:        depreacedName,
			Description: "replaced with: " + newName,
		},
		ErrorFmt:      "`--%s` flag is no longer supported. Use `--%s` instead.",
		WarningFmt:    "`--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.",
		depreacedFlag: depreacedFlag,
		newFlag:       newFlag,
	}
}

func (ctrl *DeprecatedFlagName) Evaluate(ctx context.Context) error {
	var (
		valueName = ctrl.depreacedFlag.Value().GetName()
		flagName  string
	)

	if valueName == "" || !ctrl.depreacedFlag.Value().IsArgSet() || slices.Contains(ctrl.newFlag.Names(), valueName) {
		return nil
	}

	if names := ctrl.newFlag.Names(); len(names) > 0 {
		flagName = names[0]

		if ctrl.newFlag.TakesValue() {
			value := ctrl.newFlag.Value().String()

			if value == "" {
				value = ctrl.depreacedFlag.Value().String()
			}

			flagName += "=" + value
		}
	}

	if ctrl.Enabled {
		if ctrl.Status != strict.ActiveStatus || ctrl.ErrorFmt == "" {
			return nil
		}

		return errors.Errorf(ctrl.ErrorFmt, valueName, flagName)
	}

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.WarningFmt != "" {
		ctrl.OnceWarn.Do(func() {
			logger.Warnf(ctrl.WarningFmt, valueName, flagName)
		})
	}

	return nil
}
