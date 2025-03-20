package hclparse

import (
	"fmt"
	"reflect"
)

type PanicWhileParsingConfigError struct {
	RecoveredValue any
	ConfigFile     string
}

func (err PanicWhileParsingConfigError) Error() string {
	return fmt.Sprintf("Recovering panic while parsing '%s'. Got error of type '%v': %v", err.ConfigFile, reflect.TypeOf(err.RecoveredValue), err.RecoveredValue)
}
