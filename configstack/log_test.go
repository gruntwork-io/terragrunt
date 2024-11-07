package configstack_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"

	"github.com/stretchr/testify/assert"
)

func ptr(str string) *string {
	return &str
}

func TestLogReductionHook(t *testing.T) {
	t.Parallel()
	var hook = configstack.NewForceLogLevelHook(log.ErrorLevel)

	stdout := bytes.Buffer{}

	formatter := format.NewFormatter(format.NewKeyValueFormat())
	formatter.DisableColors()

	var testLogger = log.New(
		log.WithOutput(&stdout),
		log.WithHooks(hook),
		log.WithLevel(log.DebugLevel),
		log.WithFormatter(formatter),
	)

	testLogger.Info("Test tomato")
	testLogger.Error("666 potato 111")

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
