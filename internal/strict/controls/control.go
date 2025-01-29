package controls

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/slices"
)

var _ = strict.Control(new(Control))

type Control struct {
	// Name is the name of the Control.
	Name string

	Description string
	// Status of the strict Control.
	Status strict.Status

	Enabled bool

	Category *strict.Category

	Subcontrols strict.Controls

	// Error is the Error that will be returned when the Control is Enabled.
	Error error
	// Warning is a Warning that will be logged when the Control is not Enabled.
	Warning string

	OnceWarn sync.Once
}

func (ctrl *Control) String() string {
	return ctrl.GetName()
}

func (ctrl *Control) GetName() string {
	return ctrl.Name
}

func (ctrl *Control) GetDescription() string {
	return ctrl.Description
}

func (ctrl *Control) GetStatus() strict.Status {
	return ctrl.Status
}

func (ctrl *Control) GetEnabled() bool {
	return ctrl.Enabled
}

func (ctrl *Control) GetCategory() *strict.Category {
	return ctrl.Category
}

func (ctrl *Control) SetCategory(category *strict.Category) {
	ctrl.Category = category
}

func (ctrl *Control) Enable() {
	ctrl.Enabled = true
}

func (ctrl *Control) GetSubcontrols() strict.Controls {
	return ctrl.Subcontrols
}

func (ctrl *Control) AddSubcontrols(newCtrls ...strict.Control) {
	for _, newCtrl := range newCtrls {
		if !slices.Contains(ctrl.Subcontrols.Names(), newCtrl.GetName()) {
			ctrl.Subcontrols = append(ctrl.Subcontrols, newCtrls...)
		}
	}
}

// Evaluate returns an Error if the Control is Enabled otherwise logs the Warning message returns nil.
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
