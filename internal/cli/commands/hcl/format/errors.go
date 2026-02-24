package format

import "fmt"

// FileNeedsFormattingError is an error that is returned when a file needs formatting.
type FileNeedsFormattingError struct {
	Path string
}

func (e FileNeedsFormattingError) Error() string {
	return fmt.Sprintf("File '%s' needs formatting", e.Path)
}
