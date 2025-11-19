package report

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Summary formats data from a report for output as a summary.
type Summary struct {
	firstRunStart        *time.Time
	lastRunEnd           *time.Time
	padder               string
	workingDir           string
	runs                 []*Run
	UnitsSucceeded       int
	UnitsFailed          int
	EarlyExits           int
	Excluded             int
	shouldColor          bool
	showUnitLevelSummary bool
}

// These are undocumented temporary environment variables that are used
// to play with the summary, so that we can experiment with it.
const (
	envTmpUndocumentedReportPadder                  = "TMP_UNDOCUMENTED_REPORT_PADDER"
	envTmpUndocumentedReportUnitColorize            = "TMP_UNDOCUMENTED_REPORT_UNIT_COLORIZE"
	envTmpUndocumentedColorizeDefaultSummaryPadding = "TMP_UNDOCUMENTED_COLORIZE_DEFAULT_SUMMARY_PADDING"
)

// Summarize returns a summary of the report.
func (r *Report) Summarize() *Summary {
	summary := &Summary{
		workingDir:           r.workingDir,
		shouldColor:          r.shouldColor,
		showUnitLevelSummary: r.showUnitLevelSummary,
		padder:               ".",
		runs:                 r.Runs,
	}

	if os.Getenv(envTmpUndocumentedReportPadder) != "" {
		summary.padder = os.Getenv(envTmpUndocumentedReportPadder)
	}

	if len(r.Runs) == 0 {
		return summary
	}

	for _, run := range r.Runs {
		summary.Update(run)
	}

	return summary
}

func (s *Summary) TotalUnits() int {
	return len(s.runs)
}

func (s *Summary) Update(run *Run) {
	run.mu.RLock()
	defer run.mu.RUnlock()

	switch run.Result {
	case ResultSucceeded:
		s.UnitsSucceeded++
	case ResultFailed:
		s.UnitsFailed++
	case ResultEarlyExit:
		s.EarlyExits++
	case ResultExcluded:
		s.Excluded++
	}

	if s.firstRunStart == nil || run.Started.Before(*s.firstRunStart) {
		s.firstRunStart = &run.Started
	}

	if !run.Ended.IsZero() && (s.lastRunEnd == nil || run.Ended.After(*s.lastRunEnd)) {
		s.lastRunEnd = &run.Ended
	}
}

// TotalDuration returns the total duration of all runs in the report.
func (s *Summary) TotalDuration() time.Duration {
	if s.firstRunStart == nil || s.lastRunEnd == nil {
		return 0
	}

	return s.lastRunEnd.Sub(*s.firstRunStart)
}

// TotalDurationString returns the total duration of all runs in the report as a string.
// It returns the duration in the format that is easy to understand by humans.
func (s *Summary) TotalDurationString(colorizer *Colorizer) string {
	duration := s.TotalDuration()

	return colorizer.colorDuration(duration)
}

// WriteSummary writes the summary to a writer.
func (r *Report) WriteSummary(w io.Writer) error {
	summary := r.Summarize()

	// Don't write anything if there are no units
	if summary.TotalUnits() == 0 {
		return nil
	}

	_, err := fmt.Fprintf(w, "\n")
	if err != nil {
		return err
	}

	err = summary.Write(w)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "\n")
	if err != nil {
		return err
	}

	return nil
}

// Write writes the summary to a writer.
func (s *Summary) Write(w io.Writer) error {
	colorizer := NewColorizer(s.shouldColor)

	if s.showUnitLevelSummary {
		return s.writeUnitLevelSummary(w, colorizer)
	}

	header := fmt.Sprintf("%s  %s  %s",
		colorizer.headingTitleColorizer(runSummaryHeader),
		colorizer.headingUnitColorizer(fmt.Sprintf("%d units", s.TotalUnits())),
		s.TotalDurationString(colorizer),
	)
	if err := s.writeSummaryHeader(w, header); err != nil {
		return err
	}

	separatorLine := fmt.Sprintf("%s%s", prefix, strings.Repeat("─", separatorLineLength))
	if err := s.writeSummaryHeader(w, separatorLine); err != nil {
		return err
	}

	if s.UnitsSucceeded > 0 {
		if err := s.writeSummaryEntry(
			w,
			colorizer.successColorizer(successLabel),
			colorizer.successUnitColorizer(strconv.Itoa(s.UnitsSucceeded)),
			colorizer,
		); err != nil {
			return err
		}
	}

	if s.UnitsFailed > 0 {
		if err := s.writeSummaryEntry(
			w,
			colorizer.failureColorizer(failureLabel),
			colorizer.failureUnitColorizer(strconv.Itoa(s.UnitsFailed)),
			colorizer,
		); err != nil {
			return err
		}
	}

	if s.EarlyExits > 0 {
		if err := s.writeSummaryEntry(
			w,
			colorizer.exitColorizer(earlyExitLabel),
			colorizer.exitUnitColorizer(strconv.Itoa(s.EarlyExits)),
			colorizer,
		); err != nil {
			return err
		}
	}

	if s.Excluded > 0 {
		if err := s.writeSummaryEntry(
			w,
			colorizer.excludeColorizer(excludeLabel),
			colorizer.excludeUnitColorizer(strconv.Itoa(s.Excluded)),
			colorizer,
		); err != nil {
			return err
		}
	}

	return nil
}

