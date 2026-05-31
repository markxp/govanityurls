package github

import (
	"fmt"

	"github.com/markxp/govanityurls/registry"
)

var _ registry.Registry = (*GithubRegistry)(nil)

// GithubRegistry implements registry.Registry for GitHub repositories.
type GithubRegistry struct {
	Branch     string
	DefaultVCS string
}

// ServerName returns the host name of the registry.
func (r *GithubRegistry) ServerName() string {
	return "github.com"
}

// VCS returns the version control system used by the registry.
func (r *GithubRegistry) VCS() string {
	if r.DefaultVCS == "" || r.DefaultVCS == "git" {
		return "git"
	}
	return "hg"
}

// Display returns the content for `go-source` meta tag from the second to the fourth field.
// It returns 3 fields: `ui-home`, `ui-directory`, `ui-file`.
func (r *GithubRegistry) Display(importPath, repo string) string {
	b := r.Branch
	if b == "" {
		b = "main"
	}

	// GitHub repo URL is also the UI home URL.
	// For example: https://github.com/user/repo
	return fmt.Sprintf("%s %s/tree/%s{/dir} %s/blob/%s{/dir}/{file}#L{line}", repo, repo, b, repo, b)
}

// Subdir returns the subdirectory of the import path.
func (r *GithubRegistry) Subdir(importPath, repo string) string {
	return registry.ExtractSubdir(importPath, repo)
}
