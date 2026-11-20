package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/auth/credentials/idtoken"
	"github.com/markxp/govanityurls"
	oo "github.com/markxp/govanityurls/auth/oauth2"
	"github.com/markxp/govanityurls/auth/oidc"
	"github.com/markxp/govanityurls/storage"
	"golang.org/x/oauth2"
)

var globalEmulatorHost string

func TestMain(m *testing.M) {
	// Find a free port for the emulator
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(m.Run())
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	emulatorHost := fmt.Sprintf("127.0.0.1:%d", port)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "gcloud", "emulators", "firestore", "start", "--host-port="+emulatorHost)
	if err := cmd.Start(); err != nil {
		cancel()
		globalEmulatorHost = ""
	} else {
		globalEmulatorHost = emulatorHost
		if !waitForPort(emulatorHost, 10*time.Second) {
			cancel()
			_ = cmd.Wait()
			globalEmulatorHost = ""
		}
	}

	code := m.Run()

	if globalEmulatorHost != "" {
		cancel()
		_ = cmd.Wait()
	}

	os.Exit(code)
}

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
	m.data[path] = config
	return nil
}

func (m *mockStorage) Close(ctx context.Context) error { return nil }

func (m *mockStorage) ListAll(ctx context.Context) ([]string, error) {
	var keys []string
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockStorage) Delete(ctx context.Context, path string) error {
	delete(m.data, path)
	return nil
}

func TestCloudRunSetupApp(t *testing.T) {
	store := &mockStorage{
		data: map[string]*storage.RepoConfig{
			"/my-repo": {
				Repo: "https://github.com/user/my-repo",
				VCS:  "git",
			},
		},
	}
	logger := slog.Default()
	oauthConfig := &oauth2.Config{
		ClientID: "test-client-id",
	}

	allowedEmails := []string{"admin@example.com"}
	allowedDomains := []string{"example.com"}

	mockValidator := func(ctx context.Context, token, audience string) (*oo.Payload, error) {
		if token == "valid-token" {
			return &oo.Payload{
				Subject: "admin",
				Claims: map[string]any{
					"email": "admin@example.com",
				},
			}, nil
		}
		return nil, errors.New("invalid token")
	}

	stateSecret := []byte("test-secret-key-32-bytes-long-!")

	opts := []oo.Option{
		oo.WithAllowedEmails(allowedEmails),
		oo.WithAllowedDomains(allowedDomains),
		oo.WithLogger(logger),
		oo.WithJWTValidator(mockValidator),
	}
	authMiddleware, loginHandler, callbackHandler := oo.New(oauthConfig, stateSecret, "/_admin/login", "/_admin", opts...)

	app := govanityurls.NewApp("example.com", 300, store, nil, nil, nil, logger)

	mux := http.NewServeMux()
	registerRoutes(mux, app, authMiddleware, loginHandler, callbackHandler, nil)

	handler := mux

	if app == nil {
		t.Fatal("expected app to not be nil")
	}

	t.Run("Public path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/my-repo?go-get=1", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
	})

	t.Run("Private path unauthorized - GET redirects", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_admin", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Errorf("expected status 307 redirect, got %d", w.Code)
		}
	})

	t.Run("Private path unauthorized - POST returns 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/_admin", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Private path authorized via Bearer token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/_admin", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Returns 200/400 instead of 401/403 (since token is validated, it passes middleware to the handler)
		if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
			t.Errorf("expected authorized status, got %d", w.Code)
		}
	})

	t.Run("Login path bypasses authMiddleware", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_admin/login", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusTemporaryRedirect {
			t.Errorf("expected login handler to redirect (307), got %d", w.Code)
		}
		loc := w.Header().Get("Location")
		if !strings.Contains(loc, "client_id=test-client-id") {
			t.Errorf("expected redirect to carry client_id, got %q", loc)
		}
	})

	t.Run("Callback path bypasses authMiddleware", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_admin/callback", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected callback handler to return 400 Bad Request without cookie, got %d", w.Code)
		}
	})
}

