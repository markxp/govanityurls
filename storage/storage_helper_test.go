package storage

import (
	"context"
	"testing"
)

func testStorage(t *testing.T, s Storage) {
	ctx := context.Background()

	// 1. Test Set / Get
	config := &RepoConfig{
		Repo:    "https://github.com/example/repo",
		VCS:     "git",
		Display: "https://github.com/example/repo _ _",
	}
	if err := s.Set(ctx, "/repo", config); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := s.Get(ctx, "/repo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil, want config")
	}
	if got.Repo != config.Repo || got.VCS != config.VCS || got.Display != config.Display {
		t.Errorf("Get = %v, want %v", got, config)
	}

	// 2. Test ListAll
	paths, err := s.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	found := false
	for _, p := range paths {
		if p == "/repo" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListAll did not find /repo in %v", paths)
	}

	// 3. Test Delete
	if err := s.Delete(ctx, "/repo"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err = s.Get(ctx, "/repo")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Errorf("Get after delete = %v, want nil", got)
	}
}
