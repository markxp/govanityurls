package registry

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/markxp/govanityurls/storage"
)

// RepoChecker is the interface that wraps the CheckRepo method.
type RepoChecker interface {
	CheckRepo(ctx context.Context, name string) (string, error)
}

// Registry helps fix the repo config and generate the `go-import` and `go-source` meta tags.
type Registry interface {
	// ServerName returns where the source code is stored, e.g. "github.com"
	ServerName() string

	// VCS returns the version control system used by the registry. e.g. "git"
	// The available values are "git", "hg", "svn", "bzr", "fossil" or "mod" (for Go module proxy)
	VCS() string

	// Display returns the content for `go-source` meta tag from the second to the fourth field.
	// It returns the content of 3 fields: `ui-home`, `ui-directory`, `ui-file`.
	// There are placeholders for `ui-directory` and `ui-file` for directory, file, and line level search.
	//
	// {/dir} replace the directory with "dir". if the "dir" is not empty, add a trailing slash at the beginning. eg. "/tools", ""
	// {dir} replace the directory with "dir". eg. "tools", ""
	// {file} replace the file with "file". eg. "doc.go"
	// {line} replace the line with "line". eg. 42
	//
	// For real world example, Github uses  ".../tree/main{/dir}" for `ui-directory`.
	// If we asks for root directory, it returns ".../tree/main".
	// If we asks for tools directory, it returns ".../tree/main/tools".
	//
	// The purpose of `go-source` is to provide code browsing UI links to pkg.go.dev.
	Display(importPath, repo string) string

	// Subdir returns the subdirectory of the import path.
	// extract the subdir from importPath based on the repo url.
	//
	// The `go-import` meta tag support `subdir` from Go 1.25.
	// Use `go help importpath` in Go 1.25+ to see the details.
	Subdir(importPath, repo string) string
}

type RepoConfigFixer map[string]Registry

func (r RepoConfigFixer) Add(registry Registry) {
	if r == nil {
		r = make(RepoConfigFixer)
	}
	r[registry.ServerName()] = registry
}

func (r RepoConfigFixer) Fix(importPath string, config *storage.RepoConfig) (*storage.RepoConfig, error) {
	if config == nil {
		return nil, errors.New("repo config is required")
	}
	if config.Repo == "" {
		return config, errors.New("repo is required")
	}
	u, err := url.Parse(config.Repo)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return config, fmt.Errorf("invalid repo URL: %w", err)
	}

	// Use the registry to fix the repo config, if any field is missing.
	registry, ok := r[u.Host]
	if ok {
		if config.VCS == "" {
			config.VCS = registry.VCS()
		}
		if config.Display == "" {
			config.Display = registry.Display(importPath, config.Repo)
		}
		if config.Subdir == "" {
			config.Subdir = registry.Subdir(importPath, config.Repo)
		}
	}

	// Now call validation logic
	repoRe := regexp.MustCompile(`^https://.*$`)
	if !repoRe.MatchString(config.Repo) {
		return config, errors.New(`"repo" is where the source code is stored, e.g. "https://github.com/user/code"`)
	}

	vcsRe := regexp.MustCompile(`^(git|hg|svn|bzr|fossil|mod)$`)
	if config.VCS == "" {
		return config, errors.New("vcs is required")
	}
	if !vcsRe.MatchString(config.VCS) {
		return config, errors.New(`unknown version control system (vcs), must be one of "git", "hg", "svn", "bzr", "fossil", or "mod"(Go 1.25+ module)`)
	}

	if config.Display == "" {
		return config, errors.New("display is required")
	}

	// Validate Display format: should have at least 3 fields (`home`, `directory`, `file` templates)
	displayFields := strings.Fields(config.Display)
	if len(displayFields) < 3 {
		return config, errors.New(`"display" should have at least 3 fields: "home", "directory", "file" templates`)
	}

	return config, nil
}

type mockRegistry struct{}

func (r *mockRegistry) ServerName() string {
	return "example.com"
}

func (r *mockRegistry) VCS() string {
	return "git"
}

func (r *mockRegistry) Display(importPath, repo string) string {
	return "https://example.com/user/repo https://example.com/user/repo{/dir} https://example.com/user/repo{/dir}/{file}#{line}"
}

func (r *mockRegistry) Subdir(importPath, repo string) string {
	return "mock"
}