func TestMainE2E(t *testing.T) {
	if globalEmulatorHost == "" {
		t.Skip("skipping E2E test because gcloud firestore emulator failed to start or is not installed")
	}

	// 1. Build the cmd/run binary
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "vanity-run")
	buildCmd := exec.Command("go", "build", "-o", binPath, "main.go")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build cmd/run binary: %v", err)
	}

	// 2. Find free ports for main server and health server
	serverPort := getFreePort(t)
	healthPort := getFreePort(t)

	// 3. Start the vanity-run binary
	runCmd := exec.Command(binPath)
	runCmd.Env = append(os.Environ(),
		"GOOGLE_CLOUD_PROJECT=test-project",
		"FIRESTORE_COLLECTION=vanity_urls_test",
		"FIRESTORE_EMULATOR_HOST="+globalEmulatorHost,
		"OAUTH_CLIENT_ID=test-client-id",
		"OAUTH_CLIENT_SECRET=test-client-secret",
		"OAUTH_REDIRECT_URL=http://localhost:"+fmt.Sprintf("%d", serverPort)+"/_admin/callback",
		"ALLOWED_DOMAINS=example.com",
		"PORT="+fmt.Sprintf("%d", serverPort),
		"HEALTH_PORT="+fmt.Sprintf("%d", healthPort),
		"OAUTH_STATE_SECRET=test-secret-key-at-least-32-bytes-long",
	)

	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start vanity-run binary: %v", err)
	}

	serverHost := fmt.Sprintf("127.0.0.1:%d", serverPort)
	healthHost := fmt.Sprintf("127.0.0.1:%d", healthPort)
	if !waitForPort(serverHost, 5*time.Second) || !waitForPort(healthHost, 5*time.Second) {
		t.Logf("stdout: %s", stdout.String())
		t.Logf("stderr: %s", stderr.String())
		_ = runCmd.Process.Kill()
		t.Fatal("timeout waiting for main/health server to be ready")
	}

	// 5. Query health check
	resp, err := http.Get("http://" + healthHost)
	if err != nil {
		t.Fatalf("failed to query health check: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected health check 200, got %d", resp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if string(bodyBytes) != "ok" {
		t.Errorf("expected health check response 'ok', got %q", string(bodyBytes))
	}

	// 6. Query admin (unauthorized redirect to login)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	respAdmin, err := client.Get("http://" + serverHost + "/_admin")
	if err != nil {
		t.Fatalf("failed to query /_admin: %v", err)
	}
	defer respAdmin.Body.Close()
	if respAdmin.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("expected redirect 307 for unauthorized admin access, got %d", respAdmin.StatusCode)
	}
	loc := respAdmin.Header.Get("Location")
	if !strings.HasPrefix(loc, "/_admin/login") {
		t.Errorf("expected location to redirect to login, got %q", loc)
	}

	// 7. Stop the process gracefully
	if err := runCmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- runCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("stdout: %s", stdout.String())
			t.Logf("stderr: %s", stderr.String())
			t.Errorf("process exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = runCmd.Process.Kill()
		t.Fatal("process failed to exit gracefully on SIGINT")
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func waitForPort(host string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", host, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

type mockTaskSubmitter struct{}

func (m *mockTaskSubmitter) CreateTask(ctx context.Context, payload *govanityurls.WriteConfigPayload) error {
	return nil
}

func (m *mockTaskSubmitter) Close(ctx context.Context) error { return nil }

func TestCloudRunInternalOIDCRouting(t *testing.T) {
	store := &mockStorage{data: map[string]*storage.RepoConfig{}}
	logger := slog.Default()
	oauthConfig := &oauth2.Config{ClientID: "test-client-id"}
	stateSecret := []byte("test-secret-key-32-bytes-long-!")

	authMiddleware, loginHandler, callbackHandler := oo.New(
		oauthConfig,
		stateSecret,
		"/_admin/login",
		"/_admin",
		oo.WithAllowedEmails([]string{"admin@example.com"}),
		oo.WithLogger(logger),
		// Mirrors idtoken.Validate: the token is only valid for the audience it was minted for.
		oo.WithJWTValidator(func(ctx context.Context, token, audience string) (*oo.Payload, error) {
			if token == "valid-admin-token" && audience == oauthConfig.ClientID {
				return &oo.Payload{
					Subject: "admin",
					Claims:  map[string]any{"email": "admin@example.com", "email_verified": true},
				}, nil
			}
			return nil, errors.New("invalid admin token or audience mismatch")
		}),
	)

	const taskSA = "cloud-tasks@my-project.iam.gserviceaccount.com"
	const targetAudience = "https://go.example.com/_internal/registerRepo"

	oidcValidator := oidc.NewValidator(
		targetAudience,
		[]string{taskSA},
		oidc.WithLogger(logger),
		oidc.WithJWTValidator(func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
			if token == "valid-tasks-oidc" && audience == targetAudience {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims: map[string]any{
						"email":          taskSA,
						"email_verified": true,
					},
				}, nil
			}
			return nil, errors.New("invalid oidc token")
		}),
	)

	asyncCfg := &govanityurls.AsyncRegisterConfig{
		TaskClient:               &mockTaskSubmitter{},
		TaskClientServiceAccount: taskSA,
	}
	app := govanityurls.NewApp("example.com", 300, store, nil, nil, asyncCfg, logger)
	mux := http.NewServeMux()
	registerRoutes(mux, app, authMiddleware, loginHandler, callbackHandler, oidcValidator.Middleware)

	t.Run("Internal endpoint rejects unauthenticated request with 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(`{"path":"/pkg","config":{"repo":"https://github.com/u/pkg","vcs":"git"}}`))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Internal endpoint rejects invalid OIDC token with 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(`{"path":"/pkg","config":{"repo":"https://github.com/u/pkg","vcs":"git"}}`))
		req.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Internal endpoint accepts valid Cloud Tasks OIDC token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(`{"path":"/pkg","config":{"repo":"https://github.com/u/pkg","vcs":"git","display":"https://github.com/u/pkg https://github.com/u/pkg/tree/main{/dir} https://github.com/u/pkg/blob/main{/dir}/{file}#L{line}"}}`))
		req.Header.Set("Authorization", "Bearer valid-tasks-oidc")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}
		if store.data["/pkg"] == nil {
			t.Errorf("expected /pkg to be saved in storage")
		}
	})

	t.Run("Admin token is rejected on internal endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer valid-admin-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Admin session cookie is rejected on internal endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: "session", Value: "valid-admin-token"})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Cloud Tasks token is rejected on admin endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_admin", nil)
		req.Header.Set("Authorization", "Bearer valid-tasks-oidc")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", w.Code)
		}
	})

	t.Run("Admin token is accepted on admin endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_admin", nil)
		req.Header.Set("Authorization", "Bearer valid-admin-token")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})
}

