package strict

import "golang.org/x/exp/slices"

const (
	// ActiveStatus is the Status of a control that is ongoing.
	ActiveStatus Status = iota
	// CompletedStatus is the Status of a Control that is completed.
	CompletedStatus
	// SuspendedStatus is the Status of a Control that is suspended, it does nothing and is left only to avoid returning the `InvalidControlNameError` Error.
	SuspendedStatus
)

var statusNames = map[Status]string{
	ActiveStatus:    "Active",
	CompletedStatus: "Completed",
	SuspendedStatus: "Suspended",
}

type Statuses []Status

func (statuses Statuses) Contains(status Status) bool {
	return slices.Contains(statuses, status)
}

type Status byte

func (status Status) String() string {
	if name, ok := statusNames[status]; ok {
		return name
	}

	return "unknown"
}
