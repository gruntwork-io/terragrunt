package strict

import (
	"cmp"
	"context"
	"slices"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

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
func (ctrls Controls) Names() []string {
	var names []string

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

	seen := make(map[string]struct{}, len(ctrls))

	for _, ctrl := range ctrls {
		name := ctrl.GetName()
		if _, ok := seen[name]; ok {
			continue
		}

		seen[name] = struct{}{}

		unique = append(unique, ctrl)
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

// LogEnabled logs the control names that are enabled.
func (ctrls Controls) LogEnabled(logger log.Logger) {
	enabledControls := ctrls.FilterByEnabled()

	if len(enabledControls) > 0 {
		logger.Debugf("Enabled strict control(s): %s", enabledControls.Names())
	}
}

// LogCompletedControls warns about any completed controls from the given explicitly requested names.
func (ctrls Controls) LogCompletedControls(logger log.Logger, requestedNames []string) {
	completedControls := ctrls.FilterByNames(requestedNames...).FilterByStatus(CompletedStatus)

	if len(completedControls) > 0 {
		logger.Warn(NewCompletedControlsWarning(completedControls.Names()).String())
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
	found := make(Controls, 0, len(ctrls))

	for _, ctrl := range ctrls {
		found = append(found, ctrl.GetSubcontrols()...)
	}

	return found
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

// Sort returns `ctrls` in sorted order by `Status` then `Name`. Empty-name
// controls sort first to keep the existing ordering stable.
func (ctrls Controls) Sort() Controls {
	slices.SortFunc(ctrls, func(a, b Control) int {
		aName, bName := a.GetName(), b.GetName()

		if aName == "" || bName == "" {
			return cmp.Compare(aName, bName)
		}

		if c := cmp.Compare(a.GetStatus(), b.GetStatus()); c != 0 {
			return c
		}

		return cmp.Compare(aName, bName)
	})

	return ctrls
}
