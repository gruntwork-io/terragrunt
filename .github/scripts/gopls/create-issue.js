const fs = require('fs');

/**
 * Creates a GitHub issue for gopls quickfix problems found
 * @param {Object} params - Parameters object
 * @param {Object} params.github - GitHub API client
 * @param {Object} params.context - GitHub Actions context
 * @param {Object} params.core - GitHub Actions core utilities
 * @returns {Promise<number>} The created issue number
 */
module.exports = async ({ github, context, core }) => {
  try {
    const { FIXED_FILES_PATH } = process.env;
    // Read the files that were fixed from provided paths
    const fixedFiles = fs.readFileSync(FIXED_FILES_PATH, 'utf8');

    const issueBody = `## Gopls Quickfix Issues Found

The monthly gopls quickfix check found issues in the following files:

\`\`\`
${fixedFiles}
\`\`\`

### Details
- **Run Date**: ${new Date().toISOString()}
- **Workflow**: [${context.workflow}](https://github.com/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId})
- **Commit**: ${context.sha}

### Next Steps
A pull request will be created to address these issues automatically.
`;

    const issue = await github.rest.issues.create({
      owner: context.repo.owner,
      repo: context.repo.repo,
      title: `🔧 Gopls Quickfix Issues Found - ${new Date().toISOString().split('T')[0]}`,
      body: issueBody,
      labels: ['gopls', 'automated', 'maintenance']
    });

    const issueNumber = issue.data.number;
    core.setOutput('issue_number', issueNumber);
    console.log(`Created issue #${issueNumber}`);

    return issueNumber;
  } catch (error) {
    core.setFailed(`Failed to create issue: ${error.message}`);
    throw error;
  }
};
