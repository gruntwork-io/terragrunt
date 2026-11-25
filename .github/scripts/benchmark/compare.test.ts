import { describe, expect, test } from "bun:test";

import {
  addHyperfineExport,
  buildReport,
  type FlaggedMetric,
  flaggedScenarios,
  groupFlagged,
  headline,
  largestPerCommand,
  mannWhitneyP,
  median,
  type MetricKind,
  parseGoBench,
  renderComment,
  renderSummary,
  scenariosToConfirm,
  type ScenarioSamples,
  type Thresholds,
  type Verdict,
} from "./compare.ts";

const thresholds: Thresholds = { time: 5, memory: 10, alloc: 2 };

const steady = [1.0, 1.01, 0.99, 1.02, 0.98, 1.0, 1.01, 0.99];
const slower = steady.map((value) => value * 1.2);
const faster = steady.map((value) => value * 0.8);

function hyperfineSamples(base: number[], head: number[]): Map<string, ScenarioSamples> {
  return new Map([["find.flat.10", new Map([["time_wall_clock", { base, head }]])]]);
}

function report(results: Map<string, ScenarioSamples>, confirm?: Map<string, ScenarioSamples>) {
  return buildReport({
    comparison: "previous",
    baseSha: "a".repeat(40),
    headSha: "b".repeat(40),
    thresholds,
    results,
    confirm,
    gobench: new Map(),
  });
}

describe("median", () => {
  test("takes the middle of an odd count", () => {
    expect(median([3, 1, 2])).toBe(2);
  });

  test("averages the middle pair of an even count", () => {
    expect(median([4, 1, 3, 2])).toBe(2.5);
  });
});

describe("mannWhitneyP", () => {
  test("is small when every head sample exceeds every base sample", () => {
    expect(mannWhitneyP(steady, slower)).toBeLessThan(0.01);
  });

  test("is large for interleaved samples", () => {
    expect(mannWhitneyP([1, 3, 5, 7, 9], [2, 4, 6, 8, 10])).toBeGreaterThan(0.5);
  });

  test("is 1 when every value is the same", () => {
    expect(mannWhitneyP([5, 5, 5], [5, 5, 5])).toBe(1);
  });

  test("is 1 when a side is empty", () => {
    expect(mannWhitneyP([], [1, 2, 3])).toBe(1);
  });

  test("matches the exact test's verdict for constant sides that differ", () => {
    expect(mannWhitneyP([100, 100, 100, 100, 100, 100], [103, 103, 103, 103, 103, 103])).toBeLessThan(0.05);
  });
});

describe("buildReport", () => {
  test("flags a slowdown past the threshold as a regression", () => {
    const result = report(hyperfineSamples(steady, slower));

    expect(result.regressions).toBe(1);
    expect(result.scenarios[0].metrics[0].verdict).toBe("regression");
    expect(result.scenarios[0].metrics[0].deltaPct).toBeCloseTo(20, 5);
  });

  test("flags a speedup past the threshold as an improvement", () => {
    const result = report(hyperfineSamples(steady, faster));

    expect(result.improvements).toBe(1);
    expect(result.scenarios[0].metrics[0].verdict).toBe("improvement");
  });

  test("leaves a significant change below the threshold unflagged", () => {
    const result = report(
      hyperfineSamples(
        steady,
        steady.map((value) => value * 1.04),
      ),
    );

    expect(result.scenarios[0].metrics[0].verdict).toBe("unchanged");
  });

  test("leaves a large but noisy change unflagged", () => {
    const result = report(hyperfineSamples([1, 5, 1, 5, 1, 5], [2, 9, 1, 9, 2, 1]));

    expect(result.scenarios[0].metrics[0].verdict).toBe("unchanged");
  });

  test("keeps a regression that the confirmation pass repeats", () => {
    const result = report(hyperfineSamples(steady, slower), hyperfineSamples(steady, slower));

    expect(result.scenarios[0].metrics[0].verdict).toBe("regression");
  });

  test("downgrades a regression that the confirmation pass does not repeat", () => {
    const result = report(hyperfineSamples(steady, slower), hyperfineSamples(steady, steady));

    expect(result.regressions).toBe(0);
    expect(result.scenarios[0].metrics[0].verdict).toBe("unconfirmed");
  });

  test("downgrades a regression that the confirmation pass did not rerun", () => {
    const result = report(hyperfineSamples(steady, slower), new Map());

    expect(result.scenarios[0].metrics[0].verdict).toBe("unconfirmed");
  });

  test("flags an allocation change without a confirmation pass", () => {
    const gobench = new Map([
      [
        "find.flat.10",
        new Map([["allocs/op", { base: [1000, 1000, 1000, 1000, 1000, 1000], head: [1030, 1030, 1030, 1030, 1030, 1030] }]]),
      ],
    ]);

    const result = buildReport({
      comparison: "previous",
      baseSha: "a",
      headSha: "b",
      thresholds,
      results: new Map(),
      confirm: new Map(),
      gobench,
    });

    expect(result.scenarios[0].metrics[0].verdict).toBe("regression");
  });

  test("compares a phase duration from a Go benchmark as a time metric", () => {
    const gobench = new Map([["ManyEmptyTerragruntPlans/1000_units", new Map([["plan_s/op", { base: steady, head: slower }]])]]);

    const result = buildReport({
      comparison: "release",
      baseSha: "a",
      headSha: "b",
      thresholds,
      results: new Map(),
      gobench,
    });

    expect(result.scenarios[0].metrics[0]).toMatchObject({ name: "plan_s/op", kind: "time", verdict: "regression" });
  });

  test("orders scenarios by unit count as a number", () => {
    const results = new Map<string, ScenarioSamples>();
    for (const units of [1000, 10, 100, 1]) {
      results.set(`find.flat.${units}`, new Map([["time_wall_clock", { base: steady, head: steady }]]));
    }

    expect(report(results).scenarios.map((scenario) => scenario.id)).toEqual([
      "find.flat.1",
      "find.flat.10",
      "find.flat.100",
      "find.flat.1000",
    ]);
  });
});

