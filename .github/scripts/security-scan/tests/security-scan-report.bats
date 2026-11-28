#!/usr/bin/env bats

setup() {
  # Keep any summary a script writes inside this test's tmpdir, not in the job summary.
  export GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md"

  SCRIPT="${BATS_TEST_DIRNAME}/../security-scan-report.sh"
  CURRENT_RAW="${BATS_TEST_TMPDIR}/current-raw.json"
  PREVIOUS_RAW="${BATS_TEST_TMPDIR}/previous-raw.json"
  CURRENT="${BATS_TEST_TMPDIR}/current-summary.json"
  PREVIOUS="${BATS_TEST_TMPDIR}/previous-summary.json"
  REPORT="${BATS_TEST_TMPDIR}/report.json"
  SUPPRESSIONS="${BATS_TEST_TMPDIR}/report-suppressions.json"
  # Isolate the fixtures from the repository suppressions config; tests opt in by writing $SUPPRESSIONS.
  export SECURITY_SCAN_SUPPRESSIONS_FILE="$SUPPRESSIONS"

  # Current scan: a duplicated HIGH vuln, a severity-less vuln, a failing and a
  # passing misconfiguration, and a CRITICAL secret.
  cat >"$CURRENT_RAW" <<'EOF'
{
  "SchemaVersion": 2,
  "Results": [
    {
      "Target": "go.mod",
      "Class": "lang-pkgs",
      "Type": "gomod",
      "Vulnerabilities": [
        {"VulnerabilityID": "CVE-2026-1111", "PkgName": "golang.org/x/foo", "InstalledVersion": "1.0.0", "FixedVersion": "1.0.1", "Severity": "HIGH", "Title": "foo before 1.0.1 allows RCE"},
        {"VulnerabilityID": "CVE-2026-1111", "PkgName": "golang.org/x/foo", "InstalledVersion": "1.0.0", "FixedVersion": "1.0.1", "Severity": "HIGH", "Title": "foo before 1.0.1 allows RCE"},
        {"VulnerabilityID": "GO-2026-9999", "PkgName": "golang.org/x/bar", "InstalledVersion": "0.2.0", "Title": "bar is unmaintained"}
      ]
    },
    {
      "Target": "Dockerfile",
      "Class": "config",
      "Type": "dockerfile",
      "Misconfigurations": [
        {"ID": "DS002", "AVDID": "AVD-DS-0002", "Title": "Image user should not be root", "Severity": "MEDIUM", "Status": "FAIL", "CauseMetadata": {"StartLine": 5}},
        {"ID": "DS999", "AVDID": "AVD-DS-0999", "Title": "Passing check", "Severity": "LOW", "Status": "PASS"}
      ]
    },
    {
      "Target": "config/creds.txt",
      "Class": "secret",
      "Secrets": [
        {"RuleID": "github-pat", "Category": "GitHub", "Severity": "CRITICAL", "Title": "GitHub Personal Access Token", "StartLine": 10}
      ]
    }
  ]
}
EOF

  # Previous scan: shares GO-2026-9999 and AVD-DS-0002 with current, lacks the
  # HIGH vuln and the secret (new this week), and has one LOW vuln fixed since.
  cat >"$PREVIOUS_RAW" <<'EOF'
{
  "SchemaVersion": 2,
  "Results": [
    {
      "Target": "go.mod",
      "Class": "lang-pkgs",
      "Type": "gomod",
      "Vulnerabilities": [
        {"VulnerabilityID": "GO-2026-9999", "PkgName": "golang.org/x/bar", "InstalledVersion": "0.2.0", "Title": "bar is unmaintained"},
        {"VulnerabilityID": "CVE-2020-0001", "PkgName": "example.com/old", "InstalledVersion": "2.0.0", "FixedVersion": "2.0.1", "Severity": "LOW", "Title": "old bug"}
      ]
    },
    {
      "Target": "Dockerfile",
      "Class": "config",
      "Type": "dockerfile",
      "Misconfigurations": [
        {"ID": "DS002", "AVDID": "AVD-DS-0002", "Title": "Image user should not be root", "Severity": "MEDIUM", "Status": "FAIL"}
      ]
    }
  ]
}
EOF

  "$SCRIPT" summary "$CURRENT_RAW" "$CURRENT"
  "$SCRIPT" summary "$PREVIOUS_RAW" "$PREVIOUS"
  "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "$REPORT"
}

