/**
 * Compares benchmark samples from a base and a head build of Terragrunt and
 * writes a JSON report, a Markdown summary, and the list of scenarios to rerun.
 *
 * Usage:
 *   bun compare.ts --out <report.json> --summary <summary.md>
 *     [--results <dir>] [--flagged <ids.txt>] [--confirm <dir>] [--gobench <dir>]
 *     [--benchstat <file>] [--profiles <dir>] [--comment <comment.md>]
 *
 * --comment writes a shorter summary for notify-pr.ts to post on a pull request.
 *
 * --results and --confirm hold <scenario>/<side>-<round>.json files written by
 * run-hyperfine.sh. --gobench holds base.txt and head.txt written by
 * run-go-bench.sh.
 *
 * Environment:
 *   BENCH_COMPARISON             Label for the report, such as `push, fast suite`.
 *   BENCH_BASE_SHA, BENCH_HEAD_SHA
 *   BENCH_TIME_THRESHOLD_PCT     Smallest change in a time metric to flag (default 5).
 *   BENCH_MEMORY_THRESHOLD_PCT   Smallest change in peak RSS to flag (default 10).
 *   BENCH_ALLOC_THRESHOLD_PCT    Smallest change in B/op or allocs/op to flag (default 2).
 */

import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { parseArgs } from "node:util";

export type Side = "base" | "head";
export type Samples = Record<Side, number[]>;
export type MetricKind = "time" | "memory" | "alloc";
export type Source = "hyperfine" | "gobench";
export type Verdict = "regression" | "improvement" | "unconfirmed" | "unchanged";

export type MetricSpec = {
  name: string;
  kind: MetricKind;
  unit: "seconds" | "bytes" | "count";
  source: Source;
};

export type Thresholds = Record<MetricKind, number>;

export type MetricResult = MetricSpec & {
  base: number;
  head: number;
  deltaPct: number;
  p: number;
  verdict: Verdict;
};

export type ScenarioResult = { id: string; metrics: MetricResult[] };

export type Report = {
  comparison: string;
  baseSha: string;
  headSha: string;
  thresholds: Thresholds;
  regressions: number;
  improvements: number;
  scenarios: ScenarioResult[];
};

/** Samples for one scenario, keyed by metric name. */
export type ScenarioSamples = Map<string, Samples>;

/**
 * A change counts only when a rank test rejects "both sides draw from the same
 * distribution" at this level.
 */
export const SIGNIFICANCE = 0.05;

/**
 * Profile diffs included in the pull request comment. A GitHub comment holds
 * 65,536 characters, and the run summary has the rest.
 */
const COMMENT_PROFILE_DIFFS = 3;

export const HYPERFINE_METRICS: MetricSpec[] = [
  { name: "time_wall_clock", kind: "time", unit: "seconds", source: "hyperfine" },
  { name: "time_cpu", kind: "time", unit: "seconds", source: "hyperfine" },
  { name: "memory_peak_resident", kind: "memory", unit: "bytes", source: "hyperfine" },
];

export const GOBENCH_METRICS: MetricSpec[] = [
  { name: "sec/op", kind: "time", unit: "seconds", source: "gobench" },
  { name: "B/op", kind: "alloc", unit: "bytes", source: "gobench" },
  { name: "allocs/op", kind: "alloc", unit: "count", source: "gobench" },
];

export function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);

  if (sorted.length % 2 === 1) {
    return sorted[mid];
  }

  return (sorted[mid - 1] + sorted[mid]) / 2;
}

/**
 * Two-sided Mann-Whitney U test, using the normal approximation with tie and
 * continuity corrections. Returns 1 when either side is empty or every value
 * is the same.
 */
