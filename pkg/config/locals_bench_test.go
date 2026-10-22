package config_test

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

// BenchmarkEvaluateLocalsBlock measures evaluating a locals block of n locals
// shaped as a chain, where each local references the one before it, and as a
// flat block, where no local references another.
func BenchmarkEvaluateLocalsBlock(b *testing.B) {
	for _, shape := range []struct {
		local func(i int) string
		name  string
	}{
		{
			local: func(i int) string {
				if i == 0 {
					return `"x"`
				}

				return fmt.Sprintf(`"${local.l%d}x"`, i-1)
			},
			name: "chain",
		},
		{
			local: func(int) string {
				return `"x"`
			},
			name: "flat",
		},
	} {
		for _, n := range []int{10, 100, 500} {
			var sb strings.Builder

			sb.WriteString("locals {\n")

			for i := range n {
				fmt.Fprintf(&sb, "  l%d = %s\n", i, shape.local(i))
			}

			sb.WriteString("}\n")

			file, err := hclparse.NewParser().ParseFromString(sb.String(), config.DefaultTerragruntConfigPath)
			require.NoError(b, err)

			b.Run(shape.name+"/n="+strconv.Itoa(n), func(b *testing.B) {
				l := logger.CreateLogger()
				l.SetOptions(log.WithOutput(io.Discard))

				v := venvtest.NewWithOSFS()
				ctx, pctx := newTestParsingContext(b, config.DefaultTerragruntConfigPath)

				for b.Loop() {
					locals, err := config.EvaluateLocalsBlock(ctx, l, v, pctx, file)
					require.NoError(b, err)
					require.Len(b, locals, n)
				}
			})
		}
	}
}
