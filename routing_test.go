package govanityurls

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markxp/govanityurls/contrib/github"
	"github.com/markxp/govanityurls/registry"
	"github.com/markxp/govanityurls/storage"
)

func TestRegisterHandlers(t *testing.T) {
	headerGuard := func(headerName, expected string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(headerName) != expected {
					http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
			})
		}
	}

	t.Run("Separate Admin and Internal Middleware", func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{
			"/pub": {Repo: "https://github.com/example/pub", VCS: "git"},
		}}
		asyncCfg := &AsyncRegisterConfig{TaskClientServiceAccount: "tasks@example.iam.gserviceaccount.com"}
		app := NewApp("example.com", 300, store, nil, []registry.Registry{&github.GithubRegistry{}}, asyncCfg, nil)

		mux := http.NewServeMux()
		app.RegisterHandlers(
			mux,
			WithAdminMiddleware(headerGuard("X-Admin-Token", "secret-admin")),
			WithInternalMiddleware(headerGuard("X-Internal-Token", "secret-internal")),
		)

		// Public route requires no auth
		reqPub := httptest.NewRequest(http.MethodGet, "/pub?go-get=1", nil)
		recPub := httptest.NewRecorder()
		mux.ServeHTTP(recPub, reqPub)
		if recPub.Code != http.StatusOK {
			t.Errorf("public route status = %d, want 200", recPub.Code)
		}

		// Admin route rejects missing/internal token, accepts admin token
		reqAdminUnauth := httptest.NewRequest(http.MethodGet, "/_admin", nil)
		reqAdminUnauth.Header.Set("X-Internal-Token", "secret-internal")
		recAdminUnauth := httptest.NewRecorder()
		mux.ServeHTTP(recAdminUnauth, reqAdminUnauth)
		if recAdminUnauth.Code != http.StatusUnauthorized {
			t.Errorf("admin route with wrong token status = %d, want 401", recAdminUnauth.Code)
		}

		reqAdminAuth := httptest.NewRequest(http.MethodGet, "/_admin", nil)
		reqAdminAuth.Header.Set("X-Admin-Token", "secret-admin")
		recAdminAuth := httptest.NewRecorder()
		mux.ServeHTTP(recAdminAuth, reqAdminAuth)
		if recAdminAuth.Code != http.StatusOK {
			t.Errorf("admin route with admin token status = %d, want 200", recAdminAuth.Code)
		}

		// Internal route rejects admin token, accepts internal token
		body := `{"path":"mod","config":{"repo":"https://github.com/example/mod","vcs":"git"}}`
		reqIntUnauth := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(body))
		reqIntUnauth.Header.Set("X-Admin-Token", "secret-admin")
		recIntUnauth := httptest.NewRecorder()
		mux.ServeHTTP(recIntUnauth, reqIntUnauth)
		if recIntUnauth.Code != http.StatusUnauthorized {
			t.Errorf("internal route with admin token status = %d, want 401", recIntUnauth.Code)
		}

		reqIntAuth := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(body))
		reqIntAuth.Header.Set("X-Internal-Token", "secret-internal")
		recIntAuth := httptest.NewRecorder()
		mux.ServeHTTP(recIntAuth, reqIntAuth)
		if recIntAuth.Code != http.StatusCreated {
			t.Errorf("internal route with internal token status = %d, want 201", recIntAuth.Code)
		}
	})

	t.Run("WithPrivateMiddleware covers both admin and internal routes", func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{}}
		asyncCfg := &AsyncRegisterConfig{TaskClientServiceAccount: "tasks@example.iam.gserviceaccount.com"}
		app := NewApp("example.com", 300, store, nil, []registry.Registry{&github.GithubRegistry{}}, asyncCfg, nil)

		mux := http.NewServeMux()
		app.RegisterHandlers(mux, WithPrivateMiddleware(headerGuard("X-IAP-JWT", "valid-iap")))

		for _, target := range []struct {
			method string
			path   string
			body   string
			wantOK int
		}{
			{http.MethodGet, "/_admin", "", http.StatusOK},
			{http.MethodGet, "/_admin/missing", "", http.StatusNotFound},
			{http.MethodPost, "/_internal/registerRepo", `{"path":"mod","config":{"repo":"https://github.com/example/mod","vcs":"git"}}`, http.StatusCreated},
		} {
			reqUnauth := httptest.NewRequest(target.method, target.path, strings.NewReader(target.body))
			recUnauth := httptest.NewRecorder()
			mux.ServeHTTP(recUnauth, reqUnauth)
			if recUnauth.Code != http.StatusUnauthorized {
				t.Errorf("%s %s unauthenticated status = %d, want 401", target.method, target.path, recUnauth.Code)
			}

			reqAuth := httptest.NewRequest(target.method, target.path, strings.NewReader(target.body))
			reqAuth.Header.Set("X-IAP-JWT", "valid-iap")
			recAuth := httptest.NewRecorder()
			mux.ServeHTTP(recAuth, reqAuth)
			if recAuth.Code != target.wantOK {
				t.Errorf("%s %s authenticated status = %d, want %d", target.method, target.path, recAuth.Code, target.wantOK)
			}
		}
	})

	t.Run("Backward-compatible GetPublicHandlers and GetPrivateHandlers", func(t *testing.T) {
		store := &mockStorage{data: map[string]*storage.RepoConfig{
			"/legacy": {Repo: "https://github.com/example/legacy", VCS: "git"},
		}}
		asyncCfg := &AsyncRegisterConfig{TaskClientServiceAccount: "tasks@example.iam.gserviceaccount.com"}
		app := NewApp("example.com", 300, store, nil, []registry.Registry{&github.GithubRegistry{}}, asyncCfg, nil)

		mux := http.NewServeMux()
		for p, h := range app.GetPublicHandlers() {
			mux.Handle(p, h)
		}
		for p, h := range app.GetPrivateHandlers() {
			mux.Handle(p, h)
		}

		req := httptest.NewRequest(http.MethodGet, "/legacy?go-get=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /legacy?go-get=1 status = %d, want 200", rec.Code)
		}
	})
}

func TestRegisterHandlersPanics(t *testing.T) {
	noopMW := func(next http.Handler) http.Handler { return next }

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "WithAdminMiddleware(nil) panics",
			fn: func() {
				_ = WithAdminMiddleware(nil)
			},
		},
		{
			name: "WithInternalMiddleware(nil) panics",
			fn: func() {
				_ = WithInternalMiddleware(nil)
			},
		},
		{
			name: "WithPrivateMiddleware(nil) panics",
			fn: func() {
				_ = WithPrivateMiddleware(nil)
			},
		},
		{
			name: "WithAdminMiddleware without WithInternalMiddleware when asyncCfg != nil panics",
			fn: func() {
				store := &mockStorage{data: map[string]*storage.RepoConfig{}}
				asyncCfg := &AsyncRegisterConfig{TaskClientServiceAccount: "tasks@example.iam.gserviceaccount.com"}
				app := NewApp("example.com", 300, store, nil, nil, asyncCfg, nil)
				app.RegisterHandlers(http.NewServeMux(), WithAdminMiddleware(noopMW))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected panic in %s, but did not panic", tc.name)
				}
			}()
			tc.fn()
		})
	}
}
