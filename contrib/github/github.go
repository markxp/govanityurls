package github

import (
	"fmt"
	"strings"

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
	// Extract reponame from repo url (e.g. https://github.com/user/reponame)
	parts := strings.Split(repo, "/")
	if len(parts) < 1 {
		return ""
	}
	repoName := parts[len(parts)-1]

	// Find the part after reponame in importPath
	// importPath: path/to/reponame/subdir
	idx := strings.Index(importPath, repoName)
	if idx == -1 {
		return ""
	}

	subdir := importPath[idx+len(repoName):]
	return strings.TrimPrefix(subdir, "/")
}
