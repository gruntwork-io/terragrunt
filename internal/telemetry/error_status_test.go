package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/multierror"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// exportedSpan is the part of a console-exported span that these tests read.
type exportedSpan struct {
	Status struct {
		Description string     `json:"Description"`
		Code        codes.Code `json:"Code"`
	} `json:"Status"`
	Attributes []exportedAttribute `json:"Attributes"`
}

// exportedAttribute is one attribute of an [exportedSpan].
type exportedAttribute struct {
	Value struct {
		Value any `json:"Value"`
	} `json:"Value"`
	Key string `json:"Key"`
}

// TestTraceSetsErrorStatusAndType verifies that a failed operation records the
// span status and an error.type attribute, per the OpenTelemetry
// recording-errors convention. The errors mirror what Terragrunt's spans
// return: a failed tofu command arrives as a [util.ProcessExecutionError]
// inside the [multierror.Error] a unit run collects.
func TestTraceSetsErrorStatusAndType(t *testing.T) {
	t.Parallel()

	const (
		processErrorType = "github.com/gruntwork-io/terragrunt/internal/util.ProcessExecutionError"
		diagnosticsType  = "github.com/hashicorp/hcl/v2.Diagnostics"
	)

	other := semconv.ErrorTypeOther.Value.AsString()

	tofuFailed := &util.ProcessExecutionError{Err: errors.New("exit status 1"), Command: "tofu", Args: []string{"plan"}}
	parseFailed := hcl.Diagnostics{{Severity: hcl.DiagError, Summary: "Missing expression"}}

	tests := []struct {
		err       error
		name      string
		errorType string
	}{
		{
			name:      "failed tofu command",
			err:       tofuFailed,
			errorType: processErrorType,
		},
		{
			name:      "unit run with a failed tofu command",
			err:       multierror.Join(tofuFailed),
			errorType: processErrorType,
		},
		{
			name:      "run all with two failed tofu commands",
			err:       multierror.Join(multierror.Join(tofuFailed), multierror.Join(tofuFailed)),
			errorType: processErrorType,
		},
		{
			name:      "run all with a dependent skipped after a failure",
			err:       multierror.Join(tofuFailed, runner.NewUnitEarlyExitError("app", "vpc")),
			errorType: other,
		},
		{
			name:      "config parse failure",
			err:       fmt.Errorf("parse unit: %w", parseFailed),
			errorType: diagnosticsType,
		},
		{
			name:      "wrapped sentinel",
			err:       fmt.Errorf("run unit: %w", runner.ErrRunnerNotSet),
			errorType: other,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			span := traceOnce(t, tt.err)

			require.Equal(t, codes.Error, span.Status.Code)
			require.Equal(t, tt.err.Error(), span.Status.Description)
			require.Equal(t, tt.errorType, attributeValue(span, string(semconv.ErrorTypeKey)))
		})
	}
}

// TestTraceSuccessLeavesStatusUnset verifies that a successful operation does
// not mark the span as errored.
func TestTraceSuccessLeavesStatusUnset(t *testing.T) {
	t.Parallel()

	span := traceOnce(t, nil)

	require.Equal(t, codes.Unset, span.Status.Code)
	require.Nil(t, attributeValue(span, string(semconv.ErrorTypeKey)))
}

// traceOnce traces one operation that returns err through a console exporter
// and returns the span it printed.
func traceOnce(t *testing.T, err error) exportedSpan {
	t.Helper()

	var buf bytes.Buffer

	tracer, newErr := telemetry.NewTracer(
		t.Context(), logger.CreateLogger(), "test", "v0.0.0", &buf,
		&telemetry.Options{TraceExporter: "console"},
	)
	require.NoError(t, newErr)

	traceErr := tracer.Trace(t.Context(), "op", nil, func(context.Context) error {
		return err
	})
	require.Equal(t, err, traceErr)

	var span exportedSpan
	require.NoError(t, json.Unmarshal(buf.Bytes(), &span))

	return span
}

// attributeValue returns the value of the span attribute named key, or nil
// when the span has no such attribute.
func attributeValue(span exportedSpan, key string) any {
	i := slices.IndexFunc(span.Attributes, func(attr exportedAttribute) bool {
		return attr.Key == key
	})
	if i < 0 {
		return nil
	}

	return span.Attributes[i].Value.Value
}
