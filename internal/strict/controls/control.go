package controls

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

var _ = strict.Control(new(Control))

// Control is the simplest implementation of the `strict.Control` interface.
type Control struct {
	// Error is the Error that will be returned when the Control is Enabled.
	Error error

	// Category is the category of the control.
	Category *strict.Category

	// Name is the name of the control.
	Name string

	// Description is the description of the control.
	Description string

	// Warning is a Warning that will be logged when the Control is not Enabled.
	Warning string

	// Subcontrols are child controls.
	Subcontrols strict.Controls

	// OnceWarn is used to prevent the warning message from being displayed multiple times.
	OnceWarn sync.Once

	// Status of the strict Control.
	Status strict.Status

	// Enabled indicates whether the control is enabled.
	Enabled bool

	// Suppress suppresses the warning message from being displayed.
	// Uses int32 for atomic operations (0 = false, 1 = true)
	suppress int32
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

// SuppressWarning suppresses the warning message from being displayed.
func (ctrl *Control) SuppressWarning() {
	atomic.StoreInt32(&ctrl.suppress, 1)
}

// isSuppressed returns true if warning is suppressed.
func (ctrl *Control) isSuppressed() bool {
	return atomic.LoadInt32(&ctrl.suppress) == 1
}

// Evaluate implements `strict.Control` interface.
func (ctrl *Control) Evaluate(ctx context.Context) error {
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

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.Warning != "" && !ctrl.isSuppressed() {
		ctrl.OnceWarn.Do(func() {
			logger.Warn(ctrl.Warning)
		})
	}

	if ctrl.Subcontrols == nil {
		return nil
	}

	return ctrl.Subcontrols.Evaluate(ctx)
}
