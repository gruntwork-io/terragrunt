package strict

import "slices"

const (
	// ActiveStatus is the Status of a control that is ongoing.
	ActiveStatus Status = iota
	// CompletedStatus is the Status of a Control that is completed.
	CompletedStatus
	// SuspendedStatus is the Status of a Control that is suspended.
	// It does nothing and is assigned to a control only to avoid returning the `InvalidControlNameError`.
	SuspendedStatus
)

var statusNames = map[Status]string{
	ActiveStatus:    "Active",
	CompletedStatus: "Completed",
	SuspendedStatus: "Suspended",
}

// Statuses are a set of Statuses.
type Statuses []Status

// Contains returns true if the `statuses` slice contains the given `status`.
func (statuses Statuses) Contains(status Status) bool {
	return slices.Contains(statuses, status)
}

// Status represents the status of the Control.
type Status byte

// String implements `fmt.Stringer` interface.
func (status Status) String() string {
	if name, ok := statusNames[status]; ok {
		return name
	}

	return "unknown"
}

const (
	greenColor  = "\033[0;32m"
	yellowColor = "\033[0;33m"
	resetColor  = "\033[0m"
)

// StringWithANSIColor returns a colored text representation of the status.
func (status Status) StringWithANSIColor() string {
	str := status.String()

	switch status {
	case ActiveStatus:
		return greenColor + str + resetColor
	case CompletedStatus, SuspendedStatus:
		return yellowColor + str + resetColor
	}

	return str
}
