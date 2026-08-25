package gcphelper

import (
	"errors"
	"fmt"

	"cloud.google.com/go/auth/credentials"
)

// ErrMissingCredentialsType reports a credentials payload with no "type" field. Match with errors.Is.
var ErrMissingCredentialsType = errors.New("the payload has no \"type\" field")

// ParsingCredentialsError reports a GCP credentials payload that cannot be read as JSON. Match with errors.As.
type ParsingCredentialsError struct {
	Err error
}

func (err ParsingCredentialsError) Error() string {
	return fmt.Sprintf("error parsing GCP credentials: %s", err.Err)
}

func (err ParsingCredentialsError) Unwrap() error {
	return err.Err
}

// BuildingCredentialsError reports a credentials payload the SDK refused to accept. Match with errors.As.
type BuildingCredentialsError struct {
	Err      error
	CredType credentials.CredType
}

func (err BuildingCredentialsError) Error() string {
	return fmt.Sprintf("error building GCP credentials of type %q: %s", err.CredType, err.Err)
}

func (err BuildingCredentialsError) Unwrap() error {
	return err.Err
}
