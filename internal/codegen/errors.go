package codegen

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidEncryptionFallback is returned when the remote_state encryption fallback is not
	// a map holding only method = "unencrypted".
	ErrInvalidEncryptionFallback = errors.New(
		`remote_state encryption fallback must be { method = "unencrypted" }`,
	)

	// ErrInvalidEncryptionEnforced is returned when the remote_state encryption enforced entry is not a bool.
	ErrInvalidEncryptionEnforced = errors.New("remote_state encryption enforced must be a bool")
)

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
	return fmt.Sprintf("Can not generate file: %s already exists", err.path)
}

type GenerateFileRemoveError struct {
	path string
}

func (err GenerateFileRemoveError) Error() string {
	return "Can not remove file: " + err.path
}
