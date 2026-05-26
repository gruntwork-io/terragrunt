package report

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
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
	if !shouldColor {
		identity := func(s string) string { return s }

		return &Colorizer{
			headingTitleColorizer: identity,
			headingUnitColorizer:  identity,
			successColorizer:      identity,
			failureColorizer:      identity,
			exitColorizer:         identity,
			excludeColorizer:      identity,
			successUnitColorizer:  identity,
			failureUnitColorizer:  identity,
			exitUnitColorizer:     identity,
			excludeUnitColorizer:  identity,
			nanosecondColorizer:   identity,
			microsecondColorizer:  identity,
			millisecondColorizer:  identity,
			secondColorizer:       identity,
			minuteColorizer:       identity,
			defaultColorizer:      identity,
			paddingColorizer:      identity,
		}
	}

	bold := lipgloss.NewStyle().Bold(true)

	boldYellow := bold.Foreground(lipgloss.Color("11"))
	boldWhite := bold.Foreground(lipgloss.Color("15"))
	boldGreen := bold.Foreground(lipgloss.Color("10"))
	boldRed := bold.Foreground(lipgloss.Color("9"))
	boldBlue := bold.Foreground(lipgloss.Color("12"))
	boldCyan := bold.Foreground(lipgloss.Color("14"))

	green := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	gray := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	return &Colorizer{
		headingTitleColorizer: func(s string) string { return boldYellow.Render(s) },
		headingUnitColorizer:  func(s string) string { return boldWhite.Render(s) },
		successColorizer:      func(s string) string { return boldGreen.Render(s) },
		failureColorizer:      func(s string) string { return boldRed.Render(s) },
		exitColorizer:         func(s string) string { return boldYellow.Render(s) },
		excludeColorizer:      func(s string) string { return boldBlue.Render(s) },
		successUnitColorizer:  func(s string) string { return green.Render(s) },
		failureUnitColorizer:  func(s string) string { return red.Render(s) },
		exitUnitColorizer:     func(s string) string { return yellow.Render(s) },
		excludeUnitColorizer:  func(s string) string { return blue.Render(s) },
		nanosecondColorizer:   func(s string) string { return boldCyan.Render(s) },
		microsecondColorizer:  func(s string) string { return boldCyan.Render(s) },
		millisecondColorizer:  func(s string) string { return boldCyan.Render(s) },
		secondColorizer:       func(s string) string { return boldGreen.Render(s) },
		minuteColorizer:       func(s string) string { return boldYellow.Render(s) },
		defaultColorizer:      func(s string) string { return boldWhite.Render(s) },
		paddingColorizer:      func(s string) string { return gray.Render(s) },
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
