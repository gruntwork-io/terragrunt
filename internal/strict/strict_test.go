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
	"github.com/stretchr/testify/require"
)

const (
	testOngoingAName   strict.ControlName = "test-ongoing-a"
	testOngoingBName   strict.ControlName = "test-ongoing-b"
	testOngoingCName   strict.ControlName = "test-ongoing-c"
	testCompletedAName strict.ControlName = "test-completed-a"
	testCompletedBName strict.ControlName = "test-completed-b"
	testCompletedCName strict.ControlName = "test-completed-c"
)

var (
	testOngoingA = func() *strict.Control {
		return &strict.Control{
			Name:     testOngoingAName,
			ErrorFmt: "%[1]s error ongoing a %[2]s - %[3]s.",
			WarnFmt:  "%[1]s warning ongoing a %[2]s.",
		}
	}
	testOngoingB = func() *strict.Control {
		return &strict.Control{
			Name:     testOngoingBName,
			ErrorFmt: "error ongoing b",
			WarnFmt:  "warning ongoing b",
		}
	}
	testOngoingC = func() *strict.Control {
		return &strict.Control{
			Name:     testOngoingCName,
			ErrorFmt: "%s error ongoing c %s - %s.",
			WarnFmt:  "%s warning ongoing c %s - %s.",
		}
	}
	testCompletedA = func() *strict.Control {
		return &strict.Control{
			Name:     testCompletedAName,
			Status:   strict.StatusCompleted,
			ErrorFmt: "no matter",
			WarnFmt:  "no matter",
		}
	}
	testCompletedB = func() *strict.Control {
		return &strict.Control{
			Name:     testCompletedBName,
			Status:   strict.StatusCompleted,
			ErrorFmt: "no matter",
			WarnFmt:  "no matter",
		}
	}
	testCompletedC = func() *strict.Control {
		return &strict.Control{
			Name:     testCompletedCName,
			Status:   strict.StatusCompleted,
			ErrorFmt: "no matter",
			WarnFmt:  "no matter",
		}
	}
)

func newTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}

func newTestControls() strict.Controls {
	return strict.Controls{
		testOngoingA(),
		testOngoingB(),
		testOngoingC(),
		testCompletedA(),
		testCompletedB(),
		testCompletedC(),
	}
}

func TestEnableControl(t *testing.T) {
	t.Parallel()

	type testEnableControl struct {
		controlName strict.ControlName
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
					testOngoingAName,
					nil,
				},
				{
					testOngoingCName,
					nil,
				},
				{
					testCompletedAName,
					nil,
				},
				{
					testCompletedCName,
					nil,
				},
				{
					"invalid",
					strict.NewInvalidControlNameError([]string{string(testOngoingAName), string(testOngoingBName), string(testOngoingCName)}),
				},
			},
			[]strict.ControlName{testOngoingAName, testOngoingCName, testCompletedAName, testCompletedCName},
			strict.NewCompletedControlsError([]string{string(testCompletedAName), string(testCompletedCName)}).Error(),
		},
		{
			[]testEnableControl{
				{
					testOngoingBName,
					nil,
				},
				{
					testCompletedBName,
					nil,
				},
			},
			[]strict.ControlName{testOngoingBName, testCompletedBName},
			strict.NewCompletedControlsError([]string{string(testCompletedBName)}).Error(),
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

			controls := newTestControls()

			for _, testEnableControl := range testCase.enableControls {

				err := controls.EnableControl(string(testEnableControl.controlName))

				if testEnableControl.expectedErr != nil {
					require.EqualError(t, err, testEnableControl.expectedErr.Error())

					continue
				}

				require.NoError(t, err)
			}

			var actualEnabledControls []strict.ControlName

			for _, control := range controls {
				if control.Enabled {
					actualEnabledControls = append(actualEnabledControls, control.Name)
				}
			}

			assert.ElementsMatch(t, testCase.expectedEnabledControls, actualEnabledControls)

			logger, output := newTestLogger()

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
			[]strict.ControlName{testOngoingAName, testOngoingBName, testOngoingCName},
		},
		{
			false,
			[]strict.ControlName{},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			controls := newTestControls()

			if testCase.enableStrictMode {
				controls.EnableStrictMode()
			}

			var actualEnabledControls []strict.ControlName

			for _, control := range controls {
				if control.Enabled {
					actualEnabledControls = append(actualEnabledControls, control.Name)
				}
			}

			assert.ElementsMatch(t, testCase.expectedEnabledControls, actualEnabledControls)

			logger, output := newTestLogger()

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
			[]strict.ControlName{testOngoingAName, testOngoingBName},
			[]testEvaluateControl{
				{
					testOngoingAName,
					[]any{"foo", "bar", "baz"},
					errors.Errorf(testOngoingA().ErrorFmt, "foo", "bar", "baz"),
				},
			},
			[]string{""},
		},
		{
			[]strict.ControlName{testOngoingBName},
			[]testEvaluateControl{
				{
					testOngoingBName,
					nil,
					errors.New(testOngoingB().ErrorFmt),
				},
			},
			[]string{""},
		},
		{
			// Testing output warning message once.
			[]strict.ControlName{testOngoingBName},
			[]testEvaluateControl{
				{
					testOngoingAName,
					[]any{"foo", "bar", "baz"},
					nil,
				},
				{
					testOngoingAName,
					[]any{"foo", "bar", "baz"},
					nil,
				},
			},
			[]string{fmt.Sprintf(testOngoingA().WarnFmt, "foo", "bar", "baz")},
		},
		{
			[]strict.ControlName{testCompletedAName},
			[]testEvaluateControl{
				{
					testOngoingAName,
					[]any{"foo", "bar", "baz"},
					nil,
				},
			},
			[]string{fmt.Sprintf(testOngoingA().WarnFmt, "foo", "bar", "baz")},
		},
		{
			[]strict.ControlName{testCompletedAName},
			[]testEvaluateControl{
				{
					testCompletedAName,
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

			logger, output := newTestLogger()
			controls := newTestControls()

			for _, name := range testCase.enableControls {
				controls.EnableControl(string(name))
			}

			for _, evaluateControl := range testCase.evaluateControls {
				err := controls.Evaluate(logger, evaluateControl.controlName, evaluateControl.args...)

				if evaluateControl.expectedErr != nil {
					require.EqualError(t, err, evaluateControl.expectedErr.Error())
					assert.Empty(t, output.String())

					return
				}

				require.NoError(t, err)
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
