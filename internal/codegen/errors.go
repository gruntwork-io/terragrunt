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

	// ErrEncryptionFallbackEnforced is returned when the remote_state encryption map sets both
	// fallback and enforced = true.
	ErrEncryptionFallbackEnforced = errors.New(
		"remote_state encryption fallback cannot be combined with enforced = true; remove enforced until state is migrated",
	)
)

// Custom error types

// UnknownGenerateIfExistsVal is the error for an if_exists value that is not one of the supported strings.
type UnknownGenerateIfExistsVal struct {
	val string
}

func (err UnknownGenerateIfExistsVal) Error() string {
	if err.val != "" {
		return err.val + " is not a valid value for generate if_exists"
	}

	return "Received unknown value for if_exists"
}

// UnknownGenerateIfDisabledVal is the error for an if_disabled value that is not one of the supported strings.
type UnknownGenerateIfDisabledVal struct {
	val string
}

func (err UnknownGenerateIfDisabledVal) Error() string {
	if err.val != "" {
		return err.val + " is not a valid value for generate if_disabled"
	}

	return "Received unknown value for if_disabled"
}

// GenerateFileExistsError is the error for a generate target path that already exists when if_exists is
// "error", or holds a file Terragrunt did not generate when if_exists is "overwrite_terragrunt".
type GenerateFileExistsError struct {
	path string
}

func (err GenerateFileExistsError) Error() string {
	return fmt.Sprintf("Can not generate file: %s already exists", err.path)
}

// GenerateFileRemoveError is the error when if_disabled is "remove_terragrunt" and the target path holds a
// file Terragrunt did not generate.
type GenerateFileRemoveError struct {
	path string
}

func (err GenerateFileRemoveError) Error() string {
	return "Can not remove file: " + err.path
}
