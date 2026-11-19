package test_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/fixture"
	"github.com/gruntwork-io/terragrunt/test/benchmarks/helpers"
)

// BenchmarkScenario runs the commands that .github/scripts/benchmark/run-hyperfine.sh times, in
// process and against faketofu. .github/scripts/benchmark/compare.ts parses the sub-benchmark names
// into scenario ids, so they keep the form <command>/shape=<shape>/units=<count>.
func BenchmarkScenario(b *testing.B) {
	helpers.UseFakeTofu(b)

	commands := []struct {
		name string
		args []string
	}{
		{name: "find", args: []string{"find"}},
		{name: "find-dependencies", args: []string{"find", "--json", "--dependencies"}},
		{name: "run-all-plan", args: []string{"run", "--all", "plan", "--non-interactive"}},
	}

	for _, shape := range fixture.Shapes() {
		for _, units := range []int{1, 10, 100, 1000} {
			dir := b.TempDir()
			helpers.WriteFixture(b, dir, shape, units)

			for _, command := range commands {
				args := append([]string{"terragrunt"}, command.args...)
				args = append(args, "--working-dir", dir)

				b.Run(fmt.Sprintf("%s/shape=%s/units=%d", command.name, shape, units), func(b *testing.B) {
					b.ReportAllocs()

					for b.Loop() {
						helpers.RunTerragruntCommand(b, args...)
					}
				})
			}
		}
	}
}