@test "summary counts findings by severity" {
  run jq -c '.counts' "$CURRENT"
  [ "$output" == '{"CRITICAL":1,"HIGH":1,"MEDIUM":1,"LOW":0,"UNKNOWN":1,"total":4}' ]
}

@test "summary dedupes repeated findings" {
  run jq -r '[.findings[] | select(.id == "CVE-2026-1111")] | length' "$CURRENT"
  [ "$output" == "1" ]
}

@test "summary drops passing misconfigurations" {
  run jq -r '.findings[].id' "$CURRENT"
  [[ "$output" == *"AVD-DS-0002"* ]]
  [[ "$output" != *"AVD-DS-0999"* ]]
}

@test "summary defaults a missing severity to UNKNOWN" {
  run jq -r '.findings[] | select(.id == "GO-2026-9999") | .severity' "$CURRENT"
  [ "$output" == "UNKNOWN" ]
}

@test "summary maps an unsupported severity to UNKNOWN" {
  jq '(.Results[0].Vulnerabilities[] | select(.VulnerabilityID == "CVE-2026-1111") | .Severity) = "BOGUS"' \
    "$CURRENT_RAW" >"${BATS_TEST_TMPDIR}/bogus-raw.json"
  run "$SCRIPT" summary "${BATS_TEST_TMPDIR}/bogus-raw.json" "${BATS_TEST_TMPDIR}/bogus-summary.json"
  [ "$status" -eq 0 ]
  run jq -r '.findings[] | select(.id == "CVE-2026-1111") | .severity' "${BATS_TEST_TMPDIR}/bogus-summary.json"
  [ "$output" == "UNKNOWN" ]
}

@test "summary orders findings most severe first" {
  run jq -r '[.findings[].severity] | join(",")' "$CURRENT"
  [ "$output" == "CRITICAL,HIGH,MEDIUM,UNKNOWN" ]
}

@test "summary tolerates a scan with no results" {
  echo '{"SchemaVersion": 2}' >"${BATS_TEST_TMPDIR}/empty-raw.json"
  run "$SCRIPT" summary "${BATS_TEST_TMPDIR}/empty-raw.json" "${BATS_TEST_TMPDIR}/empty-summary.json"
  [ "$status" -eq 0 ]
  run jq -r '.counts.total' "${BATS_TEST_TMPDIR}/empty-summary.json"
  [ "$output" == "0" ]
}

@test "summary fails on malformed scan JSON" {
  echo 'not json' >"${BATS_TEST_TMPDIR}/broken.json"
  run "$SCRIPT" summary "${BATS_TEST_TMPDIR}/broken.json" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"not valid JSON"* ]]
}

