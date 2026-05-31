package cloudsourcerepo

import (
	"context"
	"fmt"
	"strings"

	"github.com/markxp/govanityurls/registry"
	"google.golang.org/api/option"
	sourcerepo "google.golang.org/api/sourcerepo/v1"
)

type baseRepoChecker = registry.RepoChecker

var _ baseRepoChecker = (*RepoChecker)(nil)

// RepoChecker implements registry.RepoChecker for Google Cloud Source Repositories.
type RepoChecker struct {
	svc       *sourcerepo.Service
	projectID string
}

// NewService creates a new RepoChecker with a Google Cloud Source Repositories client.
func NewService(ctx context.Context, projectID string, opts ...option.ClientOption) (*RepoChecker, error) {
	svc, err := sourcerepo.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &RepoChecker{svc: svc, projectID: projectID}, nil
}

// CheckRepo checks if a repository exists in the given project.
// It returns the repository URL if found, or an empty string if not found.
// The path is expected to be the repository name (e.g., "my-repo").
func (s *RepoChecker) CheckRepo(_ context.Context, repoName string) (string, error) {
	// TODO: introduce otel tracing.
	// Ensure repoName doesn't have leading slash
	repoName = strings.TrimPrefix(repoName, "/")

	// Construct the full resource resourceName: projects/{projectID}/repos/{repoName}
	resourceName := fmt.Sprintf("projects/%s/repos/%s", s.projectID, repoName)

	repo, err := s.svc.Projects.Repos.Get(resourceName).Do()
	if err != nil {
		// If 404, return empty string and no error (not found)
		if strings.Contains(err.Error(), "404") {
			return "", nil
		}
		return "", err
	}

	return repo.Url, nil
}

// Registry implements registry.Registry for Google Cloud Source Repositories.
type Registry struct {
	Branch string
}

var _ registry.Registry = &Registry{}

const codeHostPrefix = "source.developers.google.com"
const uiHostPrefix = "source.cloud.google.com"

// ServerName returns the code address of the repository.
func (r *Registry) ServerName() string {
	return codeHostPrefix
}

// VCS returns the version control system used by the registry.
func (r *Registry) VCS() string {
	return "git"
}

// Display returns the content for `go-source` meta tag from the second to the fourth field.
// It returns 3 fields: `ui-home`, `ui-directory`, `ui-file`.
func (r *Registry) Display(importPath, repo string) string {
	b := r.Branch
	if b == "" {
		b = "main"
	}

	uiURL := strings.Replace(repo, codeHostPrefix, uiHostPrefix, 1)
	uiURL = strings.Replace(uiURL, "/p/", "/", 1)
	uiURL = strings.Replace(uiURL, "/r/", "/", 1)
	return fmt.Sprintf("%s %s/+/%s:{dir} %s/+/%s:{dir}/{file}#L{line}", uiURL, uiURL, b, uiURL, b)
}

// Subdir returns the subdirectory of the import path, extracting it from the importPath based on the repo URL.
//
// The `go-import` meta tag supports `subdir` from Go 1.25.
// Use `go help importpath` in Go 1.25+ to see the details.
func (r *Registry) Subdir(importPath, repo string) string {
	return registry.ExtractSubdir(importPath, repo)
}
