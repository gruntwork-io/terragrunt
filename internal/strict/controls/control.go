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
	Error       error
	Category    *strict.Category
	Name        string
	Description string
	Warning     string
	Subcontrols strict.Controls
	OnceWarn    sync.Once
	Status      strict.Status
	Enabled     bool
	Suppress    bool
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
	ctrl.Suppress = true
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

	if logger := log.LoggerFromContext(ctx); logger != nil && ctrl.Warning != "" && !ctrl.Suppress {
		ctrl.OnceWarn.Do(func() {
			logger.Warn(ctrl.Warning)
		})
	}

	if ctrl.Subcontrols == nil {
		return nil
	}

	return ctrl.Subcontrols.Evaluate(ctx)
}
