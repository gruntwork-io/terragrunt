// Package repourl provides utilities for handling different repository URL formats
package repourl

import (
	"fmt"
	"strings"
)

// RepoURL represents a parsed repository URL with methods to convert between different formats
type RepoURL struct {
	// Original is the unmodified input URL
	Original string

	// Protocol is "cln", "git", "https", "ssh", etc.
	Protocol string

	// Host is the hostname (e.g., "github.com")
	Host string

	// Owner is the organization or user
	Owner string

	// Repo is the repository name
	Repo string

	// Path is the optional subdirectory path
	Path string
}

// Parse takes a URL string and returns a parsed RepoURL
func Parse(url string) (*RepoURL, error) {
	result := &RepoURL{
		Original: url,
	}

	// Handle cln:// protocol
	if strings.HasPrefix(url, "cln://") {
		result.Protocol = "cln"
		url = strings.TrimPrefix(url, "cln://")
	}

	// Handle git:: protocol
	if strings.HasPrefix(url, "git::") {
		result.Protocol = "git"
		url = strings.TrimPrefix(url, "git::")
	}

	// Handle SSH URLs (git@github.com:org/repo.git)
	if strings.HasPrefix(url, "git@") {
		result.Protocol = "ssh"
		parts := strings.Split(strings.TrimPrefix(url, "git@"), ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid SSH URL format: %s", url)
		}
		result.Host = parts[0]

		repoPath := strings.TrimSuffix(parts[1], ".git")
		pathParts := strings.SplitN(repoPath, "/", 2)

		if len(pathParts) > 1 {
			result.Owner = pathParts[0]
			result.Repo = pathParts[1]
		} else {
			result.Repo = pathParts[0]
		}

		return result, nil
	}

	// Handle HTTPS URLs
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		if strings.HasPrefix(url, "https://") {
			result.Protocol = "https"
			url = strings.TrimPrefix(url, "https://")
		} else {
			result.Protocol = "http"
			url = strings.TrimPrefix(url, "http://")
		}

		// Split on // to get the module path if it exists
		urlParts := strings.Split(url, "//")
		url = urlParts[0]

		if len(urlParts) > 1 {
			result.Path = urlParts[1]
		}

		// Parse the remaining URL
		parts := strings.Split(url, "/")
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid HTTPS URL format: %s", result.Original)
		}

		result.Host = parts[0]
		result.Owner = parts[1]
		result.Repo = strings.TrimSuffix(parts[2], ".git")

		return result, nil
	}

	// Default case - assume it's a path without protocol
	parts := strings.Split(url, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid URL format: %s", url)
	}

	result.Host = parts[0]
	result.Owner = parts[1]
	result.Repo = strings.TrimSuffix(parts[2], ".git")

	if len(parts) > 3 {
		result.Path = strings.Join(parts[3:], "/")
	}

	return result, nil
}

// ToSSH returns the URL in SSH format (git@github.com:org/repo.git)
func (r *RepoURL) ToSSH() string {
	return fmt.Sprintf("git@%s:%s/%s.git", r.Host, r.Owner, r.Repo)
}

// ToHTTPS returns the URL in HTTPS format (https://github.com/org/repo)
func (r *RepoURL) ToHTTPS() string {
	return fmt.Sprintf("https://%s/%s/%s", r.Host, r.Owner, r.Repo)
}

// ToCLN returns the URL in CLN format (cln://github.com/org/repo)
func (r *RepoURL) ToCLN() string {
	base := fmt.Sprintf("cln://%s/%s/%s", r.Host, r.Owner, r.Repo)
	if r.Path != "" {
		return fmt.Sprintf("%s//%s", base, r.Path)
	}
	return base
}

// ToTerraformSource returns the URL in Terraform source format
func (r *RepoURL) ToTerraformSource() string {
	// For CLN protocol, preserve the format
	if r.Protocol == "cln" {
		return r.ToCLN()
	}

	// For SSH protocol, preserve the SSH format for private repos
	if r.Protocol == "ssh" {
		sshURL := r.ToSSH()
		if r.Path != "" {
			return fmt.Sprintf("git::%s//%s", sshURL, r.Path)
		}
		return fmt.Sprintf("git::%s", sshURL)
	}

	// For other protocols
	base := fmt.Sprintf("%s/%s/%s", r.Host, r.Owner, r.Repo)
	if r.Path != "" {
		return fmt.Sprintf("%s//%s", base, r.Path)
	}
	return base
}
