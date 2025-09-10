package controls

import (
	"context"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	GlobalFlagsCategoryName     = "Global flags"
	CommandFlagsCategoryNameFmt = "`%s` command flags"
)

var _ = strict.Control(new(DeprecatedFlagName))

// DeprecatedFlagName is strict control for deprecated flag names.
type DeprecatedFlagName struct {
	deprecatedFlag cli.Flag
	newFlag        cli.Flag
	*Control
	ErrorFmt   string
	WarningFmt string
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
		ErrorFmt: "The `--%s` flag is no longer supported. Use `--%s` instead.",
		// Output example:
		// The `--terragrunt-working-dir` flag is deprecated and will be removed in a future version of Terragrunt. Use `--working-dir=./test/fixtures/extra-args/` instead.
		WarningFmt:     "The `--%s` flag is deprecated and will be removed in a future version of Terragrunt. Use `--%s` instead.",
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

	if valueName == "" || !ctrl.deprecatedFlag.Value().IsArgSet() || !slices.Contains(ctrl.deprecatedFlag.Names(), valueName) {
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

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.WarningFmt != "" && !ctrl.isSuppressed() {
		ctrl.OnceWarn.Do(func() {
			logger.Warnf(ctrl.WarningFmt, valueName, flagName)
		})
	}

	return nil
}
