package format

import "fmt"

// FileNeedsFormattingError is returned when HCL input needs formatting.
type FileNeedsFormattingError struct {
	Path string
}

func (e FileNeedsFormattingError) Error() string {
	return fmt.Sprintf("File '%s' needs formatting", e.Path)
}
