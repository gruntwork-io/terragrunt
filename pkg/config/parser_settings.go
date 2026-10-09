package config

import (
	"io"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
)

// maxParserOptionCount is the most options [ParserOptions] returns: the diagnostics writer,
// the logger, the bare-include rewrite, and three diagnostics handlers.
const maxParserOptionCount = 6

// DiagnosticsOutput selects where a parser writes the diagnostics its handlers let through.
type DiagnosticsOutput int

const (
	// DiagnosticsLogged writes diagnostics as error-level log lines.
	DiagnosticsLogged DiagnosticsOutput = iota
	// DiagnosticsSuppressed writes diagnostics to the venv's error writer when the logger is at
	// debug level, and discards them otherwise.
	DiagnosticsSuppressed
	// DiagnosticsDiscarded discards diagnostics.
	DiagnosticsDiscarded
)

// ParserSettings configures the HCL parsers a [ParsingContext] builds.
type ParserSettings struct {
	// DiagnosticsHandler receives the diagnostics of every parsed file after the other handlers.
	// It is a result sink the caller owns, like [ParsingContext.FilesRead], and is nil except
	// for `hcl validate`. It must not capture a logger or a venv.
	DiagnosticsHandler func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error)
	// HaltOnErrorOnlyInBlocks, when not empty, drops a file's errors unless one falls in a block
	// with one of these names.
	HaltOnErrorOnlyInBlocks []string
	// Diagnostics selects where diagnostics are written.
	Diagnostics DiagnosticsOutput
	// RewriteBareInclude gives a bare `include {}` block a label before decoding.
	RewriteBareInclude bool
	// IgnoreDiagnostics logs diagnostics at debug level and drops them.
	IgnoreDiagnostics bool
	// SkipDefaults leaves out the logger, the bare-include rewrite, and the logged diagnostics
	// writer.
	SkipDefaults bool
}

// DefaultParserSettings returns the settings a new [ParsingContext] starts with. The bare-include
// rewrite is on unless the bare-include strict control is enabled.
func DefaultParserSettings(strictControls strict.Controls) ParserSettings {
	return ParserSettings{
		RewriteBareInclude: bareIncludeAllowed(strictControls),
	}
}

// ParserOptions builds the [hclparse.Option] list for s. Diagnostics are logged through l and
// written through v.
func ParserOptions(l log.Logger, v *venv.Venv, s ParserSettings) []hclparse.Option {
	opts := make([]hclparse.Option, 0, maxParserOptionCount)

	if opt, ok := diagnosticsWriterOption(l, v, s); ok {
		opts = append(opts, opt)
	}

	if !s.SkipDefaults {
		opts = append(opts, hclparse.WithLogger(l))

		if s.RewriteBareInclude {
			opts = append(opts, hclparse.WithFileUpdate(updateBareIncludeBlock))
		}
	}

	if len(s.HaltOnErrorOnlyInBlocks) > 0 {
		opts = append(opts, hclparse.WithHaltOnErrorOnlyForBlocks(s.HaltOnErrorOnlyInBlocks))
	}

	if s.IgnoreDiagnostics {
		opts = append(opts, hclparse.WithDiagnosticsHandler(func(
			_ *hcl.File,
			diags hcl.Diagnostics,
		) (hcl.Diagnostics, error) {
			l.Debugf("Suppressed parsing errors %v", diags)
			return nil, nil
		}))
	}

	if s.DiagnosticsHandler != nil {
		opts = append(opts, hclparse.WithDiagnosticsHandler(s.DiagnosticsHandler))
	}

	return opts
}

// diagnosticsWriterOption returns the diagnostics writer option for s, and false when s asks
// for none.
func diagnosticsWriterOption(l log.Logger, v *venv.Venv, s ParserSettings) (hclparse.Option, bool) {
	switch s.Diagnostics {
	case DiagnosticsSuppressed:
		if l.Level() >= log.DebugLevel {
			return hclparse.WithDiagnosticsWriter(v, v.Writers.ErrWriter, true), true
		}

		return hclparse.WithDiagnosticsWriter(v, io.Discard, true), true
	case DiagnosticsDiscarded:
		return hclparse.WithDiagnosticsWriter(v, io.Discard, true), true
	case DiagnosticsLogged:
	}

	if s.SkipDefaults {
		return nil, false
	}

	w := writer.New(
		writer.WithLogger(l),
		writer.WithDefaultLevel(log.ErrorLevel),
		writer.WithMsgSeparator(logMsgSeparator),
	)

	return hclparse.WithDiagnosticsWriter(v, w, l.Formatter().DisabledColors()), true
}

// bareIncludeAllowed reports whether strictControls allow bare includes, which they do unless
// the bare-include control is enabled.
func bareIncludeAllowed(strictControls strict.Controls) bool {
	strictControl := strictControls.Find(controls.BareInclude)
	if strictControl == nil {
		return true
	}

	// Evaluating the control here would spend its one-shot warning, or suppress it for good,
	// before any file with a bare include is parsed.
	return !strictControl.GetEnabled()
}
