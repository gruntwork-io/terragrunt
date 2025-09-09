package controls

import (
	"context"
	"slices"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	GlobalEnvVarsCategoryName     = "Global env vars"
	CommandEnvVarsCategoryNameFmt = "`%s` command env vars"
)

var _ = strict.Control(new(DeprecatedEnvVar))

// DeprecatedEnvVar is strict control for deprecated environment variables.
type DeprecatedEnvVar struct {
	deprecatedFlag cli.Flag
	newFlag        cli.Flag
	*Control
	ErrorFmt   string
	WarningFmt string
}

// NewDeprecatedEnvVar returns a new `DeprecatedEnvVar` instance.
// Since we don't know which env vars can be used at the time of definition,
// we take the first env var from the list `GetEnvVars()` for the name and description to display it in `info strict`.
func NewDeprecatedEnvVar(deprecatedFlag, newFlag cli.Flag, newValue string) *DeprecatedEnvVar {
	var (
		deprecatedName = util.FirstElement(util.RemoveEmptyElements(deprecatedFlag.GetEnvVars()))
		newName        = util.FirstElement(util.RemoveEmptyElements(newFlag.GetEnvVars()))
	)

	if newValue != "" {
		newName += "=" + newValue
	}

	return &DeprecatedEnvVar{
		Control: &Control{
			Name:        deprecatedName,
			Description: "replaced with: " + newName,
		},
		ErrorFmt: "The `%s` environment variable is no longer supported. Use `%s` instead.",
		// The `TERRAGRUNT_LOG_LEVEL` environment variable is deprecated and will be removed in a future version of Terragrunt. Use `TG_LOG_LEVEL=trace` instead.
		WarningFmt:     "The `%s` environment variable is deprecated and will be removed in a future version of Terragrunt. Use `%s` instead.",
		deprecatedFlag: deprecatedFlag,
		newFlag:        newFlag,
	}
}

// Evaluate implements `strict.Control` interface.
func (ctrl *DeprecatedEnvVar) Evaluate(ctx context.Context) error {
	var (
		valueName = ctrl.deprecatedFlag.Value().GetName()
		envName   string
	)

	if valueName == "" || !ctrl.deprecatedFlag.Value().IsEnvSet() || !slices.Contains(ctrl.deprecatedFlag.GetEnvVars(), valueName) {
		return nil
	}

	if names := ctrl.newFlag.GetEnvVars(); len(names) > 0 {
		envName = names[0]

		value := ctrl.newFlag.Value().String()

		if v, ok := ctrl.newFlag.Value().Get().(bool); ok && ctrl.newFlag.Value().IsNegativeBoolFlag() {
			value = strconv.FormatBool(!v)
		}

		if value == "" {
			value = ctrl.deprecatedFlag.Value().String()
		}

		envName += "=" + value
	}

	if ctrl.Enabled {
		if ctrl.Status != strict.ActiveStatus || ctrl.ErrorFmt == "" {
			return nil
		}

		return errors.Errorf(ctrl.ErrorFmt, valueName, envName)
	}

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.WarningFmt != "" && !ctrl.isSuppressed() {
		ctrl.OnceWarn.Do(func() {
			logger.Warnf(ctrl.WarningFmt, valueName, envName)
		})
	}

	return nil
}
