package govanityurls

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/markxp/govanityurls/contrib/cloudsourcerepo"
	"github.com/markxp/govanityurls/contrib/github"
	"github.com/markxp/govanityurls/registry"
	"github.com/markxp/govanityurls/storage"
)

type mockStorage struct {
	mu   sync.RWMutex
	data map[string]*storage.RepoConfig
}

func (m *mockStorage) Get(_ context.Context, path string) (*storage.RepoConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if val, ok := m.data[path]; ok {
		return val, nil
	}
	return nil, nil
}

func (m *mockStorage) Set(_ context.Context, path string, config *storage.RepoConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]*storage.RepoConfig)
	}
	m.data[path] = config
	return nil
}

func (m *mockStorage) Close(_ context.Context) error {
	return nil
}

func (m *mockStorage) ListAll(_ context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var paths []string
	for k := range m.data {
		paths = append(paths, k)
	}
	return paths, nil
}

func (m *mockStorage) Delete(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, path)
	return nil
}

type mockRepoChecker struct {
	repos map[string]string
}

func (m *mockRepoChecker) CheckRepo(_ context.Context, repoName string) (string, error) {
	if url, ok := m.repos[repoName]; ok {
		return url, nil
	}
	return "", nil
}

func newTestServer(store storage.Storage, fallback *FallbackConfig, asyncCfg *AsyncRegisterConfig, cacheMaxAge int) *http.ServeMux {
	app := NewApp("example.com", cacheMaxAge, store, fallback, []registry.Registry{&github.GithubRegistry{}}, asyncCfg, nil)
	mux := http.NewServeMux()
	app.RegisterHandlers(mux)
	return mux
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		path       string // requested path
		registered map[string]*storage.RepoConfig
		csrRepos   map[string]string

		wantStatus int
		goImport   string
		goSource   string
		humanCheck string // string to check for on human page
	}{
		{
			name: "found in storage",
			path: "/portmidi",
			registered: map[string]*storage.RepoConfig{
				"/portmidi": {
					Repo:    "https://github.com/rakyll/portmidi",
					Display: "https://github.com/rakyll/portmidi _ _",
					VCS:     "git",
				},
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/portmidi git https://github.com/rakyll/portmidi",
			goSource:   "example.com/portmidi https://github.com/rakyll/portmidi _ _",
		},
		{
			name:       "not found anywhere",
			path:       "/404",
			registered: nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "found in CSR",
			path: "/my-repo",
			csrRepos: map[string]string{
				"my-repo": "https://source.developers.google.com/p/test-project/r/my-repo",
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/my-repo git https://source.developers.google.com/p/test-project/r/my-repo",
			goSource:   "example.com/my-repo https://source.cloud.google.com/p/test-project/r/my-repo https://source.cloud.google.com/p/test-project/r/my-repo/+/main:{dir} https://source.cloud.google.com/p/test-project/r/my-repo/+/main:{dir}/{file}#L{line}",
		},
		{
			name: "found in CSR with sub-path",
			path: "/my-repo/subpkg",
			csrRepos: map[string]string{
				"my-repo": "https://source.developers.google.com/p/test-project/r/my-repo",
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/my-repo git https://source.developers.google.com/p/test-project/r/my-repo",
			humanCheck: "Documentation",
		},
		{
			name: "with subdir and go-get=1",
			path: "/with-subdir?go-get=1",
			registered: map[string]*storage.RepoConfig{
				"/with-subdir": {
					Repo:    "https://github.com/rakyll/portmidi",
					VCS:     "git",
					Subdir:  "go-pkg",
					Display: "https://github.com/rakyll/portmidi _ _",
				},
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/with-subdir git https://github.com/rakyll/portmidi go-pkg",
			goSource:   "example.com/with-subdir https://github.com/rakyll/portmidi _ _",
		},
		{
			name: "Scenario 1: /a is module, /b/c is package",
			path: "/a/b/c?go-get=1",
			registered: map[string]*storage.RepoConfig{
				"/a": {Repo: "https://github.com/user/a", VCS: "git"},
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/a git https://github.com/user/a",
		},
		{
			name: "Scenario 1 (Human): doc link for /a/b/c",
			path: "/a/b/c",
			registered: map[string]*storage.RepoConfig{
				"/a": {Repo: "https://github.com/user/a", VCS: "git"},
			},
			wantStatus: http.StatusOK,
			humanCheck: `href="https://pkg.go.dev/example.com/a/b/c"`,
		},
		{
			name: "Scenario 2: /a/b is module, /c is package",
			path: "/a/b/c?go-get=1",
			registered: map[string]*storage.RepoConfig{
				"/a":   {Repo: "https://github.com/user/a", VCS: "git"},
				"/a/b": {Repo: "https://github.com/user/b", VCS: "git", Subdir: "b"},
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/a/b git https://github.com/user/b b",
		},
		{
			name: "Scenario 2 (Human): doc link for /a/b/c",
			path: "/a/b/c",
			registered: map[string]*storage.RepoConfig{
				"/a":   {Repo: "https://github.com/user/a", VCS: "git"},
				"/a/b": {Repo: "https://github.com/user/b", VCS: "git", Subdir: "b"},
			},
			wantStatus: http.StatusOK,
			humanCheck: `href="https://pkg.go.dev/example.com/a/b/c"`,
		},
		{
			name: "Scenario 3: /a/b/c is module",
			path: "/a/b/c?go-get=1",
			registered: map[string]*storage.RepoConfig{
				"/a":     {Repo: "https://github.com/user/a", VCS: "git"},
				"/a/b/c": {Repo: "https://github.com/user/c", VCS: "git", Subdir: "b/c"},
			},
			wantStatus: http.StatusOK,
			goImport:   "example.com/a/b/c git https://github.com/user/c b/c",
		},
		{
			name: "human page (no go-get=1)",
			path: "/portmidi",
			registered: map[string]*storage.RepoConfig{
				"/portmidi": {
					Repo:    "https://github.com/rakyll/portmidi",
					Display: "https://github.com/rakyll/portmidi _ _",
					VCS:     "git",
				},
			},
			wantStatus: http.StatusOK,
			humanCheck: "Go Module",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &mockStorage{
				data: map[string]*storage.RepoConfig{},
			}
			maps.Copy(store.data, test.registered)

			csrChecker := &mockRepoChecker{repos: test.csrRepos}
			fallback := &FallbackConfig{
				RepoChecker: csrChecker,
				Registry:    &cloudsourcerepo.Registry{},
			}

			mux := newTestServer(store, fallback, nil, 0)
			ts := httptest.NewServer(mux)
			t.Cleanup(ts.Close)

			resp, err := http.Get(ts.URL + test.path)
			if err != nil {
				t.Fatalf("http.Get: %v", err)
			}
			defer resp.Body.Close()

			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("io.ReadAll: %v", err)
			}

			if resp.StatusCode != test.wantStatus {
				t.Errorf("status code = %d; want %d", resp.StatusCode, test.wantStatus)
			}
			if test.wantStatus != http.StatusOK {
				return
			}

			if strings.Contains(test.path, "go-get=1") {
				if got := findMeta(data, "go-import"); got != test.goImport {
					t.Errorf("meta go-import = %q; want %q", got, test.goImport)
				}
				if test.goSource != "" {
					if got := findMeta(data, "go-source"); got != test.goSource {
						t.Errorf("meta go-source = %q; want %q", got, test.goSource)
					}
				}
			} else {
				// Human page
				if test.humanCheck != "" && !strings.Contains(string(data), test.humanCheck) {
					t.Errorf("expected human landing page to contain %q, but not found", test.humanCheck)
				}
				if findMeta(data, "go-import") != "" {
					t.Errorf("meta go-import should be empty for human page")
				}
			}
		})
	}
}

func findMeta(data []byte, name string) string {
	tagStart := []byte(`<meta name="` + name + `" content="`)
	i := strings.Index(string(data), string(tagStart))
	if i == -1 {
		return ""
	}
	content := string(data)[i+len(tagStart):]
	before, _, ok := strings.Cut(content, `"`)
	if !ok {
		return ""
	}
	return before
}

func TestCacheHeader(t *testing.T) {
	store := &mockStorage{data: map[string]*storage.RepoConfig{
		"/portmidi": {Repo: "https://github.com/rakyll/portmidi"},
	}}
	mux := newTestServer(store, nil, nil, 300)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/portmidi")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	resp.Body.Close()

	want := "public, max-age=300"
	if got := resp.Header.Get("Cache-Control"); got != want {
		t.Errorf("Cache-Control header = %q; want %q", got, want)
	}
}

func TestRegisterShutdownFuncNilTaskClient(t *testing.T) {
	store := &mockStorage{data: map[string]*storage.RepoConfig{}}
	asyncCfg := &AsyncRegisterConfig{TaskClient: nil}
	app := NewApp("example.com", 300, store, nil, nil, asyncCfg, nil)

	srv := &http.Server{}
	app.RegisterShutdownFunc(srv)
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned unexpected error: %v", err)
	}
}
