package codegen

import "fmt"

// Custom error types

type UnknownGenerateIfExistsVal struct {
	val string
}

func (err UnknownGenerateIfExistsVal) Error() string {
	if err.val != "" {
		return err.val + " is not a valid value for generate if_exists"
	}
	return "Received unknown value for if_exists"
}

type UnknownGenerateIfDisabledVal struct {
	val string
}

func (err UnknownGenerateIfDisabledVal) Error() string {
	if err.val != "" {
		return err.val + " is not a valid value for generate if_disabled"
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
	return "Can not remove terraform file: " + err.path
}
