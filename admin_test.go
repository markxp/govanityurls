package govanityurls

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markxp/govanityurls/storage"
)

func TestHandleWriteConfig(t *testing.T) {
	asyncCfg := &AsyncRegisterConfig{
		TaskClientServiceAccount: "cloudtasks@fake.service.account",
		TaskClient:               nil, // Mocking sync execution since TaskClient is interface but nil -> sync
	}

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
			wantStatus: http.StatusNotFound,
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
		_ = store.Set(context.Background(), "/my-repo", &storage.RepoConfig{
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
		_ = store.Set(context.Background(), "/my-repo/sub", &storage.RepoConfig{
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

	// Test GET /_admin/ trailing slash (empty path -> 404)
	t.Run("GET /_admin/ Trailing Slash Not Found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_admin/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /_admin/ status = %v, want %v", rec.Code, http.StatusNotFound)
		}
	})
}
