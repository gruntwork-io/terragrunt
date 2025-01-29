package strict

import (
	"context"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/slices"
)

const CompletedControlsFmt = "The following strict control(s) are already completed: %s. Please remove any completed strict controls, as setting them no longer does anything. For a list of all ongoing strict controls, and the outcomes of previous strict controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode or get the actual list by running the `terragrunt info strict` command."

type ControlNames []string

func (names ControlNames) String() string {
	return strings.Join(names, ", ")
}

// Control represents a Control that can be Enabled or disabled in strict mode.
// When the Control is Enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control interface {
	// GetName is the GetName of the Control.
	GetName() string

	GetDescription() string

	// Status of the strict Control.
	GetStatus() Status

	Enable()

	GetEnabled() bool

	GetCategory() *Category

	SetCategory(category *Category)

	GetSubcontrols() Controls

	AddSubcontrols(newCtrls ...Control)

	Evaluate(ctx context.Context) error
}

type Controls []Control

// Names returns all strict Control names.
func (ctrls Controls) Names() ControlNames {
	var names ControlNames

	for _, ctrl := range ctrls {
		if name := ctrl.GetName(); name != "" {
			names = append(names, name)
		}
	}

	slices.Sort(names)

	return names
}

func (ctrls Controls) FilterByStatus(statuses ...Status) Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if slices.Contains(statuses, ctrl.GetStatus()) {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

func (ctrls Controls) FilterByEnabled() Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if ctrl.GetEnabled() {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

func (ctrls Controls) FilterByNames(names ...string) Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if slices.Contains(names, ctrl.GetName()) {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

func (ctrls Controls) FilterByCategories(categories ...*Category) Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if category := ctrl.GetCategory(); (category == nil && len(categories) == 0) || (category != nil && slices.Contains(categories, category)) {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

func (ctrls Controls) GetCategories() Categories {
	var categories Categories

	for _, ctrl := range ctrls {
		if category := ctrl.GetCategory(); category != nil && !slices.Contains(categories, category) {
			categories = append(categories, ctrl.GetCategory())
		}
	}

	return categories
}

func (ctrls Controls) SetCategory(category *Category) {
	for _, ctrl := range ctrls {
		ctrl.SetCategory(category)
	}
}

func (ctrls Controls) Enable() {
	for _, ctrl := range ctrls {
		ctrl.Enable()
		ctrl.GetSubcontrols().Enable()
	}
}

// EnableControl validates that the specified Control name is valid and enables this Control.
func (ctrls Controls) EnableControl(name string) error {
	foundControls := ctrls.FilterByNames(name)

	if len(foundControls) == 0 {
		return NewInvalidControlNameError(ctrls.FilterByStatus(ActiveStatus).Names())
	}

	foundControls.Enable()

	return nil
}

// LogEnabled logs the control names that are enabled and have completed Status.
func (ctrls Controls) LogEnabled(logger log.Logger) {
	enabledControls := ctrls.FilterByEnabled()

	if len(enabledControls) > 0 {
		logger.Debugf("Enabled strict control(s): %s", enabledControls.Names())
	}

	completedControls := enabledControls.FilterByStatus(CompletedStatus)

	if len(completedControls) > 0 {
		logger.Warnf(CompletedControlsFmt, completedControls.Names().String())
	}
}

// Evaluate returns an error if the one of the controls is enabled otherwise logs warning messages and returns nil.
func (ctrls Controls) Evaluate(ctx context.Context) error {
	for _, ctrl := range ctrls {
		if err := ctrl.Evaluate(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (ctrls Controls) AddSubcontrols(newCtrls ...Control) {
	for _, ctrl := range ctrls {
		ctrl.AddSubcontrols(newCtrls...)
	}
}

func (ctrls Controls) GetSubcontrols() Controls {
	var found Controls

	for _, ctrl := range ctrls {
		found = append(found, ctrl.GetSubcontrols()...)
	}

	return found
}

func (ctrls Controls) AddSubcontrolsToCategory(categoryName string, controls ...Control) {
	for _, ctrl := range ctrls {
		category := ctrl.GetSubcontrols().GetCategories().Find(categoryName)

		if category == nil {
			category = &Category{Name: categoryName}
		}

		Controls(controls).SetCategory(category)

		ctrls.AddSubcontrols(controls...)
	}
}

func (ctrls Controls) Find(name string) Control {
	for _, ctrl := range ctrls {
		if ctrl != nil && ctrl.GetName() == name {
			return ctrl
		}
	}

	return nil
}

func (ctrls Controls) Len() int {
	return len(ctrls)
}

func (ctrls Controls) Less(i, j int) bool {
	if len((ctrls)[j].GetName()) == 0 {
		return false
	} else if len((ctrls)[i].GetName()) == 0 {
		return true
	}

	if (ctrls)[i].GetStatus() == (ctrls)[j].GetStatus() {
		return (ctrls)[i].GetName() < (ctrls)[j].GetName()
	}

	return (ctrls)[i].GetStatus() < (ctrls)[j].GetStatus()
}

func (ctrls Controls) Swap(i, j int) {
	(ctrls)[i], (ctrls)[j] = (ctrls)[j], (ctrls)[i]
}

func (ctrls Controls) Sort() Controls {
	sort.Sort(ctrls)

	return ctrls
}
