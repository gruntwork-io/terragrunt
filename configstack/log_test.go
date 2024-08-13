package configstack_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
)

func ptr(str string) *string {
	return &str
}

func TestLogReductionHook(t *testing.T) {
	t.Parallel()
	var hook = configstack.NewForceLogLevelHook(logrus.ErrorLevel)

	stdout := bytes.Buffer{}

	var testLogger = logrus.New()
	testLogger.Out = &stdout
	testLogger.AddHook(hook)
	testLogger.Level = logrus.DebugLevel

	logrus.NewEntry(testLogger).Info("Test tomato")
	logrus.NewEntry(testLogger).Error("666 potato 111")

	out := stdout.String()

	var firstLogEntry = ""
	var secondLogEntry = ""

	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "tomato") {
			firstLogEntry = line
			continue
		}
		if strings.Contains(line, "potato") {
			secondLogEntry = line
			continue
		}
	}
	// check that both entries got logged with error level
	assert.Contains(t, firstLogEntry, "level=error")
	assert.Contains(t, secondLogEntry, "level=error")

}
