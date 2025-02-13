package controls

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var _ = strict.Control(new(DeprecatedEnvVar))

// SuppressableControl is strict control with warnings that can be suppressed.
type SuppressableControl struct {
	*Control

	suppress bool
}

// NewSuppressableControl returns a new `SuppressableControl` instance.
// This type of strict control can have its warning suppressed at runtime.
func NewSuppressableControl(name, description, warning string, err error, category *strict.Category) *SuppressableControl {
	return &SuppressableControl{
		Control: &Control{
			Name:        name,
			Description: description,
			Error:       err,
			Warning:     warning,
			Category:    category,
		},
	}
}

// Suppress suppresses the warning message of the control.
func (ctrl *SuppressableControl) Suppress() *SuppressableControl {
	ctrl.suppress = true

	return ctrl
}

// Evaluate implements Evaluate for the `strict.Control` interface.
func (ctrl *SuppressableControl) Evaluate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.Errorf("context error during evaluation: %w", err)
	}

	if ctrl == nil {
		return nil
	}

	if ctrl.Enabled {
		if ctrl.Status != strict.ActiveStatus || ctrl.Error == nil {
			return nil
		}

		return ctrl.Error
	}

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.Warning != "" && !ctrl.suppress {
		ctrl.OnceWarn.Do(func() {
			logger.Warn(ctrl.Warning)
		})
	}

	if ctrl.Subcontrols == nil {
		return nil
	}

	return ctrl.Subcontrols.Evaluate(ctx)
}
