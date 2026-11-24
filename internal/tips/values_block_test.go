package tips_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

var tryDefault = config.TryDefault{Kind: "unit", Source: "../catalog/units/app", Name: "tags"}

func TestGiveValuesBlockTipNamesTheComponentAndValue(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	tips.GiveValuesBlockTip(logger, tryDefault, tips.NewTips())

	assert.Contains(t, output.String(), "TIP (values-block)")
	assert.Contains(t, output.String(), "The unit at ../catalog/units/app reads `values.tags` inside `try()`")
}

func TestGiveValuesBlockTipHonorsDisable(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()
	allTips := tips.NewTips()
	allTips.Find(tips.ValuesBlock).Disable()

	tips.GiveValuesBlockTip(logger, tryDefault, allTips)

	assert.Empty(t, output.String())
}
