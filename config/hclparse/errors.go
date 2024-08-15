package hclparse

import (
	"fmt"
	"reflect"
)

// PanicWhileParsingConfigError is an error type that is returned when a panic is recovered while parsing a config file.
type PanicWhileParsingConfigError struct {
	ConfigFile     string
	RecoveredValue interface{}
}

func (err PanicWhileParsingConfigError) Error() string {
	return fmt.Sprintf(
		"Recovering panic while parsing '%s'. Got error of type '%v': %v",
		err.ConfigFile,
		reflect.TypeOf(err.RecoveredValue),
		err.RecoveredValue,
	)
}
