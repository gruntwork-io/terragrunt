package cli

import "fmt"

type InvalidKeyValueError struct {
	value string
	sep   string
}

func NewInvalidKeyValueError(sep, value string) *InvalidKeyValueError {
	return &InvalidKeyValueError{value, sep}
}

func (err InvalidKeyValueError) Error() string {
	return fmt.Sprintf("invalid key-value pair, expected format KEY%sVALUE, got %s.", err.sep, err.value)
}
