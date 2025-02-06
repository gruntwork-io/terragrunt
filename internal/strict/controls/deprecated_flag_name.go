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
	MovedGlobalFlagsCategoryNameFmt = "Global flags moved to `%s` command"
	GlobalFlagsCategoryName         = "Global flags"
	CommandFlagsCategoryNameFmt     = "`%s` command flags"
)

var _ = strict.Control(new(DeprecatedFlagName))

// DeprecatedFlagName is strict control for deprecated flag names.
type DeprecatedFlagName struct {
	*Control
	ErrorFmt   string
	WarningFmt string

	deprecatedFlag cli.Flag
	newFlag        cli.Flag
}

func NewDeprecatedMovedFlagName(deprecatedFlag, newFlag cli.Flag, commandName string) *DeprecatedFlagName {
	var (
		deprecatedName = util.FirstElement(util.RemoveEmptyElements(deprecatedFlag.Names()))
		newName        = util.FirstElement(util.RemoveEmptyElements(newFlag.Names()))
	)

	return &DeprecatedFlagName{
		Control: &Control{
			Name:        deprecatedName,
			Description: "replaced with: " + newName,
		},
		ErrorFmt:       "`--%s` global flag is no longer supported. Use `--%s` instead.",
		WarningFmt:     "`--%s` global flag is moved to `" + commandName + "` command and will be removed from the global flags in a future version. Use `--%s` instead.",
		deprecatedFlag: deprecatedFlag,
		newFlag:        newFlag,
	}
}

// NewDeprecatedFlagName returns a new `DeprecatedFlagName` instance.
// Since we don't know which names can be used at the time of definition,
// we take the first name from the list `Names()` for the name and description to display it in `info strict`.
func NewDeprecatedFlagName(deprecatedFlag, newFlag cli.Flag, newValue string) *DeprecatedFlagName {
	var (
		deprecatedName = util.FirstElement(util.RemoveEmptyElements(deprecatedFlag.Names()))
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
		ErrorFmt:       "The `--%s` flag is no longer supported. Use `--%s` instead.",
		WarningFmt:     "The `--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.",
		deprecatedFlag: deprecatedFlag,
		newFlag:        newFlag,
	}
}

// Evaluate implements `strict.Control` interface.
func (ctrl *DeprecatedFlagName) Evaluate(ctx context.Context) error {
	var (
		valueName = ctrl.deprecatedFlag.Value().GetName()
		flagName  string
	)

	if valueName == "" || !ctrl.deprecatedFlag.Value().IsArgSet() || slices.Contains(ctrl.newFlag.Names(), valueName) {
		return nil
	}

	if names := ctrl.newFlag.Names(); len(names) > 0 {
		flagName = names[0]

		if ctrl.newFlag.TakesValue() {
			value := ctrl.newFlag.Value().String()

			if value == "" {
				value = ctrl.deprecatedFlag.Value().String()
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
