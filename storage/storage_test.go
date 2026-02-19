package storage

import (
	"context"
	"testing"
	"time"
)

type mockStorage struct {
	data map[string]*RepoConfig
	err  error
}

func (m *mockStorage) Get(ctx context.Context, path string) (*RepoConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	if val, ok := m.data[path]; ok {
		return val, nil
	}
	return nil, nil
}

func (m *mockStorage) Set(ctx context.Context, path string, config *RepoConfig) error {
	if m.err != nil {
		return m.err
	}
	if m.data == nil {
		m.data = make(map[string]*RepoConfig)
	}
	m.data[path] = config
	return nil
}

func (m *mockStorage) Close(_ context.Context) error {
	return nil
}

func (m *mockStorage) ListAll(ctx context.Context) ([]string, error) {
	var paths []string
	for k := range m.data {
		paths = append(paths, k)
	}
	return paths, nil
}

func (m *mockStorage) Delete(ctx context.Context, path string) error {
	delete(m.data, path)
	return nil
}

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

func TestInMemoryStorage(t *testing.T) {
	ctx := context.Background()
	backend := &mockStorage{data: make(map[string]*RepoConfig)}

	// Initialize with size 10
	mem := NewInMemoryCache(10, time.Hour, backend)

	// Run generic storage tests
	testStorage(t, mem)

	// Test specific InMemoryCache behavior: Negative Caching
	// 1. Get non-existent
	if _, err := mem.Get(ctx, "/missing"); err != nil {
		t.Fatalf("Get missing: %v", err)
	}

	// 2. Add to backend directly (bypassing cache)
	backend.data["/missing"] = &RepoConfig{Repo: "https://github.com/example/missing"}

	// 3. Get again - should still be nil due to negative cache
	got, err := mem.Get(ctx, "/missing")
	if err != nil {
		t.Fatalf("Get missing again: %v", err)
	}
	if got != nil {
		t.Errorf("Get missing again = %v, want nil (negative cache)", got)
	}

	// Test Clear
	mem.Clear(ctx)

	// 4. Get again - should now find it in backend
	got, err = mem.Get(ctx, "/missing")
	if err != nil {
		t.Fatalf("Get after clear: %v", err)
	}
	if got == nil {
		t.Errorf("Get after clear = nil, want config")
	}
}
