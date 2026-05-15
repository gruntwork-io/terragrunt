package cli_test

import (
	"bytes"
	stdErrors "errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
)

func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	t.Run("nil error returns success exit code", func(t *testing.T) {
		t.Parallel()

		l, _ := newTestLogger()
		assert.Equal(t, 0, cli.ExitCodeFor(l, []string{"terragrunt"}, "1.7.9", nil, 0, newReporter(t)))
		assert.Equal(t, 2, cli.ExitCodeFor(l, []string{"terragrunt"}, "1.7.9", nil, 2, newReporter(t)))
	})

	t.Run("regular error returns 1 with logged message", func(t *testing.T) {
		t.Parallel()

		l, buf := newTestLogger()
		code := cli.ExitCodeFor(l, []string{"terragrunt"}, "1.7.9", stdErrors.New("regular failure"), 0, newReporter(t))

		assert.Equal(t, 1, code)
		assert.Contains(t, buf.String(), "regular failure")
	})

	t.Run("cty function panic routes to reporter and returns 1", func(t *testing.T) {
		t.Parallel()

		l, buf := newTestLogger()
		ctyErr := function.PanicError{Value: "nil deref", Stack: []byte("cty stack")}
		wrapped := fmt.Errorf("evaluating: %w", ctyErr)

		code := cli.ExitCodeFor(l, []string{"terragrunt"}, "1.7.9", wrapped, 0, newReporter(t))

		assert.Equal(t, 1, code)
		assert.Contains(t, buf.String(), "TERRAGRUNT CRASH")
	})
}

// Private helper functions

func newTestLogger() (log.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})

	return log.New(log.WithOutput(buf), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter)), buf
}

func newReporter(t *testing.T) *log.PanicReporter {
	t.Helper()

	tmp := t.TempDir()

	return &log.PanicReporter{
		Now:       func() time.Time { return time.Date(2026, 5, 15, 12, 30, 45, 0, time.UTC) },
		Getwd:     func() (string, error) { return tmp, nil },
		GetPID:    func() int { return 1 },
		WriteFile: os.WriteFile,
		BuildInfo: func() (string, bool) { return "test", false },
	}
}
