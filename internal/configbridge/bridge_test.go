package configbridge

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

func TestNewParsingContextCopiesAutoRetry(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		autoRetry bool
	}{
		{name: "enabled", autoRetry: true},
		{name: "disabled", autoRetry: false},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			opts.AutoRetry = tc.autoRetry

			_, pctx := NewParsingContext(t.Context(), logger.CreateLogger(), opts)

			assert.Equal(t, tc.autoRetry, pctx.AutoRetry)
		})
	}
}