export function mannWhitneyP(a: number[], b: number[]): number {
  const n1 = a.length;
  const n2 = b.length;

  if (n1 === 0 || n2 === 0) {
    return 1;
  }

  const all = [...a.map((value) => ({ value, first: true })), ...b.map((value) => ({ value, first: false }))].sort(
    (x, y) => x.value - y.value,
  );

  let rankSum = 0;
  let tieTerm = 0;

  for (let start = 0; start < all.length; ) {
    let end = start;

    while (end < all.length && all[end].value === all[start].value) {
      end++;
    }

    // Tied values share the mean of the 1-based ranks they span.
    const rank = (start + 1 + end) / 2;
    const ties = end - start;

    tieTerm += ties ** 3 - ties;

    for (let i = start; i < end; i++) {
      if (all[i].first) {
        rankSum += rank;
      }
    }

    start = end;
  }

  const n = n1 + n2;
  const u = rankSum - (n1 * (n1 + 1)) / 2;
  const variance = ((n1 * n2) / 12) * (n + 1 - tieTerm / (n * (n - 1)));

  if (variance <= 0) {
    return 1;
  }

  const z = Math.max(0, Math.abs(u - (n1 * n2) / 2) - 0.5) / Math.sqrt(variance);

  return Math.min(1, 2 * (1 - standardNormalCdf(z)));
}

/** Compares one metric, without the confirmation pass. */
export function compareMetric(spec: MetricSpec, samples: Samples, thresholds: Thresholds): MetricResult {
  const base = median(samples.base);
  const head = median(samples.head);
  const deltaPct = base === 0 ? 0 : ((head - base) / base) * 100;
  const p = mannWhitneyP(samples.base, samples.head);

  let verdict: Verdict = "unchanged";
  if (Math.abs(deltaPct) >= thresholds[spec.kind] && p < SIGNIFICANCE) {
    verdict = deltaPct > 0 ? "regression" : "improvement";
  }

  return { ...spec, base, head, deltaPct, p, verdict };
}

/**
 * Builds the report. A hyperfine metric flagged in `results` keeps its verdict
 * only when `confirm` has samples for it that reach the same verdict; when
 * `confirm` is undefined nothing is downgraded.
 */
export function buildReport({
  comparison,
  baseSha,
  headSha,
  thresholds,
  results,
  confirm,
  gobench,
}: {
  comparison: string;
  baseSha: string;
  headSha: string;
  thresholds: Thresholds;
  results: Map<string, ScenarioSamples>;
  confirm?: Map<string, ScenarioSamples>;
  gobench: Map<string, ScenarioSamples>;
}): Report {
  const ids = [...new Set([...results.keys(), ...gobench.keys()])].sort(compareScenarioIds);

  const scenarios = ids.map((id) => {
    const metrics: MetricResult[] = [];

    for (const spec of HYPERFINE_METRICS) {
      const samples = results.get(id)?.get(spec.name);
      if (!samples) {
        continue;
      }

      const result = compareMetric(spec, samples, thresholds);

      if (result.verdict !== "unchanged" && confirm) {
        const again = confirm.get(id)?.get(spec.name);
        if (!again || compareMetric(spec, again, thresholds).verdict !== result.verdict) {
          result.verdict = "unconfirmed";
        }
      }

      metrics.push(result);
    }

    for (const [name, samples] of gobench.get(id) ?? []) {
      const spec = goBenchSpec(name);
      if (spec) {
        metrics.push(compareMetric(spec, samples, thresholds));
      }
    }

    return { id, metrics };
  });

  const verdicts = scenarios.flatMap((scenario) => scenario.metrics.map((metric) => metric.verdict));

  return {
    comparison,
    baseSha,
    headSha,
    thresholds,
    regressions: verdicts.filter((verdict) => verdict === "regression").length,
    improvements: verdicts.filter((verdict) => verdict === "improvement").length,
    scenarios,
  };
}

/** Scenarios with a flagged hyperfine metric, which are the ones a confirmation pass reruns. */
export function scenariosToConfirm(report: Report): string[] {
  return report.scenarios
    .filter((scenario) =>
      scenario.metrics.some(
        (metric) =>
          metric.source === "hyperfine" && (metric.verdict === "regression" || metric.verdict === "improvement"),
      ),
    )
    .map((scenario) => scenario.id);
}

/** Scenarios with a regressed or improved metric from either source. */
export function flaggedScenarios(report: Report): string[] {
  return report.scenarios
    .filter((scenario) =>
      scenario.metrics.some((metric) => metric.verdict === "regression" || metric.verdict === "improvement"),
    )
    .map((scenario) => scenario.id);
}

