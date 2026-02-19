package bitbucket

import (
	"fmt"
	"strings"

	"github.com/markxp/govanityurls/registry"
)

var _ registry.Registry = (*Registry)(nil)

type Registry struct {
	// Branch is the default branch to use for links (default: main)
	Branch string
	// VCSType is the version control system (default: git)
	VCSType string
}

// ServerName returns the host name of the registry.
func (r *Registry) ServerName() string {
	return "bitbucket.org"
}

// VCS returns the version control system used by the registry.
func (r *Registry) VCS() string {
	if r.VCSType == "" {
		return "git"
	}
	return r.VCSType
}

// Display returns the content for `go-source` meta tag from the second to the fourth field.
// It returns 3 fields: `ui-home`, `ui-directory`, `ui-file`.
func (r *Registry) Display(importPath, repo string) string {
	b := r.Branch
	if b == "" {
		b = "main"
	}

	// Bitbucket repo URL is also the UI home URL.
	// For example: https://bitbucket.org/user/repo
	return fmt.Sprintf("%s %s/src/%s{/dir} %s/src/%s{/dir}/{file}#lines-{line}", repo, repo, b, repo, b)
}

// Subdir returns the subdirectory of the import path.
func (r *Registry) Subdir(importPath, repo string) string {
	// Extract reponame from repo url (e.g. https://bitbucket.org/user/reponame)
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
