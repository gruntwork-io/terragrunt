package strict_test

// Add some basic tests that confirm that by default, a warning is emitted when strict mode is disabled,
// and an error is emitted when a specific control is enabled.
// Make sure to test both when the specific control is enabled, and when the global strict mode is enabled.

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testParentAName     = "test-parent-a"
	testOngoingAName    = "test-ongoing-a"
	testOngoingSubAName = "test-ongoing-sub-a"
	testOngoingBName    = "test-ongoing-b"
	testOngoingCName    = "test-ongoing-c"
	testCompletedAName  = "test-completed-a"
	testCompletedBName  = "test-completed-b"
	testCompletedCName  = "test-completed-c"
)

var (
	testOngoingSubA = func() *controls.Control {
		return &controls.Control{
			Name:    testOngoingSubAName,
			Error:   errors.New("a error ongoing"),
			Warning: "sub a warning ongoing",
		}
	}

	testParentA = func() *controls.Control {
		return &controls.Control{
			Name: testParentAName,
			Subcontrols: strict.Controls{
				testOngoingSubA(),
			},
		}
	}

	testOngoingA = func() *controls.Control {
		return &controls.Control{
			Name: testOngoingAName,
			Subcontrols: strict.Controls{
				testOngoingSubA(),
			},
			Error:   errors.New("a error ongoing"),
			Warning: "a warning ongoing",
		}
	}
	testOngoingB = func() *controls.Control {
		return &controls.Control{
			Name:    testOngoingBName,
			Error:   errors.New("error ongoing b"),
			Warning: "warning ongoing b",
		}
	}
	testOngoingC = func() *controls.Control {
		return &controls.Control{
			Name:    testOngoingCName,
			Error:   errors.New("error ongoing c"),
			Warning: "warning ongoing c",
		}
	}
	testCompletedA = func() *controls.Control {
		return &controls.Control{
			Name:    testCompletedAName,
			Status:  strict.CompletedStatus,
			Error:   errors.New("no matter"),
			Warning: "no matter",
		}
	}
	testCompletedB = func() *controls.Control {
		return &controls.Control{
			Name:    testCompletedBName,
			Status:  strict.CompletedStatus,
			Error:   errors.New("no matter"),
			Warning: "no matter",
		}
	}
	testCompletedC = func() *controls.Control {
		return &controls.Control{
			Name:    testCompletedCName,
			Status:  strict.CompletedStatus,
			Error:   errors.New("no matter"),
			Warning: "no matter",
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
		testParentA(),
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
		controlName string
		expectedErr error
	}

	testCases := []struct {
		enableControls          []testEnableControl
		expectedEnabledControls []string
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
					strict.NewInvalidControlNameError([]string{testOngoingAName, testOngoingBName, testOngoingCName, testParentAName}),
				},
			},
			[]string{testOngoingAName, testOngoingSubAName, testOngoingCName, testCompletedAName, testCompletedCName},
			fmt.Sprintf(strict.CompletedControlsFmt, strict.ControlNames([]string{testCompletedAName, testCompletedCName})),
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
			[]string{testOngoingBName, testCompletedBName},
			fmt.Sprintf(strict.CompletedControlsFmt, strict.ControlNames([]string{testCompletedBName})),
		},
		{
			[]testEnableControl{},
			[]string{},
			"",
		},
		{
			[]testEnableControl{
				{
					testParentAName,
					nil,
				},
			},
			[]string{testParentAName, testOngoingSubAName},
			"",
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			controls := newTestControls()

			for _, testEnableControl := range testCase.enableControls {

				err := controls.EnableControl(testEnableControl.controlName)

				if testEnableControl.expectedErr != nil {
					require.EqualError(t, err, testEnableControl.expectedErr.Error())

					continue
				}

				require.NoError(t, err)
			}

			var actualEnabledControls []string

			for _, control := range controls {
				if control.GetEnabled() {
					actualEnabledControls = append(actualEnabledControls, control.GetName())
				}
				for _, subcontrol := range control.GetSubcontrols() {
					if subcontrol.GetEnabled() {
						actualEnabledControls = append(actualEnabledControls, subcontrol.GetName())
					}
				}

			}

			assert.ElementsMatch(t, testCase.expectedEnabledControls, actualEnabledControls)

			logger, output := newTestLogger()

			controls.LogEnabled(logger)

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
		expectedEnabledControls []string
	}{
		{
			true,
			[]string{testOngoingAName, testOngoingBName, testOngoingCName},
		},
		{
			false,
			[]string{},
		},
	}

	for i, testCase := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			controls := newTestControls()

			if testCase.enableStrictMode {
				controls.FilterByStatus(strict.ActiveStatus).Enable()
			}

			var actualEnabledControls []string

			for _, control := range controls {
				if control.GetEnabled() {
					actualEnabledControls = append(actualEnabledControls, control.GetName())
				}
			}

			assert.ElementsMatch(t, testCase.expectedEnabledControls, actualEnabledControls)

			logger, output := newTestLogger()

			controls.LogEnabled(logger)
			assert.Empty(t, output.String())
		})
	}
}

func TestEvaluateControl(t *testing.T) {
	t.Parallel()

	type testEvaluateControl struct {
		name        string
		expectedErr error
	}

	testCases := []struct {
		enableControls   []string
		evaluateControls []testEvaluateControl
		expectedWarns    []string
	}{
		{
			[]string{testOngoingAName, testOngoingBName},
			[]testEvaluateControl{
				{
					testOngoingAName,
					testOngoingA().Error,
				},
			},
			[]string{""},
		},
		{
			[]string{testOngoingBName},
			[]testEvaluateControl{
				{
					testOngoingBName,
					testOngoingB().Error,
				},
			},
			[]string{""},
		},
		{
			// Testing output warning message once.
			[]string{testOngoingBName},
			[]testEvaluateControl{
				{
					testOngoingAName,
					nil,
				},
				{
					testOngoingAName,
					nil,
				},
			},
			[]string{testOngoingA().Warning, testOngoingSubA().Warning},
		},
		{
			[]string{testCompletedAName},
			[]testEvaluateControl{
				{
					testOngoingAName,
					nil,
				},
			},
			[]string{testOngoingA().Warning, testOngoingSubA().Warning},
		},
		{
			[]string{testCompletedAName},
			[]testEvaluateControl{
				{
					testCompletedAName,
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

			ctx := context.Background()
			ctx = log.ContextWithLogger(ctx, logger)

			for _, name := range testCase.enableControls {
				err := controls.EnableControl(name)
				require.NoError(t, err)
			}

			for _, control := range testCase.evaluateControls {
				err := controls.Find(control.name).Evaluate(ctx)
				if control.expectedErr != nil {
					require.EqualError(t, err, control.expectedErr.Error())
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
