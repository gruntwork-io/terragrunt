package global

import "github.com/gruntwork-io/terragrunt/internal/experiment"

// LogFormatExperimentRequiredError is returned when `--log-custom-format` uses
// an option that requires the log-format-options experiment, which is not enabled.
type LogFormatExperimentRequiredError struct {
	// Option is the option as written in the format string, e.g. `path=minimal`.
	Option string
}

func (err *LogFormatExperimentRequiredError) Error() string {
	return "the '" + err.Option + "' option of --" + LogCustomFormatFlagName + " requires the '" +
		experiment.LogFormatOptions + "' experiment to be enabled (e.g., --experiment=" + experiment.LogFormatOptions + ")"
}