/**
 * Parses `go test -bench` output into samples keyed by scenario id. A
 * BenchmarkScenario result such as `BenchmarkScenario/find/shape=flat/units=10-8`
 * becomes `find.flat.10`, and any other benchmark is keyed by its name without
 * the `Benchmark` prefix or the GOMAXPROCS suffix. Lines that are not results
 * are skipped.
 */
export function parseGoBench(output: string, side: Side, into: Map<string, ScenarioSamples>): void {
  for (const line of output.split("\n")) {
    const fields = line.trim().split(/\s+/);
    const id = goBenchId(fields[0] ?? "");
    if (!id || !/^\d+$/.test(fields[1] ?? "")) {
      continue;
    }

    // After the name and the iteration count, fields alternate value and unit.
    for (let i = 2; i + 1 < fields.length; i += 2) {
      const unit = fields[i + 1];
      const value = Number(fields[i]);
      if (!Number.isFinite(value)) {
        continue;
      }

      if (unit === "ns/op") {
        samplesFor(into, id, "sec/op")[side].push(value * 1e-9);
        continue;
      }

      if (goBenchSpec(unit)) {
        samplesFor(into, id, unit)[side].push(value);
      }
    }
  }
}

/**
 * Returns the scenario id of a benchmark result line's name, or undefined for a
 * line that is not a result. The trailing GOMAXPROCS is dropped.
 */
function goBenchId(name: string): string | undefined {
  const scenario = /^BenchmarkScenario\/([^/]+)\/shape=([^/]+)\/units=(\d+)(?:-\d+)?$/.exec(name);
  if (scenario) {
    return `${scenario[1]}.${scenario[2]}.${scenario[3]}`;
  }

  return /^Benchmark(.+?)(?:-\d+)?$/.exec(name)?.[1];
}

/**
 * Returns how to compare a Go benchmark metric, or undefined for one this
 * script does not compare. Besides the standard metrics it knows the
 * `<phase>_s/op` durations that the helpers in test/benchmarks report.
 */
function goBenchSpec(name: string): MetricSpec | undefined {
  const standard = GOBENCH_METRICS.find((spec) => spec.name === name);
  if (standard) {
    return standard;
  }

  if (name.endsWith("_s/op")) {
    return { name, kind: "time", unit: "seconds", source: "gobench" };
  }

  return undefined;
}

/** Adds the measurements of one hyperfine JSON export to `into`. */
export function addHyperfineExport(json: unknown, id: string, side: Side, into: Map<string, ScenarioSamples>): void {
  const results = (json as { results?: Array<{ measurements?: Array<Record<string, { value?: unknown }>> }> }).results;

  for (const result of results ?? []) {
    for (const measurement of result.measurements ?? []) {
      for (const spec of HYPERFINE_METRICS) {
        const value = measurement[spec.name]?.value;
        if (typeof value === "number" && Number.isFinite(value)) {
          samplesFor(into, id, spec.name)[side].push(value);
        }
      }
    }
  }
}

