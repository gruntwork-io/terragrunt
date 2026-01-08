package github

import (
	"context"
	"time"

	"github.com/google/go-github/v53/github"
	"github.com/gruntwork-io/terragrunt/test/flake/types"
)

// ListFailedWorkflowRuns fetches failed workflow runs for a given workflow file.
func (c *Client) ListFailedWorkflowRuns(ctx context.Context, workflow, branch string, limit int, since *time.Time) ([]types.WorkflowRun, error) {
	opts := &github.ListWorkflowRunsOptions{
		Branch: branch,
		Status: "failure",
		ListOptions: github.ListOptions{
			PerPage: limit,
		},
	}

	runs, _, err := c.client.Actions.ListWorkflowRunsByFileName(ctx, c.owner, c.repo, workflow, opts)
	if err != nil {
		return nil, err
	}

	var result []types.WorkflowRun
	for _, run := range runs.WorkflowRuns {
		// Filter by date if specified
		if since != nil && run.CreatedAt.Time.Before(*since) {
			continue
		}

		result = append(result, types.WorkflowRun{
			ID:         run.GetID(),
			RunNumber:  run.GetRunNumber(),
			HeadSHA:    run.GetHeadSHA(),
			HeadBranch: run.GetHeadBranch(),
			Status:     run.GetStatus(),
			Conclusion: run.GetConclusion(),
			CreatedAt:  run.GetCreatedAt().Time,
			UpdatedAt:  run.GetUpdatedAt().Time,
			HTMLURL:    run.GetHTMLURL(),
		})

		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

// ListJobsForRun gets all jobs for a workflow run.
func (c *Client) ListJobsForRun(ctx context.Context, runID int64) ([]types.Job, error) {
	jobs, _, err := c.client.Actions.ListWorkflowJobs(ctx, c.owner, c.repo, runID, &github.ListWorkflowJobsOptions{
		Filter: "all",
	})
	if err != nil {
		return nil, err
	}

	var result []types.Job
	for _, job := range jobs.Jobs {
		result = append(result, types.Job{
			ID:         job.GetID(),
			RunID:      runID,
			Name:       job.GetName(),
			Status:     job.GetStatus(),
			Conclusion: job.GetConclusion(),
		})
	}

	return result, nil
}

// GetFailedJobs returns only the failed jobs from a workflow run.
// It combines jobs from the direct run and from check runs (for nested workflows).
func (c *Client) GetFailedJobs(ctx context.Context, runID int64) ([]types.Job, error) {
	// First get jobs directly from this run
	jobs, err := c.ListJobsForRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	var failed []types.Job
	seenNames := make(map[string]bool)

	for _, job := range jobs {
		if job.Conclusion == "failure" {
			// Skip jobs that are likely check runs without downloadable logs
			if isCheckRunWithoutLogs(job.Name) {
				continue
			}
			failed = append(failed, job)
			seenNames[job.Name] = true
		}
	}

	// Also get failed check runs for the commit (includes nested workflow jobs)
	checkRunJobs, err := c.GetFailedCheckRunJobs(ctx, runID)
	if err != nil {
		// Log but don't fail - check runs are supplementary
		return failed, nil
	}

	// Add check run jobs that aren't already in the list
	for _, job := range checkRunJobs {
		if !seenNames[job.Name] {
			failed = append(failed, job)
			seenNames[job.Name] = true
		}
	}

	return failed, nil
}

// GetFailedCheckRunJobs gets failed jobs from check runs on the commit.
// This captures jobs from nested/called workflows.
func (c *Client) GetFailedCheckRunJobs(ctx context.Context, runID int64) ([]types.Job, error) {
	// Get the workflow run to find the commit SHA
	run, _, err := c.client.Actions.GetWorkflowRunByID(ctx, c.owner, c.repo, runID)
	if err != nil {
		return nil, err
	}

	sha := run.GetHeadSHA()

	// List check runs for this commit
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(ctx, c.owner, c.repo, sha, &github.ListCheckRunsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return nil, err
	}

	var failed []types.Job
	for _, cr := range checkRuns.CheckRuns {
		if cr.GetConclusion() == "failure" {
			name := cr.GetName()
			// Skip check runs that don't have downloadable logs
			if isCheckRunWithoutLogs(name) {
				continue
			}

			// Try to find the actual job ID for this check run
			// Check runs from workflow jobs have an external_id that matches job ID
			jobID := cr.GetID() // Use check run ID as fallback

			// Look up the actual workflow job to get its ID
			if actualJob, err := c.findJobByName(ctx, sha, name); err == nil && actualJob != nil {
				jobID = actualJob.ID
			}

			failed = append(failed, types.Job{
				ID:         jobID,
				RunID:      runID,
				Name:       name,
				Status:     cr.GetStatus(),
				Conclusion: cr.GetConclusion(),
			})
		}
	}

	return failed, nil
}

// findJobByName searches all workflow runs for a commit to find a job by name.
func (c *Client) findJobByName(ctx context.Context, sha, jobName string) (*types.Job, error) {
	// List workflow runs for this commit
	runs, _, err := c.client.Actions.ListRepositoryWorkflowRuns(ctx, c.owner, c.repo, &github.ListWorkflowRunsOptions{
		HeadSHA: sha,
		ListOptions: github.ListOptions{
			PerPage: 20,
		},
	})
	if err != nil {
		return nil, err
	}

	// Search each run for the job
	for _, run := range runs.WorkflowRuns {
		jobs, _, err := c.client.Actions.ListWorkflowJobs(ctx, c.owner, c.repo, run.GetID(), nil)
		if err != nil {
			continue
		}

		for _, job := range jobs.Jobs {
			if job.GetName() == jobName {
				return &types.Job{
					ID:         job.GetID(),
					RunID:      run.GetID(),
					Name:       job.GetName(),
					Status:     job.GetStatus(),
					Conclusion: job.GetConclusion(),
				}, nil
			}
		}
	}

	return nil, nil
}

// isCheckRunWithoutLogs returns true if the job name suggests it's a check run
// that doesn't have downloadable logs (like JUnit report actions or external services).
func isCheckRunWithoutLogs(name string) bool {
	skipPatterns := []string{
		"JUnit Test Report",
		"Test Report",
		"Check Run",
		"SonarCloud Code Analysis",
		"SonarCloud",
		"Codecov",
	}
	for _, pattern := range skipPatterns {
		if name == pattern {
			return true
		}
	}
	return false
}
