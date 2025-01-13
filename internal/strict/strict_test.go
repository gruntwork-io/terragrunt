package strict_test

// Add some basic tests that confirm that by default, a warning is emitted when strict mode is disabled,
// and an error is emitted when a specific control is enabled.
// Make sure to test both when the specific control is enabled, and when the global strict mode is enabled.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
)

const (
	testOngoingA   strict.ControlName = "test-ongoing-a"
	testOngoingB   strict.ControlName = "test-ongoing-b"
	testOngoingC   strict.ControlName = "test-ongoing-c"
	testCompletedA strict.ControlName = "test-completed-a"
	testCompletedB strict.ControlName = "test-completed-b"
	testCompletedC strict.ControlName = "test-completed-c"
)

func testLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}

func newTestControls() strict.Controls {
	return strict.Controls{
		testOngoingA: {
			ErrorFmt: "%s error ongoing a %s - %s.",
			WarnFmt:  "%[1]s warning ongoing a %[2]s.",
		},
		testOngoingB: {
			ErrorFmt: "error ongoing b",
			WarnFmt:  "warning ongoing b",
		},
		testOngoingC: {
			ErrorFmt: "%s error ongoing a %s - %s.",
			WarnFmt:  "%s warning ongoing a %s - %s.",
		},
		testCompletedA: {
			ErrorFmt: "no matters",
			WarnFmt:  "no matters",
			Status:   strict.StatusCompleted,
		},
		testCompletedB: {
			ErrorFmt: "no matters",
			WarnFmt:  "no matters",
			Status:   strict.StatusCompleted,
		},
		testCompletedC: {
			ErrorFmt: "no matters",
			WarnFmt:  "no matters",
			Status:   strict.StatusCompleted,
		},
	}
}

func TestEnableControl(t *testing.T) {
	t.Parallel()

	type testEnableControl struct {
		controlName string
		expectedErr error
	}

	testCases := []struct {
		enableControls          []testEnableControl
		expectedEnabledControls []strict.ControlName
		expectedCompletedMsg    string
	}{
		{
			[]testEnableControl{
				{
					string(testOngoingA),
					nil,
				},
				{
					string(testOngoingC),
					nil,
				},
				{
					string(testCompletedA),
					nil,
				},
				{
					string(testCompletedC),
					nil,
				},
				{
					"wrong-name",
					strict.NewInvalidControlNameError([]string{string(testOngoingA), string(testOngoingB), string(testOngoingC)}),
				},
			},
			[]strict.ControlName{testOngoingA, testOngoingC, testCompletedA, testCompletedC},
			fmt.Sprintf(strict.WarningCompletedControlsFmt, strings.Join([]string{string(testCompletedA), string(testCompletedC)}, ", ")),
		},
		{
			[]testEnableControl{
				{
					string(testOngoingB),
					nil,
				},
				{
					string(testCompletedB),
					nil,
				},
			},
			[]strict.ControlName{testOngoingB, testCompletedB},
			fmt.Sprintf(strict.WarningCompletedControlsFmt, strings.Join([]string{string(testCompletedB)}, ", ")),
		},
		{
			[]testEnableControl{},
			[]strict.ControlName{},
			"",
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			logger, output := testLogger()
			controls := newTestControls()

			for _, testEnableControl := range testCase.enableControls {

				err := controls.EnableControl(testEnableControl.controlName)

				if testEnableControl.expectedErr != nil {
					assert.EqualError(t, err, testEnableControl.expectedErr.Error())

					continue
				}

				assert.NoError(t, err)
			}

			var actualEnabledControls []strict.ControlName

			for name, control := range controls {
				if control.Enabled {
					actualEnabledControls = append(actualEnabledControls, name)
				}
			}

			assert.ElementsMatch(t, testCase.expectedEnabledControls, actualEnabledControls)

			controls.NotifyCompletedControls(logger)

			if testCase.expectedCompletedMsg == "" {
				assert.Empty(t, output.String())

				return
			}

			assert.Contains(t, strings.TrimSpace(output.String()), testCase.expectedCompletedMsg)
		})
	}
}

func TestEnableStrictMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		enableStrictMode        bool
		expectedEnabledControls []strict.ControlName
	}{
		{
			true,
			[]strict.ControlName{testOngoingA, testOngoingB, testOngoingC},
		},
		{
			false,
			[]strict.ControlName{},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			logger, output := testLogger()
			controls := newTestControls()

			if testCase.enableStrictMode {
				controls.EnableStrictMode()
			}

			var actualEnabledControls []strict.ControlName

			for name, control := range controls {
				if control.Enabled {
					actualEnabledControls = append(actualEnabledControls, name)
				}
			}

			assert.ElementsMatch(t, testCase.expectedEnabledControls, actualEnabledControls)

			controls.NotifyCompletedControls(logger)
			assert.Empty(t, output.String())
		})
	}
}

func TestEvaluateControl(t *testing.T) {
	t.Parallel()

	type testEvaluateControl struct {
		controlName strict.ControlName
		args        []any
		expectedErr error
	}

	testCases := []struct {
		enableControls   []strict.ControlName
		evaluateControls []testEvaluateControl
		expectedWarns    []string
	}{
		{
			[]strict.ControlName{testOngoingA, testOngoingB},
			[]testEvaluateControl{
				{
					testOngoingA,
					[]any{"foo", "bar", "baz"},
					errors.Errorf(newTestControls()[testOngoingA].ErrorFmt, "foo", "bar", "baz"),
				},
			},
			[]string{""},
		},
		{
			[]strict.ControlName{testOngoingB},
			[]testEvaluateControl{
				{
					testOngoingB,
					nil,
					errors.Errorf(newTestControls()[testOngoingB].ErrorFmt),
				},
			},
			[]string{""},
		},
		{
			// Testing output warning message once.
			[]strict.ControlName{testOngoingB},
			[]testEvaluateControl{
				{
					testOngoingA,
					[]any{"foo", "bar", "baz"},
					nil,
				},
				{
					testOngoingA,
					[]any{"foo", "bar", "baz"},
					nil,
				},
			},
			[]string{fmt.Sprintf(newTestControls()[testOngoingA].WarnFmt, "foo", "bar", "baz")},
		},
		{
			[]strict.ControlName{testCompletedA},
			[]testEvaluateControl{
				{
					testOngoingA,
					[]any{"foo", "bar", "baz"},
					nil,
				},
			},
			[]string{fmt.Sprintf(newTestControls()[testOngoingA].WarnFmt, "foo", "bar", "baz")},
		},
		{
			[]strict.ControlName{testCompletedA},
			[]testEvaluateControl{
				{
					testCompletedA,
					nil,
					nil,
				},
			},
			[]string{""},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			logger, output := testLogger()
			controls := newTestControls()

			for _, name := range testCase.enableControls {
				controls.EnableControl(string(name))
			}

			for _, evaluateControl := range testCase.evaluateControls {
				err := controls.Evaluate(logger, evaluateControl.controlName, evaluateControl.args...)

				if evaluateControl.expectedErr != nil {
					assert.EqualError(t, err, evaluateControl.expectedErr.Error())
					assert.Empty(t, output.String())

					return
				}

				assert.NoError(t, err)
			}

			if len(testCase.expectedWarns) == 0 {
				assert.Empty(t, output.String())

				return
			}

			actualWarns := strings.Split(strings.TrimSpace(output.String()), "\n")
			assert.ElementsMatch(t, actualWarns, testCase.expectedWarns)
		})
	}
}