export function renderSummary(report: Report, extras: { benchstat?: string; profileDiffs?: Map<string, string> }): string {
  const lines: string[] = [];
  const flagged = report.scenarios.flatMap((scenario) =>
    scenario.metrics
      .filter((metric) => metric.verdict === "regression" || metric.verdict === "improvement")
      .map((metric) => ({ id: scenario.id, metric })),
  );

  lines.push(
    `## Benchmark: ${report.comparison}`,
    "",
    `Base \`${report.baseSha.slice(0, 12)}\`, head \`${report.headSha.slice(0, 12)}\`. ` +
      `${report.regressions} regressed and ${report.improvements} improved metrics across ` +
      `${report.scenarios.length} scenarios.`,
    "",
    `A metric is flagged when its median moves by at least ${report.thresholds.time}% (time), ` +
      `${report.thresholds.memory}% (peak RSS), or ${report.thresholds.alloc}% (allocations), ` +
      `with p < ${SIGNIFICANCE}. Timings from hyperfine must also repeat in a second pass.`,
    "",
  );

  if (flagged.length > 0) {
    lines.push("| Scenario | Metric | Base | Head | Change | p |", "|---|---|---:|---:|---:|---:|");

    for (const { id, metric } of flagged) {
      lines.push(
        `| \`${id}\` | ${metric.source} ${metric.name} | ${formatValue(metric.base, metric.unit)} | ` +
          `${formatValue(metric.head, metric.unit)} | ${formatDelta(metric.deltaPct)} | ${metric.p.toFixed(4)} |`,
      );
    }

    lines.push("");
  }

  for (const id of new Set(flagged.map((entry) => entry.id))) {
    const diff = extras.profileDiffs?.get(id);
    if (diff) {
      lines.push("<details>", `<summary>CPU profile diff for <code>${id}</code></summary>`, "", "```", diff, "```", "</details>", "");
    }
  }

  lines.push(
    "<details>",
    "<summary>All scenarios</summary>",
    "",
    "| Scenario | Wall time | CPU time | Peak RSS | sec/op | B/op | allocs/op |",
    "|---|---:|---:|---:|---:|---:|---:|",
  );

  const columns = [...HYPERFINE_METRICS, ...GOBENCH_METRICS];

  for (const scenario of report.scenarios) {
    const cells = columns.map((spec) => {
      const metric = scenario.metrics.find((m) => m.name === spec.name && m.source === spec.source);
      if (!metric) {
        return "";
      }

      const mark = metric.verdict === "unchanged" ? "" : ` (${metric.verdict})`;

      return `${formatValue(metric.head, metric.unit)} ${formatDelta(metric.deltaPct)}${mark}`;
    });

    lines.push(`| \`${scenario.id}\` | ${cells.join(" | ")} |`);
  }

  lines.push("", "</details>", "");

  if (extras.benchstat) {
    lines.push("<details>", "<summary>benchstat</summary>", "", "```", extras.benchstat.trimEnd(), "```", "</details>", "");
  }

  return lines.join("\n");
}

function samplesFor(into: Map<string, ScenarioSamples>, id: string, metric: string): Samples {
  let scenario = into.get(id);
  if (!scenario) {
    scenario = new Map();
    into.set(id, scenario);
  }

  let samples = scenario.get(metric);
  if (!samples) {
    samples = { base: [], head: [] };
    scenario.set(metric, samples);
  }

  return samples;
}

/**
 * Orders `<command>.<shape>.<units>` ids by command, then shape, then unit
 * count as a number, and any other id by name after them.
 */
function compareScenarioIds(a: string, b: string): number {
  const partsA = /^([^.]+)\.([^.]+)\.(\d+)$/.exec(a);
  const partsB = /^([^.]+)\.([^.]+)\.(\d+)$/.exec(b);

  if (!partsA || !partsB) {
    return Number(!partsA) - Number(!partsB) || a.localeCompare(b);
  }

  return (
    partsA[1].localeCompare(partsB[1]) || partsA[2].localeCompare(partsB[2]) || Number(partsA[3]) - Number(partsB[3])
  );
}

/** Abramowitz and Stegun formula 7.1.26, accurate to 1.5e-7. */
function standardNormalCdf(z: number): number {
  const x = Math.abs(z) / Math.SQRT2;
  const t = 1 / (1 + 0.3275911 * x);
  const poly = t * (0.254829592 + t * (-0.284496736 + t * (1.421413741 + t * (-1.453152027 + t * 1.061405429))));
  const erf = 1 - poly * Math.exp(-x * x);

  return z >= 0 ? (1 + erf) / 2 : (1 - erf) / 2;
}

function formatValue(value: number, unit: MetricSpec["unit"]): string {
  if (unit === "seconds") {
    return value >= 1 ? `${value.toFixed(2)} s` : `${(value * 1000).toFixed(1)} ms`;
  }

  if (unit === "bytes") {
    return `${(value / (1024 * 1024)).toFixed(1)} MiB`;
  }

  return Math.round(value).toLocaleString("en-US");
}

function formatDelta(deltaPct: number): string {
  return `${deltaPct >= 0 ? "+" : ""}${deltaPct.toFixed(1)}%`;
}

function readHyperfineDir(dir: string | undefined): Map<string, ScenarioSamples> {
  const into = new Map<string, ScenarioSamples>();

  // A suite that runs only Go benchmarks has no hyperfine results.
  if (!dir || !existsSync(dir)) {
    return into;
  }

  for (const id of readdirSync(dir)) {
    for (const file of readdirSync(join(dir, id))) {
      const side = /^(base|head)-\d+\.json$/.exec(file)?.[1] as Side | undefined;
      if (side) {
        addHyperfineExport(JSON.parse(readFileSync(join(dir, id, file), "utf8")), id, side, into);
      }
    }
  }

  return into;
}