describe("scenariosToConfirm", () => {
  test("lists scenarios with a flagged hyperfine metric", () => {
    const results = hyperfineSamples(steady, slower);
    results.set("find.flat.100", new Map([["time_wall_clock", { base: steady, head: steady }]]));

    expect(scenariosToConfirm(report(results))).toEqual(["find.flat.10"]);
  });
});

describe("flaggedScenarios", () => {
  test("lists a scenario that only a Go benchmark metric flags", () => {
    const allocs = { base: [100, 100, 100, 100], head: [110, 110, 110, 110] };
    const gobench = new Map([["find.flat.10", new Map([["allocs/op", allocs]])]]);
    const built = buildReport({
      comparison: "previous",
      baseSha: "a".repeat(40),
      headSha: "b".repeat(40),
      thresholds,
      results: new Map(),
      gobench,
    });

    expect(scenariosToConfirm(built)).toEqual([]);
    expect(flaggedScenarios(built)).toEqual(["find.flat.10"]);
  });
});

describe("largestPerCommand", () => {
  test("keeps the scenario with the most units of each command", () => {
    const ids = ["find.deps.1", "find.deps.1000", "find.flat.100", "run-all-plan.flat.10", "run-all-plan.deps.1"];

    expect(largestPerCommand(ids, 3)).toEqual(["find.deps.1000", "run-all-plan.flat.10"]);
  });

  test("keeps the first of two scenarios with the same units", () => {
    expect(largestPerCommand(["find.deps.1000", "find.flat.1000"], 3)).toEqual(["find.deps.1000"]);
  });

  test("keeps an id that is not a scenario id", () => {
    expect(largestPerCommand(["find.flat.10", "ParseConfig"], 3)).toEqual(["find.flat.10", "ParseConfig"]);
  });

  test("stops at the limit", () => {
    expect(largestPerCommand(["find.flat.1", "find-dependencies.flat.1", "run-all-plan.flat.1"], 2)).toEqual([
      "find.flat.1",
      "find-dependencies.flat.1",
    ]);
  });
});

describe("parseGoBench", () => {
  const output = [
    "goos: linux",
    "pkg: github.com/gruntwork-io/terragrunt/test/benchmarks",
    "BenchmarkScenario/find/shape=flat/units=10-32         \t     400\t   2500000 ns/op\t 1511960 B/op\t   34833 allocs/op",
    "BenchmarkScenario/run-all-plan/shape=deps/units=1000-4 \t       1\t1149678616 ns/op\t3624764968 B/op\t37036053 allocs/op",
    "BenchmarkEmptyTerragruntInit/1_units-32 \t 10\t 100 ns/op",
    "PASS",
  ].join("\n");

  test("keys samples by scenario id and converts ns/op to seconds", () => {
    const into = new Map<string, ScenarioSamples>();

    parseGoBench(output, "head", into);

    expect([...into.keys()]).toEqual(["find.flat.10", "run-all-plan.deps.1000", "EmptyTerragruntInit/1_units"]);
    expect(into.get("find.flat.10")?.get("sec/op")?.head).toEqual([0.0025]);
    expect(into.get("find.flat.10")?.get("B/op")?.head).toEqual([1511960]);
    expect(into.get("run-all-plan.deps.1000")?.get("allocs/op")?.head).toEqual([37036053]);
  });

  test("keeps the phase durations that the benchmark helpers report", () => {
    const into = new Map<string, ScenarioSamples>();

    parseGoBench(
      "BenchmarkManyEmptyTerragruntPlans/1000_units-4 \t 1\t 9000000000 ns/op\t 8.75 plan_s/op\t 12.5 MB/s\t 100 B/op",
      "base",
      into,
    );

    const metrics = into.get("ManyEmptyTerragruntPlans/1000_units");

    expect([...(metrics?.keys() ?? [])]).toEqual(["sec/op", "plan_s/op", "B/op"]);
    expect(metrics?.get("plan_s/op")?.base).toEqual([8.75]);
  });

  test("skips a line that names a benchmark but has no iteration count", () => {
    const into = new Map<string, ScenarioSamples>();

    parseGoBench("BenchmarkScenario/find/shape=flat/units=10 took 12 ns/op", "head", into);

    expect(into.size).toBe(0);
  });

  test("appends a second pass to the same side", () => {
    const into = new Map<string, ScenarioSamples>();

    parseGoBench(output, "base", into);
    parseGoBench(output, "base", into);

    expect(into.get("find.flat.10")?.get("allocs/op")).toEqual({ base: [34833, 34833], head: [] });
  });
});

