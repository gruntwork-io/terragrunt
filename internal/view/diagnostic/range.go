package diagnostic

import "fmt"

// Pos represents a position in the source code.
type Pos struct {
	// Line is a one-based count for the line in the indicated file.
	Line int `json:"line"`

	// Column is a one-based count of Unicode characters from the start of the line.
	Column int `json:"column"`

	// Byte is a zero-based offset into the indicated file.
	Byte int `json:"byte"`
}

// Range represents the filename and position of the diagnostic subject.
type Range struct {
	Filename string `json:"filename"`
	Start    Pos    `json:"start"`
	End      Pos    `json:"end"`
}

func (rng Range) String() string {
	if rng.Start.Line == rng.End.Line {
		return fmt.Sprintf(
			"%s:%d,%d-%d",
			rng.Filename,
			rng.Start.Line, rng.Start.Column,
			rng.End.Column,
		)
	} else {
		return fmt.Sprintf(
			"%s:%d,%d-%d,%d",
			rng.Filename,
			rng.Start.Line, rng.Start.Column,
			rng.End.Line, rng.End.Column,
		)
	}
}
