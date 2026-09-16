# Code Review (Round 3): 6930-legacy-base64gzip / PR #6947

- Date: 2026-09-16
- Branch: 6930-legacy-base64gzip (4 commits ahead of origin/main; working tree clean)
- Base: origin/main (fetched, up to date at fe471b14a)
- PR: https://github.com/gruntwork-io/terragrunt/pull/6947 (draft, OPEN)
- Scope: full PR diff re-verified via gh and git; new commit under review: ba256f698 "chore: docs cleanup" (functions.mdx, breaking-changes changelog, strict-control page)
- Tests: not run per instructions; static review only

## New in this round

Commit ba256f698 restructures three docs pages with `<Before version="1.2.0">` / `<Since version="1.2.0">`
blocks so the pages show correct behavior for both pre- and post-1.2 readers. No Go code changed.

## Verdict

Approve. No blockers. Three carried-over nits, one new wording nit, one scope note.

## Counts by severity

| Severity | Count |
|----------|-------|
| Blocker | 0 |
| High | 0 |
| Medium | 0 |
| NIT | 4 |
| Note | 1 |

## Counts by file

| File Path | Issue Count |
|-----------|-------------|
| /projects/gruntwork/terragrunt/docs/src/data/changelog/v1.2.0/base64gzip-uses-the-current-encoder.mdx | 1 |
| /projects/gruntwork/terragrunt/docs/src/data/experiments/base64gzip-compat.mdx | 1 (carried) |
| /projects/gruntwork/terragrunt/pkg/config/config_helpers.go | 1 (carried) |
| /projects/gruntwork/terragrunt/test/integration_aws_auth_provider_role_test.go | 1 (note, carried) |

## Findings (most critical first)

### 1. NIT - changelog sentence "It now returns the new ones" has a loose antecedent

- File: /projects/gruntwork/terragrunt/docs/src/data/changelog/v1.2.0/base64gzip-uses-the-current-encoder.mdx
- Line: 8
- Issue: "Terragrunt v1.1.5 kept returning the old bytes and warned that this would change. It now
  returns the new ones." The subject switches from "Terragrunt v1.1.5" (a past release) to "It"
  (the current release); "old bytes" / "new ones" are only defined by the prior sentence's context.
  A skimming reader may misread which release does what.
- User manifestation: none; documentation only.
- Fix (wording): "Terragrunt v1.1.5 kept returning the v1.1.3 bytes and warned that this would
  change. Terragrunt 1.2 returns the current Go encoder's bytes."
- Confidence: high (verified line).

### 2. NIT - stale tense in stabilization criteria heading (carried over, rounds 1-2)

- File: /projects/gruntwork/terragrunt/docs/src/data/experiments/base64gzip-compat.mdx
- Line: 23
- Issue: "To transition the `base64gzip-compat` feature to a stable release, the following were
  addressed:" mixes future intent with past tense.
- User manifestation: none; documentation only.
- Fix (wording): "The following criteria were addressed before the experiment was completed:"
- Confidence: high.

### 3. NIT - guard closure is a permanent no-op (carried over, rounds 1-2)

- File: /projects/gruntwork/terragrunt/pkg/config/config_helpers.go
- Line: 368
- Issue: `gzipcompat.Func(func() error { return nil })` - the `onUse` guard parameter
  (/projects/gruntwork/terragrunt/internal/gzipcompat/gzipcompat.go:35-37, documented as required,
  non-nil) is now always a no-op.
- User manifestation: none.
- Fix (optional): allow nil `onUse` in `gzipcompat.Func` or add a guard-free constructor.
- Confidence: high.

### 4. NIT - strict-control page lost the "what compat returns" detail for post-1.2 readers

- File: /projects/gruntwork/terragrunt/docs/src/data/strict-controls/legacy-base64gzip.mdx
- Line: 30
- Issue: The closing line changed from "call [`base64gzip_compat()`]... It returns the v1.1.3 bytes
  and needs no flag." to just "call [`base64gzip_compat()`] instead of `base64gzip()`." The
  "needs no flag" detail was the actionable part for a reader migrating off this control. The
  linked function page carries it, so this is minor.