describe("addHyperfineExport", () => {
  test("reads each measurement of each metric", () => {
    const into = new Map<string, ScenarioSamples>();
    const measurement = (wall: number) => ({
      time_wall_clock: { value: wall, unit: "second" },
      time_cpu: { value: wall * 2, unit: "second" },
      memory_peak_resident: { value: 1024, unit: "byte" },
      exit_code: 0,
    });

    addHyperfineExport({ results: [{ measurements: [measurement(0.1), measurement(0.2)] }] }, "find.flat.1", "base", into);

    expect(into.get("find.flat.1")?.get("time_wall_clock")).toEqual({ base: [0.1, 0.2], head: [] });
    expect(into.get("find.flat.1")?.get("time_cpu")?.base).toEqual([0.2, 0.4]);
    expect(into.get("find.flat.1")?.get("memory_peak_resident")?.base).toEqual([1024, 1024]);
  });
});

describe("renderSummary", () => {
  test("lists a flagged metric with its profile diffs", () => {
    const summary = renderSummary(report(hyperfineSamples(steady, slower)), {
      profileDiffs: new Map([
        [
          "find.flat.10",
          { cpu: "flat  flat%\n0.38s 0.88% runtime.scanObject", mem: "flat  flat%\n7MB 5.45% regexp.compile" },
        ],
      ]),
    });

    expect(summary).toContain("1 regressed and 0 improved metrics across 1 scenarios");
    expect(summary).toContain("| `find.flat.10` | hyperfine time_wall_clock | 1.00 s | 1.20 s | +20.0% |");
    expect(summary).toContain("<summary>CPU profile diff for <code>find.flat.10</code></summary>");
    expect(summary).toContain("runtime.scanObject");
    expect(summary).toContain("<summary>Allocation profile diff for <code>find.flat.10</code></summary>");
    expect(summary).toContain("regexp.compile");
  });

  test("leaves out the diff a scenario has no profile for", () => {
    const summary = renderSummary(report(hyperfineSamples(steady, slower)), {
      profileDiffs: new Map([["find.flat.10", { mem: "flat  flat%\n7MB 5.45% regexp.compile" }]]),
    });

    expect(summary).not.toContain("CPU profile diff");
    expect(summary).toContain("Allocation profile diff");
  });

  test("shows no profile diff for a scenario that is not flagged", () => {
    const summary = renderSummary(report(hyperfineSamples(steady, steady)), {
      profileDiffs: new Map([["find.flat.10", { cpu: "flat  flat%\n0.38s 0.88% runtime.scanObject" }]]),
    });

    expect(summary).not.toContain("profile diff");
  });

  test("omits the flagged table when nothing is flagged", () => {
    const summary = renderSummary(report(hyperfineSamples(steady, steady)), {});

    expect(summary).not.toContain("| Scenario | Metric |");
    expect(summary).toContain("| `find.flat.10` | 1.00 s +0.0% |");
  });

  test("links the commits between base and head", () => {
    const summary = renderSummary(report(hyperfineSamples(steady, steady)), {
      repoUrl: "https://github.com/gruntwork-io/terragrunt",
    });

    expect(summary).toContain(
      `Base \`${"a".repeat(12)}\`, head \`${"b".repeat(12)}\` ` +
        `([compare](https://github.com/gruntwork-io/terragrunt/compare/${"a".repeat(40)}...${"b".repeat(40)})). `,
    );
  });

  test("names the commits without a link when the repository is unknown", () => {
    const summary = renderSummary(report(hyperfineSamples(steady, steady)), {});

    expect(summary).toContain(`Base \`${"a".repeat(12)}\`, head \`${"b".repeat(12)}\`. `);
  });

  test("names the commits without a link when base and head are the same commit", () => {
    const self = buildReport({
      comparison: "self",
      baseSha: "a".repeat(40),
      headSha: "a".repeat(40),
      thresholds,
      results: hyperfineSamples(steady, steady),
      gobench: new Map(),
    });

    const summary = renderSummary(self, { repoUrl: "https://github.com/gruntwork-io/terragrunt" });

    expect(summary).toContain(`Base \`${"a".repeat(12)}\`, head \`${"a".repeat(12)}\`. `);
    expect(summary).not.toContain("/compare/");
  });
});

