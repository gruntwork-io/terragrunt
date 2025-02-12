package controls

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var _ = strict.Control(new(Control))

// Control is the simplest implementation of the `strict.Control` interface.
type Control struct {
	// Name is the name of the control.
	Name string

	// Description is the description of the control.
	Description string

	// Status of the strict Control.
	Status strict.Status

	// Enabled indicates whether the control is enabled.
	Enabled bool

	// Category is the category of the control.
	// It is used to group controls by some name, like the command name
	Category *strict.Category

	// Subcontrols are child controls.
	// Child elements inherit parent behavior such as status and enabled state
	Subcontrols strict.Controls

	// Error is the Error that will be returned when the Control is Enabled.
	Error error

	// Warning is a Warning that will be logged when the Control is not Enabled.
	Warning string

	// OnceWarn is used to prevent the warning message from being displayed multiple times.
	OnceWarn sync.Once
}

// String implements `fmt.Stringer` interface.
func (ctrl *Control) String() string {
	return ctrl.GetName()
}

// GetName implements `strict.Control` interface.
func (ctrl *Control) GetName() string {
	return ctrl.Name
}

// GetDescription implements `strict.Control` interface.
func (ctrl *Control) GetDescription() string {
	return ctrl.Description
}

// GetStatus implements `strict.Control` interface.
func (ctrl *Control) GetStatus() strict.Status {
	return ctrl.Status
}

// GetEnabled implements `strict.Control` interface.
func (ctrl *Control) GetEnabled() bool {
	return ctrl.Enabled
}

// GetCategory implements `strict.Control` interface.
func (ctrl *Control) GetCategory() *strict.Category {
	return ctrl.Category
}

// SetCategory implements `strict.Control` interface.
func (ctrl *Control) SetCategory(category *strict.Category) {
	ctrl.Category = category
}

// Enable implements `strict.Control` interface.
func (ctrl *Control) Enable() {
	ctrl.Enabled = true
}

// GetSubcontrols implements `strict.Control` interface.
func (ctrl *Control) GetSubcontrols() strict.Controls {
	return ctrl.Subcontrols
}

// AddSubcontrols implements `strict.Control` interface.
func (ctrl *Control) AddSubcontrols(newCtrls ...strict.Control) {
	if ctrl.Subcontrols == nil {
		ctrl.Subcontrols = make([]strict.Control, 0, len(newCtrls))
	}

	ctrl.Subcontrols = append(ctrl.Subcontrols, newCtrls...)
}

// Evaluate implements `strict.Control` interface.
//
// It has a hacky variadic parameter to suppress the warning message to quickly
// address a bug in the current implementation.
// Certain evaluations should not trigger the warning, like when the control being
// evaluated is the skip-dependencies-inputs control.
func (ctrl *Control) Evaluate(ctx context.Context, suppressWarn ...bool) error {
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

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.Warning != "" {

		// Remove this suppression when the bug is fixed.
		if len(suppressWarn) > 0 && !suppressWarn[0] {
			ctrl.OnceWarn.Do(func() {
				logger.Warn(ctrl.Warning)
			})
		}
	}

	if ctrl.Subcontrols == nil {
		return nil
	}

	return ctrl.Subcontrols.Evaluate(ctx)
}
