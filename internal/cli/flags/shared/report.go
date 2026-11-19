package shared

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// SummaryDisableFlagName is the flag name for `--summary-disable`, which omits the summary printed at
	// the end of a run.
	SummaryDisableFlagName = "summary-disable"
	// SummaryPerUnitFlagName is the flag name for `--summary-per-unit`, which adds each unit's duration to
	// the run summary.
	SummaryPerUnitFlagName = "summary-per-unit"
	// ReportFileFlagName is the flag name for `--report-file`, the path of the run report. A .csv or
	// .json extension also sets the report format when `--report-format` is unset.
	ReportFileFlagName = "report-file"
	// ReportFormatFlagName is the flag name for `--report-format`, the run report format: csv or json.
	ReportFormatFlagName = "report-format"
	// ReportSchemaFlagName is the flag name for `--report-schema-file`, where to write the JSON schema of
	// the run report.
	ReportSchemaFlagName = "report-schema-file"
)

// NewReportFlags creates the flags that configure the summary and the report of a run over several units.
func NewReportFlags(opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        SummaryDisableFlagName,
			EnvVars:     tgPrefix.EnvVars(SummaryDisableFlagName),
			Destination: &opts.SummaryDisable,
			Usage:       `Disable the summary output at the end of a run.`,
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        SummaryPerUnitFlagName,
			EnvVars:     tgPrefix.EnvVars(SummaryPerUnitFlagName),
			Destination: &opts.SummaryPerUnit,
			Usage:       `Show duration information for each unit in the summary output.`,
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:    ReportFileFlagName,
			EnvVars: tgPrefix.EnvVars(ReportFileFlagName),
			Usage:   `Path to generate report file in.`,
			Setter: func(value string) error {
				if value == "" {
					return nil
				}

				opts.ReportFile = value

				ext := filepath.Ext(value)
				if ext == "" {
					ext = ".csv"
				}

				if ext != ".csv" && ext != ".json" {
					return nil
				}

				if opts.ReportFormat == "" {
					opts.ReportFormat = report.Format(ext[1:])
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:    ReportFormatFlagName,
			EnvVars: tgPrefix.EnvVars(ReportFormatFlagName),
			Usage:   `Format of the report file.`,
			Setter: func(value string) error {
				if value == "" && opts.ReportFormat == "" {
					opts.ReportFormat = report.FormatCSV

					return nil
				}

				opts.ReportFormat = report.Format(value)

				switch opts.ReportFormat {
				case report.FormatCSV:
				case report.FormatJSON:
				default:
					return fmt.Errorf("unsupported report format: %s", value)
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        ReportSchemaFlagName,
			EnvVars:     tgPrefix.EnvVars(ReportSchemaFlagName),
			Usage:       `Path to generate report schema file in.`,
			Destination: &opts.ReportSchemaFile,
		}),
	}
}
