package tips

import (
	"strings"
)

// InvalidTipNameError is an error that is returned when an invalid tip name is requested.
type InvalidTipNameError struct {
	requestedName string
	allowedNames  []string
}

func NewInvalidTipNameError(requestedName string, allowedNames []string) *InvalidTipNameError {
	return &InvalidTipNameError{
		requestedName: requestedName,
		allowedNames:  allowedNames,
	}
}

func (err InvalidTipNameError) Error() string {
	return "invalid tip suppression requested for `--no-tip`: '" + err.requestedName + "'; valid tip(s) for suppression: " + strings.Join(err.allowedNames, ", ")
}

func (err InvalidTipNameError) Is(target error) bool {
	_, ok := target.(*InvalidTipNameError)
	return ok
}