function flaggedMetric(
  id: string,
  kind: MetricKind,
  verdict: Verdict,
  deltaPct: number,
  name = "sec/op",
): FlaggedMetric {
  return {
    id,
    metric: {
      name,
      kind,
      unit: "seconds",
      source: "gobench",
      base: 1,
      head: 1 + deltaPct / 100,
      deltaPct,
      p: 0.01,
      verdict,
    },
  };
}

describe("headline", () => {
  test("lists every kind of metric that regressed", () => {
    const flagged = [
      flaggedMetric("find.flat.1", "time", "regression", 20),
      flaggedMetric("find.flat.1", "alloc", "regression", 5, "allocs/op"),
    ];

    expect(headline(flagged)).toBe("This branch is slower and allocates more than `main`.");
  });

  test("names what got better after what got worse", () => {
    const flagged = [
      flaggedMetric("find.flat.1", "time", "regression", 20),
      flaggedMetric("find.flat.1", "alloc", "improvement", -5, "allocs/op"),
    ];

    expect(headline(flagged)).toBe("This branch is slower than `main`, but allocates less.");
  });

  test("calls a kind of metric that moved both ways mixed", () => {
    const flagged = [
      flaggedMetric("find.flat.1", "time", "regression", 20),
      flaggedMetric("run-all-plan.flat.1", "time", "improvement", -20),
    ];

    expect(headline(flagged)).toBe("Results against `main` are mixed for time.");
  });

  test("says nothing moved when nothing is flagged", () => {
    expect(headline([])).toBe("No metric moved against `main`.");
  });
});

describe("groupFlagged", () => {
  test("keeps the fewest units and the largest change of a metric across the shapes of a command", () => {
    const deps10 = flaggedMetric("find-dependencies.deps.10", "time", "regression", 20);
    const deps1000 = flaggedMetric("find-dependencies.deps.1000", "time", "regression", 50);
    const flat1 = flaggedMetric("find-dependencies.flat.1", "time", "regression", 10);

    expect(groupFlagged([deps10, deps1000, flat1])).toEqual([{ first: flat1, largest: deps1000 }]);
  });

  test("keeps regressions and improvements of a metric apart", () => {
    const slower = flaggedMetric("find.flat.1", "time", "regression", 20);
    const faster = flaggedMetric("find.deps.1", "time", "improvement", -20);

    expect(groupFlagged([slower, faster])).toEqual([
      { first: slower, largest: slower },
      { first: faster, largest: faster },
    ]);
  });

  test("keeps another command apart", () => {
    const find = flaggedMetric("find.flat.1", "time", "regression", 20);
    const plan = flaggedMetric("run-all-plan.flat.1", "time", "regression", 20);

    expect(groupFlagged([find, plan])).toHaveLength(2);
  });
});

describe("renderComment", () => {
  const gobench = new Map<string, ScenarioSamples>(
    [
      [1, 1],
      [10, 1.1],
      [1000, 1.5],
    ].map(([units, factor]) => [
      `find.flat.${units}`,
      new Map([["sec/op", { base: steady, head: steady.map((value) => value * factor) }]]),
    ]),
  );

  const comment = renderComment(
    buildReport({
      comparison: "push",
      baseSha: "a".repeat(40),
      headSha: "b".repeat(40),
      thresholds,
      results: new Map(),
      gobench,
    }),
  );

  test("leads with how the branch compares with main", () => {
    expect(comment.startsWith("## Benchmark\n\n**This branch is slower than `main`.**\n")).toBe(true);
  });

  test("lists a metric once, at the fewest units that flag it", () => {
    expect(comment).toContain(
      "| `find.flat.10` | gobench sec/op | 1.00 s | 1.10 s | +10.0% | +50.0% in `find.flat.1000` |",
    );
    expect(comment).not.toContain("| `find.flat.1000` | gobench sec/op |");
  });

  test("still shows every scenario in the collapsed table", () => {
    expect(comment).toContain("| `find.flat.1000` |  |  |  | 1.50 s +50.0% (regression) |");
  });
});
