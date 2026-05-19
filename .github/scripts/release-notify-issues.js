/**
 * Notifies referenced GitHub issues when a fix lands in a published release.
 *
 * After a release transitions to `released`, this script walks every commit
 * between the previous non-prerelease tag and the current tag, finds the
 * merged PRs that introduced those commits, parses issue-closing keywords
 * from each PR body, and comments on each referenced issue with a link to
 * the release.
 *
 * @param {Object} params
 * @param {Object} params.github - GitHub API client (Octokit)
 * @param {Object} params.context - GitHub Actions context
 * @param {Object} params.core - GitHub Actions core utilities
 * @param {string} params.tagName - The current release tag (e.g. "v0.96.0")
 * @param {string} params.previousTagName - The previous non-prerelease tag
 */
const { parseIssueReferences } = require("./tip-build-notify-issues");

const MARKER_PREFIX = "release-notify:";

module.exports = async ({
  github,
  context,
  core,
  tagName,
  previousTagName,
}) => {
  const owner = context.repo.owner;
  const repo = context.repo.repo;

  if (!tagName) {
    core.setFailed("tagName is required.");
    return;
  }
  if (!previousTagName) {
    core.setFailed("previousTagName is required.");
    return;
  }

  try {
    const commits = await listCommitsInRange({
      github,
      owner,
      repo,
      base: previousTagName,
      head: tagName,
    });
    if (commits.length === 0) {
      core.info(
        `No commits between ${previousTagName} and ${tagName}. Skipping.`,
      );
      return;
    }

    const mergedPRs = await collectMergedPRs({ github, owner, repo, commits });
    if (mergedPRs.length === 0) {
      core.info(
        `No merged PRs found between ${previousTagName} and ${tagName}.`,
      );
      return;
    }

    const issueToPRs = mapIssuesToPRs({ mergedPRs, owner, repo, core });
    if (issueToPRs.size === 0) {
      core.info("No issue-closing keywords found in PR bodies. Skipping.");
      return;
    }

    core.info(
      `Found ${issueToPRs.size} issue(s) closed by PRs in ${tagName}.`,
    );

    let commented = 0;
    for (const [issueNumber, prNumbers] of issueToPRs) {
      const alreadyCommented = await hasExistingComment({
        github,
        owner,
        repo,
        issueNumber,
        tagName,
      });
      if (alreadyCommented) {
        core.info(
          `Issue #${issueNumber} already has a release comment for ${tagName}. Skipping.`,
        );
        continue;
      }

      const body = buildComment({ owner, repo, tagName, prNumbers });

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
        core.warning(
          `Failed to comment on issue #${issueNumber}: ${err.message}`,
        );
      }
    }

    core.info(`Notified ${commented} issue(s) about release ${tagName}.`);
  } catch (error) {
    core.setFailed(`Failed to notify issues: ${error.message}`);
  }
};

async function listCommitsInRange({ github, owner, repo, base, head }) {
  return await github.paginate(github.rest.repos.compareCommitsWithBasehead, {
    owner,
    repo,
    basehead: `${base}...${head}`,
    per_page: 100,
  });
}

async function collectMergedPRs({ github, owner, repo, commits }) {
  const seen = new Map();
  for (const commit of commits) {
    const { data: prs } =
      await github.rest.repos.listPullRequestsAssociatedWithCommit({
        owner,
        repo,
        commit_sha: commit.sha,
      });
    for (const pr of prs) {
      if (pr.merged_at !== null && !seen.has(pr.number)) {
        seen.set(pr.number, pr);
      }
    }
  }
  return [...seen.values()];
}

function mapIssuesToPRs({ mergedPRs, owner, repo, core }) {
  const map = new Map();
  for (const pr of mergedPRs) {
    if (!pr.body) {
      continue;
    }
    const refs = parseIssueReferences(pr.body, owner, repo);
    for (const ref of refs) {
      if (ref.owner !== owner || ref.repo !== repo) {
        if (core) {
          core.warning(
            `Skipping cross-repo issue reference: ${ref.owner}/${ref.repo}#${ref.number} (from PR #${pr.number})`,
          );
        }
        continue;
      }
      const list = map.get(ref.number) || [];
      if (!list.includes(pr.number)) {
        list.push(pr.number);
      }
      map.set(ref.number, list);
    }
  }
  return map;
}

async function hasExistingComment({
  github,
  owner,
  repo,
  issueNumber,
  tagName,
}) {
  const comments = await github.paginate(github.rest.issues.listComments, {
    owner,
    repo,
    issue_number: issueNumber,
    per_page: 100,
  });

  return comments.some(
    (c) => c.body && c.body.includes(`${MARKER_PREFIX} ${tagName}`),
  );
}

function buildComment({ owner, repo, tagName, prNumbers }) {
  const prLinks = prNumbers.map((n) => `#${n}`).join(", ");
  const releaseUrl = `https://github.com/${owner}/${repo}/releases/tag/${tagName}`;

  return [
    `A fix for this issue has been released in [\`${tagName}\`](${releaseUrl}) (${prLinks}).`,
    "",
    "You can install Terragrunt by following [these instructions](https://docs.terragrunt.com/getting-started/install/).",
    "",
    `<!-- ${MARKER_PREFIX} ${tagName} -->`,
  ].join("\n");
}

module.exports.parseIssueReferences = parseIssueReferences;
module.exports.mapIssuesToPRs = mapIssuesToPRs;
module.exports.buildComment = buildComment;
module.exports.hasExistingComment = hasExistingComment;
module.exports.collectMergedPRs = collectMergedPRs;
module.exports.listCommitsInRange = listCommitsInRange;
module.exports.MARKER_PREFIX = MARKER_PREFIX;
