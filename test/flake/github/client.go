// Package github provides GitHub API integration for fetching workflow data.
package github

import (
	"context"

	"github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
)

// Client wraps the GitHub API client for workflow operations.
type Client struct {
	client *github.Client
	owner  string
	repo   string
}

// NewClient creates a new GitHub API client.
func NewClient(token, owner, repo string) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		client: github.NewClient(tc),
		owner:  owner,
		repo:   repo,
	}
}

// Owner returns the repository owner.
func (c *Client) Owner() string {
	return c.owner
}

// Repo returns the repository name.
func (c *Client) Repo() string {
	return c.repo
}
