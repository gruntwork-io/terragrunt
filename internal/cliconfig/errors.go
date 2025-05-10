package cliconfig

import "fmt"

type NotFoundError struct {
	path string
}

func (err NotFoundError) Error() string {
	return fmt.Sprintf("could not read CLI config file %s: file not found", err.path)
}

func NewNotFoundError(path string) *NotFoundError {
	return &NotFoundError{
		path: path,
	}
}

type FileReadError struct {
	underlyingErr error
	path          string
}

func (err FileReadError) Error() string {
	return fmt.Sprintf("could not read CLI config file %s: %s", err.path, err.underlyingErr)
}

func (err FileReadError) Unwrap() error {
	return err.underlyingErr
}

func NewFileReadError(path string, err error) *FileReadError {
	return &FileReadError{
		path:          path,
		underlyingErr: err,
	}
}

type DecodeError struct {
	underlyingErr error
	path          string
}

func (err DecodeError) Error() string {
	return fmt.Sprintf("could not decode CLI config file %s: %s", err.path, err.underlyingErr)
}

func (err DecodeError) Unwrap() error {
	return err.underlyingErr
}

func NewDecodeError(path string, err error) *DecodeError {
	return &DecodeError{
		path:          path,
		underlyingErr: err,
	}
}