- User manifestation: none; documentation only.
- Fix (wording, optional): restore "It returns the v1.1.3 bytes and needs no flag."
- Confidence: high.

### 5. Note - integration test refactor is unrelated to the base64gzip change (scope, carried)

- File: /projects/gruntwork/terragrunt/test/integration_aws_auth_provider_role_test.go
- Lines: 24, 57
- Issue: `require.NotEmpty` refactor correct but unrelated to this PR topic.
- Confidence: high.

## New content verified correct (commit ba256f698)

- /projects/gruntwork/terragrunt/docs/src/content/docs/04-reference/01-hcl/04-functions.mdx -
  Before/Since blocks are accurate on both sides: the Before block correctly states the experiment
  requirement and the error for pre-1.2, and drops the stale "name may still change" line; the
  Since block correctly states no flag needed and the 1.2 default. The removed line and the new
  content match actual behavior verified in earlier rounds.
- /projects/gruntwork/terragrunt/docs/src/data/strict-controls/legacy-base64gzip.mdx - Before block
  restores the accurate pre-1.2 instructions including the `--strict-control legacy-base64gzip`
  command; Since block states the control is complete and warns on use; no contradiction with the
  new default remains.
- Changelog final line "Passing either still works and logs a warning." is accurate per
  `LogCompletedControls` (/projects/gruntwork/terragrunt/internal/strict/control.go:158-164) and
  `NotifyCompletedExperiments` (/projects/gruntwork/terragrunt/internal/experiment/experiment.go:318-330).
- Component imports verified: /projects/gruntwork/terragrunt/docs/src/components/Before.astro and
  /projects/gruntwork/terragrunt/docs/src/components/Since.astro exist.
- Missing trailing newline at EOF on functions.mdx is pre-existing, unchanged.
- ASCII check on the commit diff: clean.

## Carried-over verified-correct items (unchanged this round)

- /projects/gruntwork/terragrunt/pkg/config/config_helpers.go - gate and `base64gzip()` override
  removed; HCL-native function is the default (the documented 1.2 breaking change).
- /projects/gruntwork/terragrunt/pkg/config/errors.go - experiment error type removed, no references.
- /projects/gruntwork/terragrunt/internal/experiment/experiment.go:255-258 and experiment_test.go -
  `StatusCompleted`, test assertions correct.
- /projects/gruntwork/terragrunt/internal/strict/controls/controls.go:333-338 - Description
  rewritten for post-1.2; `Status: strict.CompletedStatus`; dead warning const deleted.
- /projects/gruntwork/terragrunt/pkg/config/config_helpers_test.go - table updated; golden vectors
  intact; `EnableControl` accepts completed controls.
- Docs claim "vendored copy of compress/flate" verified:
  /projects/gruntwork/terragrunt/internal/gzipcompat/gzipcompat.go:11 imports
  /projects/gruntwork/terragrunt/internal/vendored/compress/flate.

## CI summary (gh pr checks 6947, unchanged since round 2)

| Check | Result |
|-------|--------|
| Pull Request has non-contributor approval | fail (external repo policy; needs maintainer approval, not caused by this diff) |
| GitGuardian Security Checks | pass |
| Vercel | pending |
| CodeRabbit | pass (skipped: draft PR) |

## Junior-developer task list

1. Optional: fix the antecedent on line 8 of
   /projects/gruntwork/terragrunt/docs/src/data/changelog/v1.2.0/base64gzip-uses-the-current-encoder.mdx
   per finding 1.
2. Optional: fix the tense on line 23 of
   /projects/gruntwork/terragrunt/docs/src/data/experiments/base64gzip-compat.mdx per finding 2.
3. Optional: restore "It returns the v1.1.3 bytes and needs no flag." on line 30 of
   /projects/gruntwork/terragrunt/docs/src/data/strict-controls/legacy-base64gzip.mdx per finding 4.
4. Optional: resolve the no-op guard in /projects/gruntwork/terragrunt/pkg/config/config_helpers.go:368.
5. Keep the test refactor as-is or split it into its own commit.
6. Get a maintainer approval to satisfy the non-contributor-approval policy check; mark the PR
   ready for review.
