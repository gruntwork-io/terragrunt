package engine

import "fmt"

type archiveExtractionError struct {
	cause        error
	downloadFile string
}

func newArchiveExtractionError(downloadFile string, cause error) *archiveExtractionError {
	return &archiveExtractionError{
		cause:        cause,
		downloadFile: downloadFile,
	}
}

func (err archiveExtractionError) Error() string {
	return fmt.Sprintf("failed to extract engine archive %q: %v", err.downloadFile, err.cause)
}

func (err archiveExtractionError) Unwrap() error {
	return err.cause
}
