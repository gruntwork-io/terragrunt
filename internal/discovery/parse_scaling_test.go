package discovery_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscovery_CascadeFilterParseScaling is a regression test for issue #6323:
// `terragrunt find --filter ...<unit>` with K cascade (reverse-dependency)
// filters scales linearly in K because the upstream dependent walk re-parses
// every unit config once per graph target instead of hitting the per-component
// parse cache.
//
// The test counts REAL HCL parses by counting telemetry spans named
// discovery_parse_component (cache hits emit a counter, not a span — see
// internal/discovery/phase_parse.go). The regression contract: within a single
// discovery run, no config path is parsed more than once.
func TestDiscovery_CascadeFilterParseScaling(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// git init is REQUIRED: gitRoot must be set for the upstream dependent
	// walk (discoverDependentsUpstream) to run at all.
	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	const (
		targetCount    = 5
		unrelatedCount = 10
	)

	unitCount := 2*targetCount + unrelatedCount

	writeUnit := func(name, content string) string {
		dir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(content), 0644))

		return dir
	}

	var (
		cascadeFilters []string
		wantCascade    []string
	)

	// Targets t1..t5 (empty config) and dependents d1..d5 where d_i depends on ../t_i.
	for i := 1; i <= targetCount; i++ {
		target := fmt.Sprintf("t%d", i)
		wantCascade = append(
			wantCascade,
			writeUnit(target, ``),
			writeUnit(fmt.Sprintf("d%d", i), fmt.Sprintf(`
dependency "target" {
	config_path = "../%s"
}
`, target)),
		)
		cascadeFilters = append(cascadeFilters, "..."+target)
	}

	// Unrelated units u1..u10 (empty config) inflate the per-walk parse cost.
	for i := 1; i <= unrelatedCount; i++ {
		writeUnit(fmt.Sprintf("u%d", i), ``)
	}

	// Run A: a single cascade filter.
	unitsA, parsesA := runCascadeDiscoveryCountingParses(t, tmpDir, cascadeFilters[:1])
	assert.ElementsMatch(t, wantCascade[:2], unitsA, "...t1 should find t1 and its dependent d1")

	// Run B: K=5 cascade filters over the same fixture, fresh Discovery and Telemeter.
	unitsB, parsesB := runCascadeDiscoveryCountingParses(t, tmpDir, cascadeFilters)
	assert.ElementsMatch(t, wantCascade, unitsB, "...t1...t5 should find every target and its dependent")

	totalA, maxPathA, maxCountA := summarizeParseCounts(parsesA)
	totalB, maxPathB, maxCountB := summarizeParseCounts(parsesB)

	require.NotZero(t, totalA, "Run A produced no discovery_parse_component spans; the fixture is not exercising parsing")
	require.NotZero(t, totalB, "Run B produced no discovery_parse_component spans; the fixture is not exercising parsing")

	t.Logf("Run A (K=1): %d parses total, max per path %d (%s), per-path: %v",
		totalA, maxCountA, maxPathA, parsesA)
	t.Logf("Run B (K=%d): %d parses total, max per path %d (%s), per-path: %v",
		targetCount, totalB, maxCountB, maxPathB, parsesB)

	// Regression contract for issue #6323: within a single discovery run, no
	// config path may be parsed more than once. On the buggy code the upstream
	// dependent walk parses fresh components (created by createComponentFromPath)
	// before mapping them to canonical ones, so the parse cache never hits and
	// every graph target re-parses the whole tree: O(K x N) parses.
	assert.LessOrEqualf(t, maxCountB, 1,
		"issue #6323: config %s was parsed %d times within a single discovery run with %d cascade filters; "+
			"each config must be parsed at most once per run",
		maxPathB, maxCountB, targetCount)

	// Total parses must stay near the unit count instead of scaling with the
	// number of cascade filters (K). The +2 is headroom for incidental parses
	// of configs outside the unit set (e.g. at the fixture root); the per-path
	// assertion above carries the regression contract.
	assert.LessOrEqualf(t, totalB, unitCount+2,
		"issue #6323: %d total parses for %d units with %d cascade filters; "+
			"parse count must not scale with the number of cascade filters (K=1 run parsed %d times)",
		totalB, unitCount, targetCount, totalA)
}

// runCascadeDiscoveryCountingParses runs a fresh discovery over tmpDir with the
// given filter queries and a dedicated Telemeter, then returns the discovered
// unit paths and the number of discovery_parse_component spans per config path.
func runCascadeDiscoveryCountingParses(t *testing.T, tmpDir string, queries []string) ([]string, map[string]int) {
	t.Helper()

	l := logger.CreateLogger()

	var buf bytes.Buffer

	tlm, err := telemetry.NewTelemeter(t.Context(), l, "terragrunt", "v0.0.0-test", &buf, &telemetry.Options{
		TraceExporter: "console",
	})
	require.NoError(t, err)
	require.NotNil(t, tlm)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	filters, err := filter.ParseFilterQueries(l, queries)
	require.NoError(t, err)

	opts := &options.TerragruntOptions{
		WorkingDir:     tmpDir,
		RootWorkingDir: tmpDir,
		// Env must be non-nil: with a Telemeter in the context, gitRoot
		// detection writes the traceparent into opts.Env.
		Env: map[string]string{},
	}

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir}).
		WithSuppressParseErrors().
		WithFilters(filters)

	components, err := d.Discover(ctx, l, opts)
	require.NoError(t, err)

	require.NoError(t, tlm.Shutdown(ctx))

	return components.Filter(component.UnitKind).Paths(), countParseSpansByPath(t, &buf)
}

// countParseSpansByPath decodes the console trace exporter output in buf and
// counts spans named discovery_parse_component grouped by their "path"
// attribute. Cache hits emit a counter rather than a span, so each span is one
// actual HCL parse.
func countParseSpansByPath(t *testing.T, buf *bytes.Buffer) map[string]int {
	t.Helper()

	// Value is any rather than string because parse spans carry bool and int
	// attributes (cache_hit, depth) alongside the string path.
	type span struct {
		Name       string `json:"Name"`
		Attributes []struct {
			Value struct {
				Value any `json:"Value"`
			} `json:"Value"`
			Key string `json:"Key"`
		} `json:"Attributes"`
	}

	counts := map[string]int{}

	dec := json.NewDecoder(buf)
	for dec.More() {
		var s span
		require.NoError(t, dec.Decode(&s))

		if s.Name != "discovery_parse_component" {
			continue
		}

		for _, attr := range s.Attributes {
			if attr.Key != "path" {
				continue
			}

			path, ok := attr.Value.Value.(string)
			require.True(t, ok, "discovery_parse_component attribute %q must be a string", attr.Key)

			counts[path]++
		}
	}

	return counts
}

// summarizeParseCounts returns the total parse count and the path with the
// highest per-path count.
func summarizeParseCounts(counts map[string]int) (total int, maxPath string, maxCount int) {
	for path, count := range counts {
		total += count

		if count > maxCount {
			maxCount = count
			maxPath = path
		}
	}

	return total, maxPath, maxCount
}
