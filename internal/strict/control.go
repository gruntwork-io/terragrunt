package strict

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const CompletedControlsFmt = "The following strict control(s) are already completed: %s. Please remove any completed strict controls, as setting them no longer does anything. For a list of all ongoing strict controls, and the outcomes of previous strict controls, see https://terragrunt.gruntwork.io/docs/reference/strict-mode or get the actual list by running the `terragrunt info strict` command."

type ControlNames []string

func (names ControlNames) String() string {
	return strings.Join(names, ", ")
}

// Control represents an interface that can be enabled or disabled in strict mode.
// When the Control is Enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control interface {
	// GetName returns the name of the strict control.
	GetName() string

	// GetDescription returns the description of the strict control.
	GetDescription() string

	// GetStatus returns the status of the strict control.
	GetStatus() Status

	// Enable enables the control.
	Enable()

	// GetEnabled returns true if the control is enabled.
	GetEnabled() bool

	// GetCategory returns category of the strict control.
	GetCategory() *Category

	// SetCategory sets the category.
	SetCategory(category *Category)

	// GetSubcontrols returns all subcontrols.
	GetSubcontrols() Controls

	// AddSubcontrols adds the given `newCtrls` as subcontrols.
	AddSubcontrols(newCtrls ...Control)

	// SuppressWarning suppresses the warning message from being displayed.
	SuppressWarning()

	// Evaluate evaluates the strict control.
	Evaluate(ctx context.Context) error
}

// Controls are multiple of Controls.
type Controls []Control

// Names returns names of all `ctrls`.
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

// SuppressWarning suppresses the warning message from being displayed.
func (ctrls Controls) SuppressWarning() Controls {
	for _, ctrl := range ctrls {
		ctrl.SuppressWarning()
	}

	return ctrls
}

// FilterByStatus filters `ctrls` by given statuses.
func (ctrls Controls) FilterByStatus(statuses ...Status) Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if slices.Contains(statuses, ctrl.GetStatus()) {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

// RemoveDuplicates removes controls with duplicate names.
func (ctrls Controls) RemoveDuplicates() Controls {
	var unique Controls

	for _, ctrl := range ctrls {
		skip := false

		for _, uniqueCtrl := range unique {
			if uniqueCtrl.GetName() == ctrl.GetName() && uniqueCtrl.GetCategory().Name == ctrl.GetCategory().Name {
				skip = true
				break
			}
		}

		if !skip {
			unique = append(unique, ctrl)
		}
	}

	return unique
}

// FilterByEnabled filters `ctrls` by `Enabled: true` field.
func (ctrls Controls) FilterByEnabled() Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if ctrl.GetEnabled() {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

// FilterByNames filters `ctrls` by the given `names`.
func (ctrls Controls) FilterByNames(names ...string) Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if slices.Contains(names, ctrl.GetName()) {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

// FilterByCategories filters `ctrls` by the given `categories`.
func (ctrls Controls) FilterByCategories(categories ...*Category) Controls {
	var filtered Controls

	for _, ctrl := range ctrls {
		if category := ctrl.GetCategory(); (category == nil && len(categories) == 0) || (category != nil && slices.Contains(categories, category)) {
			filtered = append(filtered, ctrl)
		}
	}

	return filtered
}

// GetCategories returns a unique list of the `ctrls` categories.
func (ctrls Controls) GetCategories() Categories {
	var categories Categories

	for _, ctrl := range ctrls {
		if category := ctrl.GetCategory(); category != nil && !slices.Contains(categories, category) {
			categories = append(categories, ctrl.GetCategory())
		}
	}

	return categories
}

// SetCategory sets the given category for all `ctrls`.
func (ctrls Controls) SetCategory(category *Category) {
	for _, ctrl := range ctrls {
		ctrl.SetCategory(category)
	}
}

// Enable recursively enables all `ctrls`.
func (ctrls Controls) Enable() {
	for _, ctrl := range ctrls {
		ctrl.Enable()
		ctrl.GetSubcontrols().Enable()
	}
}

// EnableControl validates that the specified control name is valid and enables `ctrl`.
func (ctrls Controls) EnableControl(name string) error {
	if ctrl := ctrls.Find(name); ctrl != nil {
		ctrl.Enable()
		ctrl.GetSubcontrols().Enable()

		return nil
	}

	return NewInvalidControlNameError(ctrls.FilterByStatus(ActiveStatus).Names())
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

// AddSubcontrols adds the given `newCtrls` as subcontrols into all `ctrls`.
func (ctrls Controls) AddSubcontrols(newCtrls ...Control) {
	for _, ctrl := range ctrls {
		ctrl.AddSubcontrols(newCtrls...)
	}
}

// GetSubcontrols returns all subcontrols from all `ctrls`.
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

		ctrl.AddSubcontrols(controls...)
	}
}

// Find search control by given `name`, returns nil if not found.
func (ctrls Controls) Find(name string) Control {
	for _, ctrl := range ctrls {
		if ctrl != nil && ctrl.GetName() == name {
			return ctrl
		}
	}

	return nil
}

// Len implements `sort.Interface` interface.
func (ctrls Controls) Len() int {
	return len(ctrls)
}

// Less implements `sort.Interface` interface.
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

// Swap implements `sort.Interface` interface.
func (ctrls Controls) Swap(i, j int) {
	(ctrls)[i], (ctrls)[j] = (ctrls)[j], (ctrls)[i]
}

// Sort returns `ctrls` in sorted order by `Name` and `Status`.
func (ctrls Controls) Sort() Controls {
	sort.Sort(ctrls)

	return ctrls
}
