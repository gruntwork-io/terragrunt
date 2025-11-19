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
		expectedErr error
		controlName string
	}

	testCases := []struct {
		expectedCompletedMsg    string
		enableControls          []testEnableControl
		expectedEnabledControls []string
	}{
		{
			enableControls: []testEnableControl{
				{
					controlName: testOngoingAName,
				},
				{
					controlName: testOngoingCName,
				},
				{
					controlName: testCompletedAName,
				},
				{
					controlName: testCompletedCName,
				},
				{
					controlName: "invalid",
					expectedErr: strict.NewInvalidControlNameError([]string{testOngoingAName, testOngoingBName, testOngoingCName, testParentAName}),
				},
			},
			expectedEnabledControls: []string{testOngoingAName, testOngoingSubAName, testOngoingCName, testCompletedAName, testCompletedCName},
			expectedCompletedMsg:    fmt.Sprintf(strict.CompletedControlsFmt, strict.ControlNames([]string{testCompletedAName, testCompletedCName})),
		},
		{
			enableControls: []testEnableControl{
				{
					controlName: testOngoingBName,
				},
				{
					controlName: testCompletedBName,
				},
			},
			expectedEnabledControls: []string{testOngoingBName, testCompletedBName},
			expectedCompletedMsg:    fmt.Sprintf(strict.CompletedControlsFmt, strict.ControlNames([]string{testCompletedBName})),
		},
		{
			enableControls:          []testEnableControl{},
			expectedEnabledControls: []string{},
		},
		{
			enableControls: []testEnableControl{
				{
					controlName: testParentAName,
				},
			},
			expectedEnabledControls: []string{testParentAName, testOngoingSubAName},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			controls := newTestControls()

			for _, testEnableControl := range tc.enableControls {
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

			assert.ElementsMatch(t, tc.expectedEnabledControls, actualEnabledControls)

			logger, output := newTestLogger()

			controls.LogEnabled(logger)

			if tc.expectedCompletedMsg == "" {
				assert.Empty(t, output.String())

				return
			}

			assert.Contains(t, strings.TrimSpace(output.String()), tc.expectedCompletedMsg)
		})
	}
}

func TestEnableStrictMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedEnabledControls []string
		enableStrictMode        bool
	}{
		{
			enableStrictMode:        true,
			expectedEnabledControls: []string{testParentAName, testOngoingSubAName, testOngoingAName, testOngoingSubAName, testOngoingBName, testOngoingCName},
		},
		{
			enableStrictMode:        false,
			expectedEnabledControls: []string{},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			controls := newTestControls()

			if tc.enableStrictMode {
				controls.FilterByStatus(strict.ActiveStatus).Enable()
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

			assert.ElementsMatch(t, tc.expectedEnabledControls, actualEnabledControls)

			logger, output := newTestLogger()

			controls.LogEnabled(logger)
			assert.Empty(t, output.String())
		})
	}
}

func TestEvaluateControl(t *testing.T) {
	t.Parallel()

	type testEvaluateControl struct {
		expectedErr error
		name        string
	}

	testCases := []struct {
		enableControls   []string
		evaluateControls []testEvaluateControl
		expectedWarns    []string
	}{
		{
			enableControls: []string{testOngoingAName, testOngoingBName},
			evaluateControls: []testEvaluateControl{
				{
					name:        testOngoingAName,
					expectedErr: testOngoingA().Error,
				},
			},
			expectedWarns: []string{""},
		},
		{
			enableControls: []string{testOngoingBName},
			evaluateControls: []testEvaluateControl{
				{
					name:        testOngoingBName,
					expectedErr: testOngoingB().Error,
				},
			},
			expectedWarns: []string{""},
		},
		{
			// Testing output warning message once.
			enableControls: []string{testOngoingBName},
			evaluateControls: []testEvaluateControl{
				{
					name: testOngoingAName,
				},
				{
					name: testOngoingAName,
				},
			},
			expectedWarns: []string{testOngoingA().Warning, testOngoingSubA().Warning},
		},
		{
			enableControls: []string{testCompletedAName},
			evaluateControls: []testEvaluateControl{
				{
					name: testOngoingAName,
				},
			},
			expectedWarns: []string{testOngoingA().Warning, testOngoingSubA().Warning},
		},
		{
			enableControls: []string{testCompletedAName},
			evaluateControls: []testEvaluateControl{
				{
					name: testCompletedAName,
				},
			},
			expectedWarns: []string{""},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			logger, output := newTestLogger()
			controls := newTestControls()

			ctx := t.Context()
			ctx = log.ContextWithLogger(ctx, logger)

			for _, name := range tc.enableControls {
				err := controls.EnableControl(name)
				require.NoError(t, err)
			}

			for _, control := range tc.evaluateControls {
				err := controls.Find(control.name).Evaluate(ctx)
				if control.expectedErr != nil {
					require.EqualError(t, err, control.expectedErr.Error())
					assert.Empty(t, output.String())

					return
				}

				require.NoError(t, err)
			}

			if len(tc.expectedWarns) == 0 {
				assert.Empty(t, output.String())

				return
			}

			actualWarns := strings.Split(strings.TrimSpace(output.String()), "\n")
			assert.ElementsMatch(t, actualWarns, tc.expectedWarns)
		})
	}
}
