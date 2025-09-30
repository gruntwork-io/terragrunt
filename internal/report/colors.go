package report

import (
	"fmt"
	"os"
	"time"

	"github.com/mgutz/ansi"
)

// Colorizer is a colorizer for the run summary output.
type Colorizer struct {
	headingTitleColorizer func(string) string
	headingUnitColorizer  func(string) string
	successColorizer      func(string) string
	failureColorizer      func(string) string
	exitColorizer         func(string) string
	excludeColorizer      func(string) string
	successUnitColorizer  func(string) string
	failureUnitColorizer  func(string) string
	exitUnitColorizer     func(string) string
	excludeUnitColorizer  func(string) string
	nanosecondColorizer   func(string) string
	microsecondColorizer  func(string) string
	millisecondColorizer  func(string) string
	secondColorizer       func(string) string
	minuteColorizer       func(string) string
	defaultColorizer      func(string) string
	paddingColorizer      func(string) string
}

// NewColorizer creates a new Colorizer.
func NewColorizer(shouldColor bool) *Colorizer {
	// Check if unit colorizing is enabled via environment variable
	shouldColorizeUnits := shouldColor && os.Getenv(envTmpUndocumentedReportUnitColorize) != ""

	if !shouldColor {
		return &Colorizer{
			headingTitleColorizer: func(s string) string { return s },
			headingUnitColorizer:  func(s string) string { return s },
			successColorizer:      func(s string) string { return s },
			failureColorizer:      func(s string) string { return s },
			exitColorizer:         func(s string) string { return s },
			excludeColorizer:      func(s string) string { return s },
			successUnitColorizer:  func(s string) string { return s },
			failureUnitColorizer:  func(s string) string { return s },
			exitUnitColorizer:     func(s string) string { return s },
			excludeUnitColorizer:  func(s string) string { return s },
			nanosecondColorizer:   func(s string) string { return s },
			microsecondColorizer:  func(s string) string { return s },
			millisecondColorizer:  func(s string) string { return s },
			secondColorizer:       func(s string) string { return s },
			minuteColorizer:       func(s string) string { return s },
			defaultColorizer:      func(s string) string { return s },
			paddingColorizer:      func(s string) string { return s },
		}
	}

	// Define unit colorizers based on environment variable
	var successUnitColorizer, failureUnitColorizer, exitUnitColorizer, excludeUnitColorizer func(string) string
	if shouldColorizeUnits {
		successUnitColorizer = ansi.ColorFunc("green+h")
		failureUnitColorizer = ansi.ColorFunc("red+h")
		exitUnitColorizer = ansi.ColorFunc("yellow+h")
		excludeUnitColorizer = ansi.ColorFunc("blue+h")
	} else {
		successUnitColorizer = func(s string) string { return s }
		failureUnitColorizer = func(s string) string { return s }
		exitUnitColorizer = func(s string) string { return s }
		excludeUnitColorizer = func(s string) string { return s }
	}

	return &Colorizer{
		headingTitleColorizer: ansi.ColorFunc("yellow+bh"),
		headingUnitColorizer:  ansi.ColorFunc("white+bh"),
		successColorizer:      ansi.ColorFunc("green+bh"),
		failureColorizer:      ansi.ColorFunc("red+bh"),
		exitColorizer:         ansi.ColorFunc("yellow+bh"),
		excludeColorizer:      ansi.ColorFunc("blue+bh"),
		successUnitColorizer:  successUnitColorizer,
		failureUnitColorizer:  failureUnitColorizer,
		exitUnitColorizer:     exitUnitColorizer,
		excludeUnitColorizer:  excludeUnitColorizer,
		nanosecondColorizer:   ansi.ColorFunc("cyan+bh"),
		microsecondColorizer:  ansi.ColorFunc("cyan+bh"),
		millisecondColorizer:  ansi.ColorFunc("cyan+bh"),
		secondColorizer:       ansi.ColorFunc("green+bh"),
		minuteColorizer:       ansi.ColorFunc("yellow+bh"),
		defaultColorizer:      ansi.ColorFunc("white+bh"),
		paddingColorizer:      ansi.ColorFunc("gray"),
	}
}

// colorDuration returns the duration as a string, colored based on the duration.
func (c *Colorizer) colorDuration(duration time.Duration) string {
	// if duration is negative, return "N/A" in default color
	if duration < 0 {
		return c.defaultColorizer("N/A")
	}

	if duration < time.Microsecond {
		return c.nanosecondColorizer(fmt.Sprintf("%dns", duration.Nanoseconds()))
	}

	if duration < time.Millisecond {
		return c.microsecondColorizer(fmt.Sprintf("%dµs", duration.Microseconds()))
	}

	if duration < time.Second {
		return c.millisecondColorizer(fmt.Sprintf("%dms", duration.Milliseconds()))
	}

	if duration < time.Minute {
		return c.secondColorizer(fmt.Sprintf("%ds", int(duration.Seconds())))
	}

	return c.minuteColorizer(fmt.Sprintf("%dm", int(duration.Minutes())))
}
