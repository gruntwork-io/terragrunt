package queue_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/stretchr/testify/require"
)

// BenchmarkNewQueue measures ordering n units shaped as a chain, where each
// unit depends on the one before it, and as a fan-in, where every unit
// depends on one shared root, for both apply and destroy.
func BenchmarkNewQueue(b *testing.B) {
	for _, cmd := range []string{"apply", "destroy"} {
		for _, shape := range []struct {
			dependency func(i int) int
			name       string
		}{
			{
				dependency: func(i int) int { return i - 1 },
				name:       "chain",
			},
			{
				dependency: func(int) int { return 0 },
				name:       "fan-in",
			},
		} {
			for _, n := range []int{10, 100, 500} {
				b.Run(cmd+"/"+shape.name+"/n="+strconv.Itoa(n), func(b *testing.B) {
					units := make(component.Components, n)

					for i := range units {
						unit := component.NewUnit(fmt.Sprintf("unit-%04d", i))
						unit.SetDiscoveryContext(&component.DiscoveryContext{Cmd: cmd})

						if i > 0 {
							unit.AddDependency(units[shape.dependency(i)])
						}

						units[i] = unit
					}

					for b.Loop() {
						q, err := queue.NewQueue(units)
						require.NoError(b, err)
						require.Len(b, q.Entries, n)
					}
				})
			}
		}
	}
}
