package tips_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/stretchr/testify/assert"
)

func TestGiveOutputsBlockTipNamesTheUnit(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()

	tips.GiveOutputsBlockTip(logger, "/live/vpc/terragrunt.hcl", tips.NewTips())

	assert.Contains(t, output.String(), "TIP (outputs-block)")
	assert.Contains(t, output.String(), "/live/vpc/terragrunt.hcl")
}

func TestGiveOutputsBlockTipHonorsDisable(t *testing.T) {
	t.Parallel()

	logger, output := newTestLogger()
	allTips := tips.NewTips()
	allTips.Find(tips.OutputsBlock).Disable()

	tips.GiveOutputsBlockTip(logger, "/live/vpc/terragrunt.hcl", allTips)

	assert.Empty(t, output.String())
}
