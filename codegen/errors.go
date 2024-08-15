package codegen

import "fmt"

// Custom error types

// UnknownGenerateIfExistsValError is an error type for unknown values of generate
// if_exists.
type UnknownGenerateIfExistsValError struct {
	val string
}

// Error returns the error message for UnknownGenerateIfExistsValError.
func (err UnknownGenerateIfExistsValError) Error() string {
	if err.val != "" {
		return err.val + " is not a valid value for generate if_exists"
	}

	return "Received unknown value for if_exists"
}

// UnknownGenerateIfDisabledValError is an error type for unknown values of generate
// if_disabled.
type UnknownGenerateIfDisabledValError struct {
	val string
}

// Error returns the error message for UnknownGenerateIfDisabledVal.
func (err UnknownGenerateIfDisabledValError) Error() string {
	if err.val != "" {
		return err.val + " is not a valid value for generate if_disabled"
	}

	return "Received unknown value for if_disabled"
}

// GenerateFileExistsError is an error type for when a file already exists.
type GenerateFileExistsError struct {
	path string
}

// Error returns the error message for GenerateFileExistsError.
func (err GenerateFileExistsError) Error() string {
	return fmt.Sprintf("Can not generate terraform file: %s already exists", err.path)
}

// GenerateFileRemoveError is an error type for when a file can not be removed.
type GenerateFileRemoveError struct {
	path string
}

// Error returns the error message for GenerateFileRemoveError.
func (err GenerateFileRemoveError) Error() string {
	return "Can not remove terraform file: " + err.path
}
