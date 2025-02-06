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

// DeprecatedFlagName is strict control for deprecated flag names.
type DeprecatedFlagName struct {
	*Control
	ErrorFmt   string
	WarningFmt string

	depreacedFlag cli.Flag
	newFlag       cli.Flag
}

// NewDeprecatedFlagName returns a new `DeprecatedFlagName` instance.
// Since we don't know which names can be used at the time of definition,
// we take the first name from the list `Names()` for the name and description to display it in `info strict`.
func NewDeprecatedFlagName(depreacedFlag, newFlag cli.Flag, newValue string) *DeprecatedFlagName {
	var (
		deprecatedName = util.FirstElement(util.RemoveEmptyElements(depreacedFlag.Names()))
		newName        = util.FirstElement(util.RemoveEmptyElements(newFlag.Names()))
	)

	if newValue != "" {
		newName += "=" + newValue
	}

	return &DeprecatedFlagName{
		Control: &Control{
			Name:        deprecatedName,
			Description: "replaced with: " + newName,
		},
		ErrorFmt:      "`--%s` flag is no longer supported. Use `--%s` instead.",
		WarningFmt:    "`--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.",
		depreacedFlag: depreacedFlag,
		newFlag:       newFlag,
	}
}

// Evaluate implements `strict.Control` interface.
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