@test "summary fails on a missing scan file" {
  run "$SCRIPT" summary "${BATS_TEST_TMPDIR}/nonexistent.json" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "compare reports new and fixed findings" {
  run jq -r '[.new[].id] | join(",")' "$REPORT"
  [ "$output" == "github-pat,CVE-2026-1111" ]
  run jq -r '[.fixed[].id] | join(",")' "$REPORT"
  [ "$output" == "CVE-2020-0001" ]
  run jq -r '.unchanged' "$REPORT"
  [ "$output" == "2" ]
}

@test "compare reports a baseline when the previous summary is missing" {
  run "$SCRIPT" compare "$CURRENT" /nonexistent "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  run jq -c '{baseline, new: (.new | length), findings: (.current_findings | length)}' "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$output" == '{"baseline":true,"new":0,"findings":4}' ]
}

@test "compare treats an empty previous summary as a baseline" {
  : >"${BATS_TEST_TMPDIR}/empty-prev.json"
  run "$SCRIPT" compare "$CURRENT" "${BATS_TEST_TMPDIR}/empty-prev.json" "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  run jq -r '.baseline' "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$output" == "true" ]
}

@test "compare fails on a missing current summary" {
  run "$SCRIPT" compare "${BATS_TEST_TMPDIR}/nonexistent.json" "$PREVIOUS" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "render writes the report to the step summary" {
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" run "$SCRIPT" render "$REPORT"
  [ "$status" -eq 0 ]
  run cat "${BATS_TEST_TMPDIR}/summary.md"
  [[ "$output" == *"New findings (2)"* ]]
  [[ "$output" == *"Fixed findings (1)"* ]]
  [[ "$output" == *"CVE-2026-1111"* ]]
}

@test "render caps the current findings table" {
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" SECURITY_SCAN_RENDER_LIMIT=1 run "$SCRIPT" render "$REPORT"
  run cat "${BATS_TEST_TMPDIR}/summary.md"
  [[ "$output" == *"and 3 more"* ]]
}

@test "render tolerates a missing report" {
  run "$SCRIPT" render "${BATS_TEST_TMPDIR}/nonexistent.json"
  [ "$status" -eq 0 ]
  [[ "$output" == *"skipping summary"* ]]
}

@test "payload lists new and fixed findings" {
  run "$SCRIPT" payload "$REPORT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"New this week (2)"* ]]
  [[ "$output" == *"Fixed this week (1)"* ]]
  [[ "$output" == *"[CRITICAL] github-pat GitHub Personal Access Token (config/creds.txt:10)"* ]]
  [[ "$output" == *"[LOW] example.com/old 2.0.0 -> 2.0.1 CVE-2020-0001 (go.mod)"* ]]
}

@test "payload lists the current findings" {
  run "$SCRIPT" payload "$REPORT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Current findings (4)"* ]]
  [[ "$output" == *"golang.org/x/bar 0.2.0 GO-2026-9999 (go.mod)"* ]]
}

@test "payload omits the UNKNOWN severity tag" {
  run "$SCRIPT" payload "$REPORT"
  [[ "$output" != *"[UNKNOWN]"* ]]
}

@test "payload caps the listed findings" {
  SECURITY_SCAN_NOTIFY_LIMIT=1 run "$SCRIPT" payload "$REPORT"
  [[ "$output" == *"...and 1 more"* ]]
}

@test "payload reports when nothing is new" {
  run "$SCRIPT" compare "$CURRENT" "$CURRENT" "${BATS_TEST_TMPDIR}/same.json"
  [ "$status" -eq 0 ]
  run "$SCRIPT" payload "${BATS_TEST_TMPDIR}/same.json"
  [[ "$output" == *"No new findings this week."* ]]
}

@test "notify requires the webhook env" {
  SECURITY_SCAN_SLACK_WEBHOOK_URL="" run "$SCRIPT" notify "$REPORT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"SECURITY_SCAN_SLACK_WEBHOOK_URL"* ]]
}

@test "notify-failure requires the webhook env" {
  SECURITY_SCAN_SLACK_WEBHOOK_URL="" run "$SCRIPT" notify-failure
  [ "$status" -ne 0 ]
  [[ "$output" == *"SECURITY_SCAN_SLACK_WEBHOOK_URL"* ]]
}

@test "render reports a baseline" {
  run "$SCRIPT" compare "$CURRENT" /nonexistent "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" run "$SCRIPT" render "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  run cat "${BATS_TEST_TMPDIR}/summary.md"
  [[ "$output" == *"Baseline established: 4 total"* ]]
  [[ "$output" != *"New findings"* ]]
}

@test "payload reports a baseline" {
  run "$SCRIPT" compare "$CURRENT" /nonexistent "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  CURRENT_SHA=abc123 CURRENT_DATE=2026-08-17 run "$SCRIPT" payload "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Findings baseline: 4 total"* ]]
  [[ "$output" == *"At: 2026-08-17 abc123"* ]]
}

@test "scan checks its tools before doing any work" {
  SHIMS="${BATS_TEST_TMPDIR}/shims"
  mkdir -p "$SHIMS"
  for tool in bash dirname basename jq; do ln -s "$(command -v "$tool")" "$SHIMS/$tool"; done
  PATH="$SHIMS" run "$SCRIPT" scan . "${BATS_TEST_TMPDIR}/scan/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"trivy"* ]]
  [ ! -d "${BATS_TEST_TMPDIR}/scan" ]
}