function readGoBenchDir(dir: string | undefined): Map<string, ScenarioSamples> {
  const into = new Map<string, ScenarioSamples>();

  if (!dir) {
    return into;
  }

  for (const side of ["base", "head"] as const) {
    const file = join(dir, `${side}.txt`);
    if (existsSync(file)) {
      parseGoBench(readFileSync(file, "utf8"), side, into);
    }
  }

  // A scenario measured on one side only has nothing to compare against.
  for (const [id, scenario] of into) {
    for (const [metric, samples] of scenario) {
      if (samples.base.length === 0 || samples.head.length === 0) {
        scenario.delete(metric);
      }
    }

    if (scenario.size === 0) {
      into.delete(id);
    }
  }

  return into;
}

function readProfileDiffs(dir: string, lineLimit: number): Map<string, string> {
  const diffs = new Map<string, string>();

  for (const id of readdirSync(dir)) {
    const file = join(dir, id, "cpu-diff.txt");
    if (existsSync(file)) {
      diffs.set(id, readFileSync(file, "utf8").split("\n").slice(0, lineLimit).join("\n").trimEnd());
    }
  }

  return diffs;
}

function thresholdFromEnv(name: string, fallback: number): number {
  const raw = process.env[name];
  if (raw === undefined || raw === "") {
    return fallback;
  }

  const value = Number(raw);
  if (!Number.isFinite(value) || value < 0) {
    throw new Error(`${name} must be a non-negative number, got "${raw}"`);
  }

  return value;
}

function main(): void {
  const { values } = parseArgs({
    options: {
      results: { type: "string" },
      confirm: { type: "string" },
      gobench: { type: "string" },
      benchstat: { type: "string" },
      comment: { type: "string" },
      profiles: { type: "string" },
      out: { type: "string" },
      summary: { type: "string" },
      flagged: { type: "string" },
    },
  });

  if (!values.out || !values.summary) {
    throw new Error("--out and --summary are required");
  }

  const report = buildReport({
    comparison: process.env.BENCH_COMPARISON || "adhoc",
    baseSha: process.env.BENCH_BASE_SHA || "unknown",
    headSha: process.env.BENCH_HEAD_SHA || "unknown",
    thresholds: {
      time: thresholdFromEnv("BENCH_TIME_THRESHOLD_PCT", 5),
      memory: thresholdFromEnv("BENCH_MEMORY_THRESHOLD_PCT", 10),
      alloc: thresholdFromEnv("BENCH_ALLOC_THRESHOLD_PCT", 2),
    },
    results: readHyperfineDir(values.results),
    confirm: values.confirm && existsSync(values.confirm) ? readHyperfineDir(values.confirm) : undefined,
    gobench: readGoBenchDir(values.gobench),
  });

  const benchstat =
    values.benchstat && existsSync(values.benchstat) ? readFileSync(values.benchstat, "utf8") : undefined;

  const profileDiffs = values.profiles && existsSync(values.profiles) ? readProfileDiffs(values.profiles, 20) : undefined;

  writeFileSync(values.out, `${JSON.stringify(report, null, 2)}\n`);
  writeFileSync(values.summary, renderSummary(report, { benchstat, profileDiffs }));

  if (values.comment) {
    const shown = new Set(flaggedScenarios(report).slice(0, COMMENT_PROFILE_DIFFS));
    const commentDiffs = new Map([...(profileDiffs ?? [])].filter(([id]) => shown.has(id)));

    writeFileSync(values.comment, renderSummary(report, { profileDiffs: commentDiffs }));
  }

  if (values.flagged) {
    writeFileSync(values.flagged, scenariosToConfirm(report).map((id) => `${id}\n`).join(""));
  }

  console.log(
    `${report.comparison}: ${report.regressions} regressed, ${report.improvements} improved metrics ` +
      `across ${report.scenarios.length} scenarios`,
  );
}

if (import.meta.main) {
  main();
}
