package govanityurls

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markxp/govanityurls/contrib/cloudsourcerepo"
	"github.com/markxp/govanityurls/contrib/github"
	"github.com/markxp/govanityurls/registry"
	"github.com/markxp/govanityurls/storage"
)

type mockStorage struct {
	data map[string]*storage.RepoConfig
}

func (m *mockStorage) Get(ctx context.Context, path string) (*storage.RepoConfig, error) {
	if val, ok := m.data[path]; ok {
		return val, nil
	}
	return nil, nil
}

func (m *mockStorage) Set(ctx context.Context, path string, config *storage.RepoConfig) error {
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
	var paths []string
	for k := range m.data {
		paths = append(paths, k)
	}
	return paths, nil
}

func (m *mockStorage) Delete(_ context.Context, path string) error {
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
	for p, h := range app.GetPublicHandlers() {
		mux.Handle(p, h)
	}
	for p, h := range app.GetPrivateHandlers() {
		mux.Handle(p, h)
	}
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

func TestHandleWriteConfig(t *testing.T) {
	asyncCfg := &AsyncRegisterConfig{
		TaskClientServiceAccount: "cloudtasks@fake.service.account",
		TaskClient:               nil, // Mocking sync execution since TaskClient is interface but nil -> sync
	}
	// To test "Success" with mocked TaskClient, we'd need a mock TaskSubmitter.
	// However, if we pass nil as TaskClient, doRegisterRepo falls back to direct storage write (sync).
	// Let's test the sync fallback path which is easier and sufficient for logic verification.

	store := &mockStorage{data: map[string]*storage.RepoConfig{}}
	mux := newTestServer(store, nil, asyncCfg, 0)
	path := "/_internal/registerRepo"

	tests := []struct {
		name       string
		method     string
		headers    map[string]string
		body       string
		wantStatus int
	}{
		{
			name:       "Method Not Allowed (matches generic repo handler)",
			method:     http.MethodGet,
			wantStatus: http.StatusNotFound, // Mux 1.22+ might handle method matching differently or fallback to /
		},
		{
			name:   "Success",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:       `{"path": "foo", "config": {"repo": "https://github.com/example/foo", "vcs": "git", "display": "https://github.com/example/foo https://github.com/example/foo/tree/main{/dir} https://github.com/example/foo/blob/main{/dir}/{file}#L{line}"}}`,
			wantStatus: http.StatusCreated,
		},
		{
			name:   "Invalid Display Format",
			method: http.MethodPost,
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:       `{"path": "foo", "config": {"repo": "https://github.com/example/foo", "vcs": "git", "display": "invalid format"}}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, path, strings.NewReader(test.body))
			for k, v := range test.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != test.wantStatus {
				t.Errorf("Test %q status = %v, want %v", test.name, rec.Code, test.wantStatus)
			}
		})
	}
}

func TestAdminHandlers(t *testing.T) {
	store := &mockStorage{data: map[string]*storage.RepoConfig{}}
	mux := newTestServer(store, nil, nil, 0)

	// Test GET /_admin
	t.Run("GET /_admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_admin", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /_admin status = %v, want %v", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "<form") {
			t.Errorf("GET /_admin body missing form tag")
		}
	})

	// Test POST /_admin (Success)
	t.Run("POST /_admin Success", func(t *testing.T) {
		form := strings.NewReader("path=foo&repo=https://github.com/foo/bar&vcs=git")
		req := httptest.NewRequest(http.MethodPost, "/_admin", form)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK { // Returns 200 with success message
			t.Errorf("POST /_admin status = %v, want %v", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "registered successfully") {
			t.Errorf("POST /_admin body missing success message")
		}
		if val, err := store.Get(context.Background(), "/foo"); err != nil || val == nil {
			t.Errorf("Repo not written to storage")
		}
	})

	// Test POST /_admin (Validation Error)
	t.Run("POST /_admin Validation Error", func(t *testing.T) {
		form := strings.NewReader("path=&repo=https://github.com/foo/bar") // Missing path
		req := httptest.NewRequest(http.MethodPost, "/_admin", form)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST /_admin status = %v, want %v", rec.Code, http.StatusBadRequest)
		}
	})

	// Test GET /_admin/{repoName} (Found)
	t.Run("GET /_admin/{repoName} Found", func(t *testing.T) {
		store.Set(context.Background(), "/my-repo", &storage.RepoConfig{
			Repo: "https://github.com/user/repo",
			VCS:  "git",
		})

		req := httptest.NewRequest(http.MethodGet, "/_admin/my-repo", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /_admin/my-repo status = %v, want %v", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `value="https://github.com/user/repo"`) {
			t.Errorf("GET /_admin/my-repo body missing pre-filled repo value")
		}
	})

	// Test GET /_admin/{repoName}/{path...} (Found)
	t.Run("GET /_admin/{repoName}/{path...} Found", func(t *testing.T) {
		store.Set(context.Background(), "/my-repo/sub", &storage.RepoConfig{
			Repo: "https://github.com/user/repo/sub",
			VCS:  "git",
		})

		req := httptest.NewRequest(http.MethodGet, "/_admin/my-repo/sub", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("GET /_admin/my-repo/sub status = %v, want %v", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `value="https://github.com/user/repo/sub"`) {
			t.Errorf("GET /_admin/my-repo/sub body missing pre-filled repo value")
		}
	})

	// Test GET /_admin/{repoName} (Not Found)
	t.Run("GET /_admin/{repoName} Not Found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_admin/unknown", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /_admin/unknown status = %v, want %v", rec.Code, http.StatusNotFound)
		}
	})
}