@test "fails on an unknown subcommand" {
  run "$SCRIPT" bogus
  [ "$status" -ne 0 ]
  [[ "$output" == *"Unknown subcommand: bogus"* ]]
}

@test "compare drops suppressed findings from counts, new, and current findings" {
  cat >"$SUPPRESSIONS" <<'JSON'
{
  "findings": [
    {"id": "CVE-2026-1111", "pkg": "golang.org/x/foo", "reason": "not reachable"}
  ]
}
JSON
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$status" -eq 0 ]
  [[ "$output" == *"1 suppressed"* ]]
  run jq -c '{total: .current_counts.total, high: .current_counts.HIGH, new: [.new[].id], current: [.current_findings[].id], suppressed: [.suppressed[] | "\(.id) \(.reason)"], stale: .stale_suppressions}' "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$output" == '{"total":3,"high":0,"new":["github-pat"],"current":["github-pat","AVD-DS-0002","GO-2026-9999"],"suppressed":["CVE-2026-1111 not reachable"],"stale":[]}' ]
}

@test "compare does not report a suppressed finding as fixed" {
  # CVE-2020-0001 only exists in the previous scan.
  echo '{"findings": [{"id": "CVE-2020-0001", "reason": "accepted"}]}' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$status" -eq 0 ]
  run jq -c '{fixed: .fixed, previous_total: .previous_counts.total, stale: [.stale_suppressions[] | "\(.id) \(.state)"]}' "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$output" == '{"fixed":[],"previous_total":2,"stale":["CVE-2020-0001 unmatched"]}' ]
}

@test "compare scopes a suppression by package and target" {
  cat >"$SUPPRESSIONS" <<'JSON'
{
  "findings": [
    {"id": "CVE-2026-1111", "pkg": "example.com/other", "reason": "wrong package, must not match"},
    {"id": "AVD-DS-0002", "target": "Dockerfile", "reason": "image runs as root on purpose"}
  ]
}
JSON
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$status" -eq 0 ]
  run jq -c '{current: [.current_findings[].id], suppressed: [.suppressed[].id], stale: [.stale_suppressions[].id]}' "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$output" == '{"current":["github-pat","CVE-2026-1111","GO-2026-9999"],"suppressed":["AVD-DS-0002"],"stale":["CVE-2026-1111"]}' ]
}

@test "compare ignores an expired suppression and reports it as stale" {
  cat >"$SUPPRESSIONS" <<'JSON'
{
  "findings": [
    {"id": "CVE-2026-1111", "reason": "lapsed", "expires": "2000-01-01"},
    {"id": "GO-2026-9999", "reason": "still accepted", "expires": "2999-12-31"}
  ]
}
JSON
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$status" -eq 0 ]
  run jq -c '{current: [.current_findings[].id], suppressed: [.suppressed[].id], stale: [.stale_suppressions[] | "\(.id) \(.state)"]}' "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$output" == '{"current":["github-pat","CVE-2026-1111","AVD-DS-0002"],"suppressed":["GO-2026-9999"],"stale":["CVE-2026-1111 expired"]}' ]
}

@test "compare applies suppressions to a baseline report" {
  echo '{"findings": [{"id": "github-pat", "target": "config/creds.txt", "reason": "test fixture token"}]}' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" /nonexistent "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$status" -eq 0 ]
  run jq -c '{baseline, total: .current_counts.total, critical: .current_counts.CRITICAL, suppressed: [.suppressed[].id]}' "${BATS_TEST_TMPDIR}/baseline.json"
  [ "$output" == '{"baseline":true,"total":3,"critical":0,"suppressed":["github-pat"]}' ]
}

@test "compare uses no suppressions when the config is missing" {
  SECURITY_SCAN_SUPPRESSIONS_FILE="${BATS_TEST_TMPDIR}/missing.json" run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/none.json"
  [ "$status" -eq 0 ]
  run jq -c '{total: .current_counts.total, suppressed: .suppressed, stale: .stale_suppressions}' "${BATS_TEST_TMPDIR}/none.json"
  [ "$output" == '{"total":4,"suppressed":[],"stale":[]}' ]
}

