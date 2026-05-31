package storage

import (
	"context"
)

// RepoConfig holds the configuration for a single repository.
type RepoConfig struct {
	Repo    string `firestore:"repo" yaml:"repo" json:"repo"`
	Display string `firestore:"display" yaml:"display" json:"display"`
	VCS     string `firestore:"vcs" yaml:"vcs" json:"vcs"`
	Subdir  string `firestore:"subdir" yaml:"subdir" json:"subdir"`
}

// Storage defines the interface for retrieving and storing repository configurations.
type Storage interface {
	// Get retrieves the configuration for the given path.
	// It returns nil, nil if the path is not found.
	Get(ctx context.Context, path string) (*RepoConfig, error)

	// ListAll lists all the Go modules. It returns a slice of paths.
	ListAll(ctx context.Context) ([]string, error)

	// Set stores the configuration for the given path.
	Set(ctx context.Context, path string, config *RepoConfig) error

	// Delete deletes the Go module.
	Delete(ctx context.Context, path string) error

	// Close cleans up any resources used by the storage.
	// The context parameter is used for tracing.
	Close(ctx context.Context) error
}
