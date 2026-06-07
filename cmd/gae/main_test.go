package main

import (
	"bytes"
	"context"
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

	"github.com/markxp/govanityurls"
	"github.com/markxp/govanityurls/auth/iap"
	"github.com/markxp/govanityurls/storage"
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

func TestGAERouting(t *testing.T) {
	store := &mockStorage{
		data: map[string]*storage.RepoConfig{
			"/my-repo": {
				Repo: "https://github.com/user/my-repo",
				VCS:  "git",
			},
		},
	}
	logger := slog.Default()
	
	app := govanityurls.NewApp("example.com", 300, store, nil, nil, nil, logger)
	if app == nil {
		t.Fatal("expected app to not be nil")
	}

	mux := http.NewServeMux()

	// Register public handlers
	for path, handler := range app.GetPublicHandlers() {
		mux.Handle(path, handler)
	}

	// Register private handlers, wrapped in IAP validation middleware.
	iapValidator := iap.NewIAP("test-iap-audience", logger)
	iapMiddleware := iapValidator.Middleware

	for path, handler := range app.GetPrivateHandlers() {
		mux.Handle(path, iapMiddleware(handler))
	}

	mux.HandleFunc("GET /_ah/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	handler := mux

	t.Run("Public path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/my-repo?go-get=1", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "go-import") {
			t.Errorf("expected body to contain go-import meta tag")
		}
	})

	t.Run("Health path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_ah/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}
		if w.Body.String() != "ok" {
			t.Errorf("expected body 'ok', got %q", w.Body.String())
		}
	})

	t.Run("Private path without IAP header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_admin", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", w.Code)
		}
	})
}

func TestMainE2E(t *testing.T) {
	if globalEmulatorHost == "" {
		t.Skip("skipping E2E test because gcloud firestore emulator failed to start or is not installed")
	}

	// 1. Build the cmd/gae binary
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "vanity-gae")
	buildCmd := exec.Command("go", "build", "-o", binPath, "main.go")
	buildCmd.Dir = "."
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build cmd/gae binary: %v", err)
	}

	// 2. Find free ports for main server
	serverPort := getFreePort(t)

	// 3. Start the vanity-gae binary
	runCmd := exec.Command(binPath)
	runCmd.Env = append(os.Environ(),
		"GOOGLE_CLOUD_PROJECT=test-project",
		"FIRESTORE_COLLECTION=vanity_urls_test",
		"FIRESTORE_EMULATOR_HOST="+globalEmulatorHost,
		"IAP_AUDIENCE=test-iap-audience",
		"PORT="+fmt.Sprintf("%d", serverPort),
	)

	var stdout, stderr bytes.Buffer
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	if err := runCmd.Start(); err != nil {
		t.Fatalf("failed to start vanity-gae binary: %v", err)
	}

	serverHost := fmt.Sprintf("127.0.0.1:%d", serverPort)
	if !waitForPort(serverHost, 5*time.Second) {
		t.Logf("stdout: %s", stdout.String())
		t.Logf("stderr: %s", stderr.String())
		_ = runCmd.Process.Kill()
		t.Fatal("timeout waiting for main server to be ready")
	}

	// 5. Query health check (App Engine uses /_ah/health)
	resp, err := http.Get("http://" + serverHost + "/_ah/health")
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

	// 6. Query admin (unauthorized since no IAP header is passed)
	respAdmin, err := http.Get("http://" + serverHost + "/_admin")
	if err != nil {
		t.Fatalf("failed to query /_admin: %v", err)
	}
	defer respAdmin.Body.Close()
	if respAdmin.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected unauthorized 401, got %d", respAdmin.StatusCode)
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
