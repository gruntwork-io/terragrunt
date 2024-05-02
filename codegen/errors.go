package codegen

import "fmt"

// Custom error types

type UnknownGenerateIfExistsVal struct {
	val string
}

func (err UnknownGenerateIfExistsVal) Error() string {
	if err.val != "" {
		return fmt.Sprintf("%s is not a valid value for generate if_exists", err.val)
	}
	return "Received unknown value for if_exists"
}

type UnknownGenerateIfDisabledVal struct {
	val string
}

func (err UnknownGenerateIfDisabledVal) Error() string {
	if err.val != "" {
		return fmt.Sprintf("%s is not a valid value for generate if_disabled", err.val)
	}
	return "Received unknown value for if_disabled"
}

type GenerateFileExistsError struct {
	path string
}

func (err GenerateFileExistsError) Error() string {
	return fmt.Sprintf("Can not generate terraform file: %s already exists", err.path)
}

type GenerateFileRemoveError struct {
	path string
}

func (err GenerateFileRemoveError) Error() string {
	return fmt.Sprintf("Can not remove terraform file: %s", err.path)
}
