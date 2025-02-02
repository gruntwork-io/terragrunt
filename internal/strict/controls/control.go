package controls

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/slices"
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
	for _, newCtrl := range newCtrls {
		if !slices.Contains(ctrl.Subcontrols.Names(), newCtrl.GetName()) {
			ctrl.Subcontrols = append(ctrl.Subcontrols, newCtrls...)
		}
	}
}

// Evaluate implements `strict.Control` interface.
func (ctrl *Control) Evaluate(ctx context.Context) error {
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
		ctrl.OnceWarn.Do(func() {
			logger.Warn(ctrl.Warning)
		})
	}

	return ctrl.Subcontrols.Evaluate(ctx)
}
