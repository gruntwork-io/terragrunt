package strict

// InvalidControlNameError is an error that is returned when an invalid control name is requested.
type InvalidControlNameError struct {
	allowedNames ControlNames
}

func NewInvalidControlNameError(allowedNames ControlNames) *InvalidControlNameError {
	return &InvalidControlNameError{
		allowedNames: allowedNames,
	}
}

func (err InvalidControlNameError) Error() string {
	return "allowed control(s): " + err.allowedNames.String()
}