const (
	prefix                     = "   "
	unitPrefixMultiplier       = 2
	runSummaryHeader           = "❯❯ Run Summary"
	successLabel               = "Succeeded"
	failureLabel               = "Failed"
	earlyExitLabel             = "Early Exits"
	excludeLabel               = "Excluded"
	separatorLineLength        = 28
	durationAlignmentOffset    = 4
	headerUnitCountSpacing     = 2
	defaultUnitNameLength      = 20
	headerPaddingAdjustment    = 3
	separatorPaddingAdjustment = 2
)

func (s *Summary) writeSummaryHeader(w io.Writer, value string) error {
	_, err := fmt.Fprintf(w, "%s\n", value)
	if err != nil {
		return err
	}

	return nil
}

func (s *Summary) writeSummaryEntry(w io.Writer, label string, value string, colorizer *Colorizer) error {
	_, err := fmt.Fprintf(w, "%s%s%s%s\n", prefix, label, s.padding(label, colorizer), value)
	if err != nil {
		return err
	}

	return nil
}

// writeUnitLevelSummary writes the summary with unit level summaries grouped by categories
func (s *Summary) writeUnitLevelSummary(w io.Writer, colorizer *Colorizer) error {
	maxUnitNameLength := 0

	for _, run := range s.runs {
		name := run.Path
		if s.workingDir != "" {
			name = strings.TrimPrefix(name, s.workingDir+string(os.PathSeparator))
		}

		if len(name) > maxUnitNameLength {
			maxUnitNameLength = len(name)
		}
	}

	headerPadding := 0
	if maxUnitNameLength > defaultUnitNameLength {
		headerPadding = maxUnitNameLength - defaultUnitNameLength + headerPaddingAdjustment
	}

	header := fmt.Sprintf(
		"%s  %s%s  %s",
		runSummaryHeader,
		colorizer.headingUnitColorizer(fmt.Sprintf("%d units", s.TotalUnits())),
		strings.Repeat(" ", headerPadding),
		s.TotalDurationString(colorizer),
	)
	if err := s.writeSummaryHeader(w, colorizer.headingTitleColorizer(header)); err != nil {
		return err
	}

	separatorAdjustment := 0
	if headerPadding > 0 {
		separatorAdjustment = headerPadding - separatorPaddingAdjustment
	}

	separatorLine := fmt.Sprintf("%s%s", prefix, strings.Repeat("─", separatorLineLength+separatorAdjustment))
	if err := s.writeSummaryHeader(w, separatorLine); err != nil {
		return err
	}

	resultGroups := map[Result][]*Run{
		ResultSucceeded: {},
		ResultFailed:    {},
		ResultEarlyExit: {},
		ResultExcluded:  {},
	}

	for _, run := range s.runs {
		resultGroups[run.Result] = append(resultGroups[run.Result], run)
	}

	categories := []struct {
		colorizer     func(string) string
		unitColorizer func(string) string
		result        Result
		label         string
		count         int
	}{
		{
			colorizer:     colorizer.successColorizer,
			unitColorizer: colorizer.successUnitColorizer,
			result:        ResultSucceeded,
			label:         successLabel,
			count:         s.UnitsSucceeded,
		},
		{
			colorizer:     colorizer.failureColorizer,
			unitColorizer: colorizer.failureUnitColorizer,
			result:        ResultFailed,
			label:         failureLabel,
			count:         s.UnitsFailed,
		},
		{
			colorizer:     colorizer.exitColorizer,
			unitColorizer: colorizer.exitUnitColorizer,
			result:        ResultEarlyExit,
			label:         earlyExitLabel,
			count:         s.EarlyExits,
		},
		{
			colorizer:     colorizer.excludeColorizer,
			unitColorizer: colorizer.excludeUnitColorizer,
			result:        ResultExcluded,
			label:         excludeLabel,
			count:         s.Excluded,
		},
	}

	for _, category := range categories {
		if category.count > 0 {
			categoryHeader := fmt.Sprintf("%s (%d)", category.label, category.count)

			categoryHeaderColored := category.colorizer(categoryHeader)
			if _, err := fmt.Fprintf(w, "%s%s\n", prefix, categoryHeaderColored); err != nil {
				return err
			}

			runs := resultGroups[category.result]
			slices.SortFunc(runs, func(a, b *Run) int {
				aDuration := a.Ended.Sub(a.Started)
				bDuration := b.Ended.Sub(b.Started)

				return int(bDuration - aDuration)
			})

			for _, run := range runs {
				if err := s.writeUnitDuration(w, run, colorizer, category.unitColorizer); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// writeUnitDuration writes unit duration with cleaner formatting
func (s *Summary) writeUnitDuration(w io.Writer, run *Run, colorizer *Colorizer, unitColorizer func(string) string) error {
	duration := run.Ended.Sub(run.Started)

	name := run.Path
	if s.workingDir != "" {
		name = strings.TrimPrefix(name, s.workingDir+string(os.PathSeparator))
	}

	padding := s.unitDurationPadding(name, colorizer)

	_, err := fmt.Fprintf(
		w, "%s%s%s%s\n",
		strings.Repeat(prefix, unitPrefixMultiplier),
		unitColorizer(name),
		padding,
		colorizer.colorDuration(duration),
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Summary) padding(label string, colorizer *Colorizer) string {
	headerUnitCountVisualPosition := s.visualLength(runSummaryHeader) + headerUnitCountSpacing

	currentLabelLength := s.visualLength(label)
	currentPosition := len(prefix) + currentLabelLength

	paddingNeeded := headerUnitCountVisualPosition - currentPosition

	paddingNeeded -= 4

	if paddingNeeded < 0 {
		paddingNeeded = 0
	}

	padding := strings.Repeat(s.padder, paddingNeeded)

	whitespaceLen := 2

	if len(padding) < whitespaceLen {
		return "  "
	}

	padding = " " + padding[1:len(padding)-1] + " "

	if os.Getenv(envTmpUndocumentedColorizeDefaultSummaryPadding) == "true" {
		return colorizer.paddingColorizer(padding)
	}

	return strings.ReplaceAll(padding, s.padder, " ")
}

// ansiRegex is used to remove ANSI escape codes from strings.
// We compile it here to avoid re-compiling it on every call to visualLength.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visualLength calculates the visual length of a string by removing ANSI escape codes
func (s *Summary) visualLength(text string) int {
	cleanText := ansiRegex.ReplaceAllString(text, "")

	return len(cleanText)
}

// unitDurationPadding calculates padding for unit names to align durations with header
func (s *Summary) unitDurationPadding(name string, colorizer *Colorizer) string {
	maxUnitNameLength := 0

	for _, run := range s.runs {
		runName := run.Path
		if s.workingDir != "" {
			runName = strings.TrimPrefix(runName, s.workingDir+string(os.PathSeparator))
		}

		if len(runName) > maxUnitNameLength {
			maxUnitNameLength = len(runName)
		}
	}

	headerPadding := 0
	if maxUnitNameLength > defaultUnitNameLength {
		headerPadding = maxUnitNameLength - defaultUnitNameLength + headerPaddingAdjustment
	}

	headerPrefix := fmt.Sprintf("%s  %d units  ", runSummaryHeader, s.TotalUnits())
	headerDurationColumn := len(headerPrefix) + headerPadding

	unitPrefix := strings.Repeat(prefix, unitPrefixMultiplier)
	currentPosition := len(unitPrefix) + len(name)

	paddingNeeded := max(1, headerDurationColumn-currentPosition-durationAlignmentOffset)

	padding := strings.Repeat(s.padder, paddingNeeded)

	whitespaceLen := 2

	if len(padding) < whitespaceLen {
		return "  "
	}

	padding = " " + padding[1:len(padding)-1] + " "

	return colorizer.paddingColorizer(padding)
}