@test "compare rejects a suppression without a reason" {
  echo '{"findings": [{"id": "CVE-2026-1111"}]}' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"findings[0]: reason is required"* ]]
  [ ! -f "${BATS_TEST_TMPDIR}/out.json" ]
}

@test "compare rejects unknown suppression fields and bad dates" {
  echo '{"findings": [{"id": "CVE-2026-1111", "reason": "x", "package": "golang.org/x/foo"}]}' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown field(s) package"* ]]

  echo '{"findings": [{"id": "CVE-2026-1111", "reason": "x", "expires": "next week"}]}' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"expires must be a YYYY-MM-DD date"* ]]

  echo '{"vulnerabilities": []}' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown field(s) vulnerabilities"* ]]
}

@test "compare rejects a malformed suppressions file" {
  echo 'not json' >"$SUPPRESSIONS"
  run "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/out.json"
  [ "$status" -ne 0 ]
  [[ "$output" == *"is invalid"* ]]
}

@test "render lists suppressed findings and stale suppressions" {
  cat >"$SUPPRESSIONS" <<'JSON'
{
  "findings": [
    {"id": "CVE-2026-1111", "pkg": "golang.org/x/foo", "reason": "not reachable"},
    {"id": "CVE-2000-0000", "reason": "long gone"}
  ]
}
JSON
  "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/suppressed.json"
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" run "$SCRIPT" render "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$status" -eq 0 ]
  run cat "${BATS_TEST_TMPDIR}/summary.md"
  [[ "$output" == *"All current findings (3)"* ]]
  [[ "$output" == *"Suppressed findings (1)"* ]]
  [[ "$output" == *"| HIGH | vuln | CVE-2026-1111 | go.mod | golang.org/x/foo | 1.0.0 | not reachable |"* ]]
  [[ "$output" == *"Stale suppressions (1)"* ]]
  [[ "$output" == *"- CVE-2000-0000 - unmatched"* ]]
}

@test "render omits the suppression sections when nothing is configured" {
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" run "$SCRIPT" render "$REPORT"
  [ "$status" -eq 0 ]
  run cat "${BATS_TEST_TMPDIR}/summary.md"
  [[ "$output" != *"Suppressed findings"* ]]
  [[ "$output" != *"Stale suppressions"* ]]
}

@test "render tolerates a report without suppression fields" {
  jq 'del(.suppressed, .stale_suppressions)' "$REPORT" >"${BATS_TEST_TMPDIR}/old-report.json"
  GITHUB_STEP_SUMMARY="${BATS_TEST_TMPDIR}/summary.md" run "$SCRIPT" render "${BATS_TEST_TMPDIR}/old-report.json"
  [ "$status" -eq 0 ]
  run "$SCRIPT" payload "${BATS_TEST_TMPDIR}/old-report.json"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Current findings (4)"* ]]
}

@test "payload lists suppressed findings and stale suppressions" {
  cat >"$SUPPRESSIONS" <<'JSON'
{
  "findings": [
    {"id": "CVE-2026-1111", "pkg": "golang.org/x/foo", "reason": "not reachable"},
    {"id": "CVE-2000-0000", "reason": "long gone", "expires": "2000-01-01"}
  ]
}
JSON
  "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/suppressed.json"
  run "$SCRIPT" payload "${BATS_TEST_TMPDIR}/suppressed.json"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Findings: 3 total (1 CRITICAL, 1 MEDIUM)"* ]]
  [[ "$output" == *"Current findings (3)"* ]]
  [[ "$output" == *"Suppressed (1):"* ]]
  [[ "$output" == *"[HIGH] golang.org/x/foo 1.0.0 -> 1.0.1 CVE-2026-1111 (go.mod)"* ]]
  [[ "$output" == *"Stale suppressions (1): CVE-2000-0000 (expired)"* ]]
}

@test "the repository suppressions config is valid" {
  [ -f "${BATS_TEST_DIRNAME}/../report-suppressions.json" ]
  run env -u SECURITY_SCAN_SUPPRESSIONS_FILE "$SCRIPT" compare "$CURRENT" "$PREVIOUS" "${BATS_TEST_TMPDIR}/repo.json"
  [ "$status" -eq 0 ]
}
