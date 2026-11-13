package tips

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveOutputsBlockTip emits the OutputsBlock tip for the dependency target at configPath without an outputs block.
func GiveOutputsBlockTip(l log.Logger, configPath string, allTips Tips) {
	allTips.Find(OutputsBlock).EvaluateWith(l, fmt.Sprintf(
		"A `dependency` block reads %s, which declares no `outputs` block. %s",
		configPath,
		outputsBlockFix,
	))
}

// GiveOutputsBlockTips emits the OutputsBlock tip for each unit a dependency of cfg reads without an outputs block.
func GiveOutputsBlockTips(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	allTips Tips,
) error {
	targets, err := config.UnitsWithoutOutputs(ctx, l, v, pctx, cfg)
	if err != nil {
		return err
	}

	for _, target := range targets {
		GiveOutputsBlockTip(l, target, allTips)
	}

	return nil
}
