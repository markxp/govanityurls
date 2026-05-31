//go:build integration

package cloudsourcerepo

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"google.golang.org/api/option"
	sourcerepo "google.golang.org/api/sourcerepo/v1"
)

func TestIntegration_CheckRepo(t *testing.T) {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		t.Fatalf("GOOGLE_CLOUD_PROJECT not set")
	}

	ctx := context.Background()
	svc, err := NewService(ctx, projectID, option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("Failed to create CSR service: %v", err)
	}

	// Create a random repo name
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	repoName := fmt.Sprintf("test-repo-%d", rnd.Intn(100000))
	fullRepoName := fmt.Sprintf("projects/%s/repos/%s", projectID, repoName)

	// Create the repo using the raw service within CSRService (we'll need access or create a separate client)
	// Since svc.svc is private, we'll create a new client just for setup/teardown
	adminSvc, err := sourcerepo.NewService(ctx)
	if err != nil {
		t.Fatalf("Failed to create admin service: %v", err)
	}

	repo := &sourcerepo.Repo{
		Name: fullRepoName,
	}

	createdRepo, err := adminSvc.Projects.Repos.Create(fmt.Sprintf("projects/%s", projectID), repo).Do()
	if err != nil {
		t.Fatalf("Failed to create test repo: %v", err)
	}
	t.Logf("Created test repo: %s", createdRepo.Name)

	// Cleanup
	t.Cleanup(func() {
		if _, err := adminSvc.Projects.Repos.Delete(createdRepo.Name).Do(); err != nil {
			t.Errorf("Failed to delete test repo: %v", err)
		} else {
			t.Logf("Deleted test repo: %s", createdRepo.Name)
		}
	})

	// Test CheckRepo - Positive Case
	url, err := svc.CheckRepo(context.Background(), repoName)
	if err != nil {
		t.Errorf("CheckRepo failed: %v", err)
	}
	if url == "" {
		t.Errorf("CheckRepo returned empty URL for existing repo")
	}
	if url != createdRepo.Url {
		t.Errorf("CheckRepo returned wrong URL: got %q, want %q", url, createdRepo.Url)
	}

	// Test CheckRepo - Negative Case
	nonExistentRepo := fmt.Sprintf("non-existent-repo-%d", rnd.Intn(100000))
	url, err = svc.CheckRepo(context.Background(), nonExistentRepo)
	if err != nil {
		t.Errorf("CheckRepo failed for non-existent repo: %v", err)
	}
	if url != "" {
		t.Errorf("CheckRepo returned URL for non-existent repo: %q", url)
	}
}