func TestRegisterRoutesFailsClosedWithoutInternalMiddleware(t *testing.T) {
	store := &mockStorage{data: map[string]*storage.RepoConfig{}}
	asyncCfg := &govanityurls.AsyncRegisterConfig{TaskClient: &mockTaskSubmitter{}}
	app := govanityurls.NewApp("example.com", 300, store, nil, nil, asyncCfg, slog.Default())
	noop := func(h http.Handler) http.Handler { return h }

	defer func() {
		if recover() == nil {
			t.Error("expected registerRoutes to panic when internal handlers exist without internalMiddleware")
		}
	}()
	registerRoutes(http.NewServeMux(), app, noop, http.NotFoundHandler(), http.NotFoundHandler(), nil)
}

func TestValidateAuthConfig(t *testing.T) {
	const validSecret = "test-secret-key-at-least-32-bytes-long"

	tests := []struct {
		name           string
		clientID       string
		clientSecret   string
		redirectURL    string
		stateSecret    string
		allowedEmails  []string
		allowedDomains []string
		wantErr        bool
	}{
		{
			name:          "valid with allowedEmails",
			clientID:      "cid",
			clientSecret:  "csecret",
			redirectURL:   "https://go.example.com/_admin/callback",
			stateSecret:   validSecret,
			allowedEmails: []string{"admin@example.com"},
			wantErr:       false,
		},
		{
			name:           "valid with allowedDomains",
			clientID:       "cid",
			clientSecret:   "csecret",
			redirectURL:    "https://go.example.com/_admin/callback",
			stateSecret:    validSecret,
			allowedDomains: []string{"example.com"},
			wantErr:        false,
		},
		{
			name:          "missing clientID",
			clientSecret:  "csecret",
			redirectURL:   "https://go.example.com/_admin/callback",
			stateSecret:   validSecret,
			allowedEmails: []string{"admin@example.com"},
			wantErr:       true,
		},
		{
			name:          "missing clientSecret",
			clientID:      "cid",
			redirectURL:   "https://go.example.com/_admin/callback",
			stateSecret:   validSecret,
			allowedEmails: []string{"admin@example.com"},
			wantErr:       true,
		},
		{
			name:          "missing redirectURL",
			clientID:      "cid",
			clientSecret:  "csecret",
			stateSecret:   validSecret,
			allowedEmails: []string{"admin@example.com"},
			wantErr:       true,
		},
		{
			name:          "short stateSecret (< 32 bytes)",
			clientID:      "cid",
			clientSecret:  "csecret",
			redirectURL:   "https://go.example.com/_admin/callback",
			stateSecret:   "too-short",
			allowedEmails: []string{"admin@example.com"},
			wantErr:       true,
		},
		{
			name:         "missing both allowedEmails and allowedDomains fails closed",
			clientID:     "cid",
			clientSecret: "csecret",
			redirectURL:  "https://go.example.com/_admin/callback",
			stateSecret:  validSecret,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAuthConfig(tc.clientID, tc.clientSecret, tc.redirectURL, tc.stateSecret, tc.allowedEmails, tc.allowedDomains)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateAuthConfig() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
