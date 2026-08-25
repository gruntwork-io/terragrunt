package gcphelper

import "errors"

var (
	// ErrParsingCredentials reports a GCP credentials payload that is not usable JSON.
	ErrParsingCredentials = errors.New("error parsing GCP credentials")
	// ErrBuildingCredentials reports a credentials payload the SDK refused to turn into credentials.
	ErrBuildingCredentials = errors.New("error building GCP credentials")
)
