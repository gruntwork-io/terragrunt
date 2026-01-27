package tips_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTipEvaluate(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	tip := &tips.Tip{
		Name:    "test-tip",
		Message: "This is a test tip message",
	}

	tip.Evaluate(logger)

	assert.Contains(
		t,
		strings.TrimSpace(output.String()),
		"This is a test tip message",
	)
}

func TestTipEvaluateWithNilLogger(t *testing.T) {
	t.Parallel()

	tip := &tips.Tip{
		Name:    "test-tip",
		Message: "This is a test tip message",
	}

	// Should not panic
	tip.Evaluate(nil)
}

func TestTipEvaluateOnNilTip(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()

	var tip *tips.Tip

	// Should not panic
	tip.Evaluate(logger)
}

func TestTipDisable(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	tip := &tips.Tip{
		Name:    "test-tip",
		Message: "This tip should not appear",
	}

	tip.Disable()
	tip.Evaluate(logger)

	assert.Empty(t, output.String())
}

func TestTipOnceShowEnsuresTipShownOnlyOnce(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	tip := &tips.Tip{
		Name:    "test-tip",
		Message: "Once message",
	}

	tip.Evaluate(logger)
	tip.Evaluate(logger)
	tip.Evaluate(logger)

	content := output.String()
	count := strings.Count(content, "Once message")

	assert.Equal(t, 1, count, "Tip should only be shown once per session")
}

func TestTipsDisableAll(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	allTips := tips.NewTips()
	allTips.DisableAll()

	for _, tip := range allTips {
		tip.Evaluate(logger)
	}

	assert.Empty(t, output.String())
}

func TestTipsDisableTip(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	allTips := tips.NewTips()

	err := allTips.DisableTip(tips.DebuggingDocs)
	require.NoError(t, err)

	tip := allTips.Find(tips.DebuggingDocs)
	require.NotNil(t, tip)

	tip.Evaluate(logger)

	assert.Empty(t, output.String())
}

func TestTipsDisableTipInvalidName(t *testing.T) {
	t.Parallel()

	allTips := tips.NewTips()

	err := allTips.DisableTip("invalid-tip-name")

	require.Error(t, err)

	var invalidErr *tips.InvalidTipNameError
	require.ErrorAs(t, err, &invalidErr)
	assert.Contains(t, err.Error(), "invalid tip suppression requested for `--no-tip`: 'invalid-tip-name'")
	assert.Contains(t, err.Error(), "valid tip(s) for suppression:")
	assert.Contains(t, err.Error(), tips.DebuggingDocs)
}

func TestTipsFind(t *testing.T) {
	t.Parallel()

	allTips := tips.NewTips()

	tip := allTips.Find(tips.DebuggingDocs)
	require.NotNil(t, tip)
	assert.Equal(t, tips.DebuggingDocs, tip.Name)
}

func TestTipsFindNonExistent(t *testing.T) {
	t.Parallel()

	allTips := tips.NewTips()

	tip := allTips.Find("non-existent")
	assert.Nil(t, tip)
}

func TestTipsNames(t *testing.T) {
	t.Parallel()

	allTips := tips.NewTips()

	names := allTips.Names()

	assert.Contains(t, names, tips.DebuggingDocs)
}

func TestNewTips(t *testing.T) {
	t.Parallel()

	allTips := tips.NewTips()

	assert.NotEmpty(t, allTips)

	// Verify the debugging-docs tip exists and has the expected message
	tip := allTips.Find(tips.DebuggingDocs)
	require.NotNil(t, tip)
	assert.Contains(t, tip.Message, "troubleshooting")
}

func newTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}
