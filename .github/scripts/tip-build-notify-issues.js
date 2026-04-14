/**
 * Notifies referenced GitHub issues when a fix lands in the tip build.
 *
 * After a PR is merged to main and the tip build is uploaded, this script
 * finds issue-closing keywords in the PR body and comments on each
 * referenced issue with installation instructions for the tip build.
 *
 * @param {Object} params
 * @param {Object} params.github - GitHub API client (Octokit)
 * @param {Object} params.context - GitHub Actions context
 * @param {Object} params.core - GitHub Actions core utilities
 */
module.exports = async ({ github, context, core }) => {
  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const commitSha = context.sha;

  try {
    const mergedPRs = await findMergedPRs({ github, owner, repo, commitSha });
    if (mergedPRs.length === 0) {
      core.info('No merged PR found for this commit. Skipping issue notification.');
      return;
    }

    const issueNumbers = new Set();
    const prNumbers = [];

    for (const pr of mergedPRs) {
      if (!pr.body) {
        continue;
      }

      prNumbers.push(pr.number);
      const refs = parseIssueReferences(pr.body, owner, repo);

      for (const ref of refs) {
        if (ref.owner !== owner || ref.repo !== repo) {
          core.warning(`Skipping cross-repo issue reference: ${ref.owner}/${ref.repo}#${ref.number}`);
          continue;
        }
        issueNumbers.add(ref.number);
      }
    }

    if (issueNumbers.size === 0) {
      core.info('No issue-closing keywords found in PR body. Skipping.');
      return;
    }

    core.info(`Found ${issueNumbers.size} issue(s) referenced by PR(s) #${prNumbers.join(', #')}.`);

    let commented = 0;
    for (const issueNumber of issueNumbers) {
      const alreadyCommented = await hasExistingComment({ github, owner, repo, issueNumber, commitSha });
      if (alreadyCommented) {
        core.info(`Issue #${issueNumber} already has a tip build comment for ${commitSha}. Skipping.`);
        continue;
      }

      const body = buildComment({ owner, repo, commitSha, prNumbers, runId: context.runId });

      try {
        await github.rest.issues.createComment({
          owner,
          repo,
          issue_number: issueNumber,
          body,
        });
        core.info(`Commented on issue #${issueNumber}.`);
        commented++;
      } catch (err) {
        core.warning(`Failed to comment on issue #${issueNumber}: ${err.message}`);
      }
    }

    core.info(`Notified ${commented} issue(s) about tip build.`);
  } catch (error) {
    core.setFailed(`Failed to notify issues: ${error.message}`);
  }
};

/**
 * Finds merged PRs associated with the given commit SHA.
 */
async function findMergedPRs({ github, owner, repo, commitSha }) {
  const { data: pullRequests } = await github.rest.repos.listPullRequestsAssociatedWithCommit({
    owner,
    repo,
    commit_sha: commitSha,
  });

  return pullRequests.filter(pr => pr.merged_at !== null);
}

/**
 * Parses issue-closing keywords from a PR body.
 *
 * Matches patterns like:
 *   closes #123, fixes #456, resolves #789
 *   close #123, fix #456, resolve #789
 *   closed #123, fixed #456, resolved #789
 *   closes https://github.com/owner/repo/issues/123
 *
 * @returns {Array<{owner: string, repo: string, number: number}>}
 */
function parseIssueReferences(body, defaultOwner, defaultRepo) {
  const pattern = /(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:https?:\/\/github\.com\/([^/]+)\/([^/]+)\/issues\/(\d+)|#(\d+))/gi;
  const refs = [];
  const seen = new Set();

  let match;
  while ((match = pattern.exec(body)) !== null) {
    let refOwner, refRepo, refNumber;

    if (match[1]) {
      // Full URL form: https://github.com/owner/repo/issues/123
      refOwner = match[1];
      refRepo = match[2];
      refNumber = parseInt(match[3], 10);
    } else {
      // Shorthand form: #123
      refOwner = defaultOwner;
      refRepo = defaultRepo;
      refNumber = parseInt(match[4], 10);
    }

    const key = `${refOwner}/${refRepo}#${refNumber}`;
    if (!seen.has(key)) {
      seen.add(key);
      refs.push({ owner: refOwner, repo: refRepo, number: refNumber });
    }
  }

  return refs;
}

/**
 * Checks whether a tip build comment for this commit already exists on the issue.
 */
async function hasExistingComment({ github, owner, repo, issueNumber, commitSha }) {
  const { data: comments } = await github.rest.issues.listComments({
    owner,
    repo,
    issue_number: issueNumber,
    per_page: 100,
  });

  return comments.some(c => c.body && c.body.includes(`\`tip-${commitSha}\``));
}

/**
 * Builds the comment body to post on the issue.
 */
function buildComment({ owner, repo, commitSha, prNumbers, runId }) {
  const prLinks = prNumbers.map(n => `#${n}`).join(', ');
  const runUrl = `https://github.com/${owner}/${repo}/actions/runs/${runId}`;

  return [
    `A fix for this issue has been included in the latest [\`tip\` build](${runUrl}) (\`tip-${commitSha}\`), built from ${prLinks}.`,
    '',
    `You can install the tip build by following [these instructions](https://docs.terragrunt.com/getting-started/install/#install-tip-or-test-builds).`,
    '',
    'This fix will also be included in a future release.',
  ].join('\n');
}
